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
