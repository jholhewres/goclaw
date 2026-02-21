// Package security – ssrf.go implements SSRF (Server-Side Request Forgery)
// protection for web_fetch and similar tools. Resolves hostnames first to
// defend against DNS rebinding, then validates resolved IPs against private
// ranges, metadata endpoints, and blocked hosts.
package security

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"
)

// builtinBlockedHosts are always blocked regardless of user config.
var builtinBlockedHosts = []string{
	"localhost.localdomain",
	"metadata.google.internal",
}

// SSRFConfig configures SSRF protection behavior.
type SSRFConfig struct {
	// AllowPrivate allows requests to private IPs (default: false).
	AllowPrivate bool `yaml:"allow_private"`

	// AllowedHosts is a whitelist. If set, only these hosts are allowed.
	AllowedHosts []string `yaml:"allowed_hosts"`

	// BlockedHosts is a blacklist (checked even if AllowPrivate is true).
	BlockedHosts []string `yaml:"blocked_hosts"`
}

// SSRFGuard validates URLs before outgoing HTTP requests to prevent SSRF.
type SSRFGuard struct {
	cfg    SSRFConfig
	logger *slog.Logger
}

// NewSSRFGuard creates a new SSRF guard from config.
func NewSSRFGuard(cfg SSRFConfig, logger *slog.Logger) *SSRFGuard {
	if logger == nil {
		logger = slog.Default()
	}
	return &SSRFGuard{
		cfg:    cfg,
		logger: logger.With("component", "ssrf_guard"),
	}
}

// IsAllowed checks if a URL is safe to fetch (not internal/private).
// Resolves the hostname first to defend against DNS rebinding.
func (g *SSRFGuard) IsAllowed(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Block non-HTTP(S) schemes.
	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case "http", "https":
		// OK
	default:
		if scheme != "" {
			g.logger.Warn("SSRF blocked: non-HTTP scheme", "url", rawURL, "scheme", scheme)
			return fmt.Errorf("SSRF: scheme %q not allowed (use http or https)", scheme)
		}
		// Empty scheme might be a host without scheme; block if it looks like file:// etc.
		if strings.HasPrefix(strings.ToLower(rawURL), "file:") {
			g.logger.Warn("SSRF blocked: file URL", "url", rawURL)
			return fmt.Errorf("SSRF: file:// URLs are not allowed")
		}
	}

	// Block file://
	if scheme == "file" {
		g.logger.Warn("SSRF blocked: file URL", "url", rawURL)
		return fmt.Errorf("SSRF: file:// URLs are not allowed")
	}

	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("SSRF: no host in URL")
	}

	// Enforce strict dotted-decimal IPv4 format (openclaw security hardening).
	// Block legacy forms: octal (0177.0.0.1), hex (0x7f.0.0.1), short (127.1),
	// and packed integer (2130706433) before DNS resolution.
	if err := validateIPv4Literal(host); err != nil {
		g.logger.Warn("SSRF blocked: legacy IPv4 format", "url", rawURL, "host", host)
		return err
	}

	// Block localhost and 0.0.0.0 by hostname.
	hostLower := strings.ToLower(host)
	if hostLower == "localhost" || hostLower == "0.0.0.0" {
		g.logger.Warn("SSRF blocked: localhost/0.0.0.0", "url", rawURL)
		return fmt.Errorf("SSRF: %s is not allowed", host)
	}

	// Check builtin blocked hostnames (always enforced).
	for _, blocked := range builtinBlockedHosts {
		if strings.EqualFold(hostLower, blocked) {
			g.logger.Warn("SSRF blocked: builtin blocked host", "url", rawURL, "host", host)
			return fmt.Errorf("SSRF: host %s is not allowed", host)
		}
	}

	// Check blocked hosts blacklist.
	for _, blocked := range g.cfg.BlockedHosts {
		if strings.EqualFold(host, blocked) {
			g.logger.Warn("SSRF blocked: host in blacklist", "url", rawURL, "host", host)
			return fmt.Errorf("SSRF: host %s is blocked", host)
		}
	}

	// If whitelist is set, only allowed hosts pass.
	if len(g.cfg.AllowedHosts) > 0 {
		allowed := false
		for _, h := range g.cfg.AllowedHosts {
			if strings.EqualFold(host, h) {
				allowed = true
				break
			}
		}
		if !allowed {
			g.logger.Warn("SSRF blocked: host not in whitelist", "url", rawURL, "host", host)
			return fmt.Errorf("SSRF: host %s is not in the allowed list", host)
		}
		// Whitelist passed; still need to resolve and check metadata.
	}

	// Resolve hostname FIRST (DNS rebinding protection).
	ips, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("SSRF: cannot resolve host %s: %w", host, err)
	}

	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			// Fail closed: unrecognised IP format is blocked.
			g.logger.Warn("SSRF blocked: unrecognised IP in DNS response", "url", rawURL, "ip", ipStr)
			return fmt.Errorf("SSRF: unrecognised IP address %q for host %s", ipStr, host)
		}

		if err := g.checkIP(ip, rawURL); err != nil {
			return err
		}
	}

	return nil
}

// validateIPv4Literal enforces strict dotted-decimal IPv4 format.
// Blocks legacy forms that some systems may interpret as loopback or private IPs:
//   - Octal: 0177.0.0.1, 0177.0.1 (each octet starting with 0 is octal)
//   - Hex: 0x7f.0.0.1, 0x7f000001
//   - Short: 127.1 (expands to 127.0.0.1), 127.0.1
//   - Packed integer: 2130706433 (decimal representation of 127.0.0.1)
//
// Only standard dotted-decimal (e.g., 127.0.0.1) passes validation.
func validateIPv4Literal(host string) error {
	// Check for hex prefixes (before any other checks).
	if strings.Contains(strings.ToLower(host), "0x") {
		return fmt.Errorf("SSRF: hex IPv4 notation not allowed (use standard dotted-decimal)")
	}

	// Check if host looks like it could be an IPv4 literal (digits and dots).
	if !isPossibleIPv4Literal(host) {
		return nil // Not an IPv4 literal, let DNS resolve it.
	}

	// Split into octets.
	parts := strings.Split(host, ".")
	if len(parts) > 4 {
		return fmt.Errorf("SSRF: invalid IPv4 address (too many octets)")
	}

	// Less than 4 parts could be short notation (e.g., 127.1).
	// Standard DNS resolution may interpret these differently across systems.
	if len(parts) < 4 {
		return fmt.Errorf("SSRF: short IPv4 notation not allowed (use standard dotted-decimal, e.g., 127.0.0.1)")
	}

	// Validate each octet.
	for _, part := range parts {
		if part == "" {
			return fmt.Errorf("SSRF: empty octet in IPv4 address")
		}

		// Check for leading zeros (octal notation).
		// "0" alone is fine, but "0177" or "007" is octal.
		if len(part) > 1 && part[0] == '0' {
			return fmt.Errorf("SSRF: octal IPv4 notation not allowed (use standard dotted-decimal)")
		}

		// Verify all characters are digits.
		for _, c := range part {
			if c < '0' || c > '9' {
				return fmt.Errorf("SSRF: invalid character in IPv4 address")
			}
		}

		// Verify value is 0-255 (already validated by net.ParseIP later,
		// but we fail earlier with a clearer error message).
		val := 0
		for _, c := range part {
			val = val*10 + int(c-'0')
		}
		if val > 255 {
			return fmt.Errorf("SSRF: IPv4 octet out of range")
		}
	}

	return nil
}

// isPossibleIPv4Literal checks if a hostname might be an IPv4 literal.
func isPossibleIPv4Literal(host string) bool {
	if host == "" {
		return false
	}
	digits := 0
	for _, c := range host {
		if c >= '0' && c <= '9' {
			digits++
		} else if c == '.' || c == 'x' || c == 'X' {
			// dots are OK, x/X might indicate hex
		} else {
			return false // Contains non-IPv4 characters (letters, etc.)
		}
	}
	return digits > 0 // At least one digit.
}

// extractEmbeddedIPv4 extracts an IPv4 address embedded in an IPv6 address
// via NAT64 (64:ff9b::/96), 6to4 (2002::/16), ISATAP (::5efe:), or Teredo (2001:0000::/32).
// Returns nil if the address is not an IPv6 transition mechanism.
func extractEmbeddedIPv4(ip net.IP) net.IP {
	ip6 := ip.To16()
	if ip6 == nil || ip.To4() != nil {
		return nil // not an IPv6 address
	}

	// NAT64: 64:ff9b::/96 — last 4 bytes are the IPv4 address.
	if ip6[0] == 0x00 && ip6[1] == 0x64 && ip6[2] == 0xff && ip6[3] == 0x9b &&
		ip6[4] == 0 && ip6[5] == 0 && ip6[6] == 0 && ip6[7] == 0 &&
		ip6[8] == 0 && ip6[9] == 0 && ip6[10] == 0 && ip6[11] == 0 {
		return net.IP(ip6[12:16])
	}

	// 6to4: 2002::/16 — bytes 2-5 are the IPv4 address.
	if ip6[0] == 0x20 && ip6[1] == 0x02 {
		return net.IP(ip6[2:6])
	}

	// Teredo: 2001:0000::/32 — last 4 bytes XOR 0xFFFFFFFF are IPv4.
	if ip6[0] == 0x20 && ip6[1] == 0x01 && ip6[2] == 0x00 && ip6[3] == 0x00 {
		embedded := make(net.IP, 4)
		v := binary.BigEndian.Uint32(ip6[12:16])
		binary.BigEndian.PutUint32(embedded, v^0xFFFFFFFF)
		return embedded
	}

	// ISATAP: ::5efe:<ipv4> — bytes 8-9 are 0x00,0x00 or 0x02,0x00; bytes 10-11 are 0x5e,0xfe.
	if ip6[10] == 0x5e && ip6[11] == 0xfe {
		return net.IP(ip6[12:16])
	}

	return nil
}

// checkIP validates a resolved IP against private ranges and metadata endpoints.
func (g *SSRFGuard) checkIP(ip net.IP, rawURL string) error {
	// Check IPv6 transition mechanisms — extract and validate embedded IPv4.
	if embedded := extractEmbeddedIPv4(ip); embedded != nil {
		if err := g.checkIP(embedded, rawURL); err != nil {
			return fmt.Errorf("SSRF: IPv6 transition address %s embeds blocked IPv4: %w", ip.String(), err)
		}
	}

	// Normalize to IPv4 for range checks.
	ip4 := ip.To4()
	if ip4 != nil {
		ip = ip4
	}

	// Loopback: 127.0.0.0/8
	if ip4 != nil && ip4[0] == 127 {
		g.logger.Warn("SSRF blocked: loopback IP", "url", rawURL, "ip", ip.String())
		return fmt.Errorf("SSRF: loopback IP %s is not allowed", ip.String())
	}

	// Private: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
	if ip4 != nil {
		if ip4[0] == 10 {
			if !g.cfg.AllowPrivate {
				g.logger.Warn("SSRF blocked: private IP 10.x", "url", rawURL, "ip", ip.String())
				return fmt.Errorf("SSRF: private IP %s is not allowed", ip.String())
			}
		}
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			if !g.cfg.AllowPrivate {
				g.logger.Warn("SSRF blocked: private IP 172.16-31.x", "url", rawURL, "ip", ip.String())
				return fmt.Errorf("SSRF: private IP %s is not allowed", ip.String())
			}
		}
		if ip4[0] == 192 && ip4[1] == 168 {
			if !g.cfg.AllowPrivate {
				g.logger.Warn("SSRF blocked: private IP 192.168.x", "url", rawURL, "ip", ip.String())
				return fmt.Errorf("SSRF: private IP %s is not allowed", ip.String())
			}
		}
	}

	// Link-local: 169.254.0.0/16 (includes metadata 169.254.169.254)
	if ip4 != nil && ip4[0] == 169 && ip4[1] == 254 {
		g.logger.Warn("SSRF blocked: link-local/metadata IP", "url", rawURL, "ip", ip.String())
		return fmt.Errorf("SSRF: link-local/metadata IP %s is not allowed", ip.String())
	}

	// 0.0.0.0
	if ip4 != nil && ip4[0] == 0 && ip4[1] == 0 && ip4[2] == 0 && ip4[3] == 0 {
		g.logger.Warn("SSRF blocked: 0.0.0.0", "url", rawURL)
		return fmt.Errorf("SSRF: 0.0.0.0 is not allowed")
	}

	// IPv6 loopback ::1
	if ip.To16() != nil && ip.To4() == nil {
		if ip.Equal(net.ParseIP("::1")) {
			g.logger.Warn("SSRF blocked: IPv6 loopback", "url", rawURL)
			return fmt.Errorf("SSRF: IPv6 loopback ::1 is not allowed")
		}
		// Link-local: fe80::/10
		if len(ip) >= 2 && ip[0] == 0xfe && (ip[1]&0xc0) == 0x80 {
			g.logger.Warn("SSRF blocked: IPv6 link-local", "url", rawURL, "ip", ip.String())
			return fmt.Errorf("SSRF: IPv6 link-local %s is not allowed", ip.String())
		}
	}

	return nil
}
