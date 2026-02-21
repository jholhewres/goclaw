package security

import (
	"log/slog"
	"net"
	"testing"
)

func newTestSSRFGuard(cfg SSRFConfig) *SSRFGuard {
	return NewSSRFGuard(cfg, slog.Default())
}

func TestSSRFGuard_BlocksLocalhost(t *testing.T) {
	t.Parallel()
	g := newTestSSRFGuard(SSRFConfig{})

	blocked := []string{
		"http://localhost/secret",
		"http://localhost:8080/api",
		"http://0.0.0.0/admin",
	}
	for _, u := range blocked {
		if err := g.IsAllowed(u); err == nil {
			t.Errorf("expected %q to be blocked", u)
		}
	}
}

func TestSSRFGuard_BlocksLoopbackIP(t *testing.T) {
	t.Parallel()
	g := newTestSSRFGuard(SSRFConfig{})

	if err := g.IsAllowed("http://127.0.0.1/secret"); err == nil {
		t.Error("expected 127.0.0.1 to be blocked")
	}
	if err := g.IsAllowed("http://127.0.0.42/api"); err == nil {
		t.Error("expected 127.0.0.42 to be blocked")
	}
}

func TestSSRFGuard_BlocksPrivateIPs(t *testing.T) {
	t.Parallel()
	g := newTestSSRFGuard(SSRFConfig{})

	privateIPs := []string{
		"http://10.0.0.1/internal",
		"http://172.16.0.1/admin",
		"http://172.31.255.255/x",
		"http://192.168.1.1/router",
	}
	for _, u := range privateIPs {
		if err := g.IsAllowed(u); err == nil {
			t.Errorf("expected private IP %q to be blocked", u)
		}
	}
}

func TestSSRFGuard_AllowsPrivateWhenConfigured(t *testing.T) {
	t.Parallel()
	g := newTestSSRFGuard(SSRFConfig{AllowPrivate: true})

	if err := g.IsAllowed("http://10.0.0.1/ok"); err != nil {
		t.Errorf("AllowPrivate=true should allow 10.x: %v", err)
	}
	if err := g.IsAllowed("http://192.168.1.1/ok"); err != nil {
		t.Errorf("AllowPrivate=true should allow 192.168.x: %v", err)
	}
}

func TestSSRFGuard_BlocksLinkLocal(t *testing.T) {
	t.Parallel()
	g := newTestSSRFGuard(SSRFConfig{AllowPrivate: true}) // even with AllowPrivate

	if err := g.IsAllowed("http://169.254.169.254/metadata"); err == nil {
		t.Error("expected metadata IP 169.254.169.254 to be blocked")
	}
}

func TestSSRFGuard_BlocksFileScheme(t *testing.T) {
	t.Parallel()
	g := newTestSSRFGuard(SSRFConfig{})

	if err := g.IsAllowed("file:///etc/passwd"); err == nil {
		t.Error("expected file:// to be blocked")
	}
}

func TestSSRFGuard_BlocksNonHTTPSchemes(t *testing.T) {
	t.Parallel()
	g := newTestSSRFGuard(SSRFConfig{})

	schemes := []string{
		"ftp://evil.com/file",
		"gopher://evil.com/x",
	}
	for _, u := range schemes {
		if err := g.IsAllowed(u); err == nil {
			t.Errorf("expected non-HTTP scheme to be blocked: %s", u)
		}
	}
}

func TestSSRFGuard_BlocksBlacklistedHost(t *testing.T) {
	t.Parallel()
	g := newTestSSRFGuard(SSRFConfig{BlockedHosts: []string{"evil.com"}})

	if err := g.IsAllowed("http://evil.com/path"); err == nil {
		t.Error("expected blacklisted host to be blocked")
	}
}

func TestSSRFGuard_WhitelistEnforced(t *testing.T) {
	t.Parallel()
	// Use a resolvable host for the positive case.
	g := newTestSSRFGuard(SSRFConfig{AllowedHosts: []string{"google.com"}})

	if err := g.IsAllowed("http://google.com/"); err != nil {
		t.Errorf("whitelisted host should be allowed: %v", err)
	}
	if err := g.IsAllowed("http://other.com/v1"); err == nil {
		t.Error("non-whitelisted host should be blocked")
	}
}

func TestSSRFGuard_EmptyURL(t *testing.T) {
	t.Parallel()
	g := newTestSSRFGuard(SSRFConfig{})

	if err := g.IsAllowed(""); err == nil {
		t.Error("expected error for empty URL")
	}
}

func TestExtractEmbeddedIPv4_NAT64(t *testing.T) {
	t.Parallel()
	// 64:ff9b::7f00:1 embeds 127.0.0.1
	ip := net.ParseIP("64:ff9b::7f00:1")
	embedded := extractEmbeddedIPv4(ip)
	if embedded == nil {
		t.Fatal("expected embedded IPv4 from NAT64 address")
	}
	if !embedded.Equal(net.ParseIP("127.0.0.1")) {
		t.Errorf("expected 127.0.0.1, got %s", embedded.String())
	}
}

func TestExtractEmbeddedIPv4_6to4(t *testing.T) {
	t.Parallel()
	// 2002:7f00:1:: embeds 127.0.0.1
	ip := net.ParseIP("2002:7f00:1::")
	embedded := extractEmbeddedIPv4(ip)
	if embedded == nil {
		t.Fatal("expected embedded IPv4 from 6to4 address")
	}
	if !embedded.Equal(net.ParseIP("127.0.0.1")) {
		t.Errorf("expected 127.0.0.1, got %s", embedded.String())
	}
}

func TestExtractEmbeddedIPv4_ISATAP(t *testing.T) {
	t.Parallel()
	// ::5efe:7f00:1 embeds 127.0.0.1
	ip := net.ParseIP("::5efe:7f00:1")
	embedded := extractEmbeddedIPv4(ip)
	if embedded == nil {
		t.Fatal("expected embedded IPv4 from ISATAP address")
	}
	if !embedded.Equal(net.ParseIP("127.0.0.1")) {
		t.Errorf("expected 127.0.0.1, got %s", embedded.String())
	}
}

func TestSSRFGuard_BlocksBuiltinHosts(t *testing.T) {
	t.Parallel()
	g := newTestSSRFGuard(SSRFConfig{})

	blocked := []string{
		"localhost.localdomain",
		"metadata.google.internal",
	}
	for _, host := range blocked {
		// These are blocked at the hostname check before DNS resolution.
		url := "http://" + host + "/path"
		if err := g.IsAllowed(url); err == nil {
			t.Errorf("expected %q to be blocked", url)
		}
	}
}

func TestSSRFGuard_BlocksLegacyIPv4Formats(t *testing.T) {
	t.Parallel()
	g := newTestSSRFGuard(SSRFConfig{})

	tests := []struct {
		name string
		url  string
		desc string
	}{
		// Octal notation (0177 = 127 in octal)
		{"octal_loopback", "http://0177.0.0.1/secret", "octal notation for 127.0.0.1"},
		{"octal_short", "http://0177.0.1/secret", "partial octal notation"},

		// Hex notation
		{"hex_prefix", "http://0x7f.0.0.1/secret", "hex prefix notation"},

		// Short notation (fewer than 4 octets)
		{"short_loopback", "http://127.1/secret", "short notation 127.1 -> 127.0.0.1"},
		{"short_partial", "http://127.0.1/secret", "short notation with 3 octets"},

		// Mixed legacy forms
		{"octal_in_octet", "http://192.168.010.1/secret", "010 is octal for 8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := g.IsAllowed(tt.url)
			if err == nil {
				t.Errorf("expected %s (%s) to be blocked", tt.url, tt.desc)
			}
		})
	}
}

func TestValidateIPv4Literal_ValidFormats(t *testing.T) {
	t.Parallel()

	validHosts := []string{
		"192.168.1.1",
		"10.0.0.1",
		"8.8.8.8",
		"1.2.3.4",
		"255.255.255.255",
		"0.0.0.1", // Single 0 is valid
		"example.com",
		"sub.example.com",
		"localhost",
	}

	for _, host := range validHosts {
		err := validateIPv4Literal(host)
		if err != nil {
			t.Errorf("expected %q to be valid, got error: %v", host, err)
		}
	}
}

func TestValidateIPv4Literal_InvalidFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		host string
		desc string
	}{
		{"0177.0.0.1", "octal notation"},
		{"0x7f.0.0.1", "hex notation"},
		{"127.1", "short notation (2 octets)"},
		{"127.0.1", "short notation (3 octets)"},
		{"192.168.01.1", "leading zero in octet"},
		{"192.168.001.1", "leading zeros in octet"},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			err := validateIPv4Literal(tt.host)
			if err == nil {
				t.Errorf("expected %s (%s) to be invalid", tt.host, tt.desc)
			}
		})
	}
}
