package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunSecurityAudit_AllClean(t *testing.T) {
	t.Parallel()

	opts := AuditOptions{
		VaultConfigured: true,
		SSRFEnabled:     true,
	}
	report := RunSecurityAudit(opts)

	if report.TotalChecks != len(SecurityChecks) {
		t.Errorf("TotalChecks = %d, want %d", report.TotalChecks, len(SecurityChecks))
	}
	if len(report.Findings) != 0 {
		t.Errorf("Findings = %d, want 0; findings: %+v", len(report.Findings), report.Findings)
	}
	if report.CriticalCount != 0 || report.WarningCount != 0 || report.InfoCount != 0 {
		t.Errorf("counts = (%d, %d, %d), want (0, 0, 0)",
			report.CriticalCount, report.WarningCount, report.InfoCount)
	}
}

func TestRunSecurityAudit_Summary(t *testing.T) {
	t.Parallel()

	t.Run("no findings", func(t *testing.T) {
		t.Parallel()
		report := &AuditReport{TotalChecks: 10}
		s := report.Summary()
		if s != "Security audit passed: 10 checks, no findings." {
			t.Errorf("Summary = %q", s)
		}
	})

	t.Run("with findings", func(t *testing.T) {
		t.Parallel()
		report := &AuditReport{
			TotalChecks:   10,
			CriticalCount: 1,
			WarningCount:  2,
			InfoCount:     1,
			Findings:      make([]AuditFinding, 4),
		}
		s := report.Summary()
		if s != "Security audit: 10 checks, 1 critical, 2 warnings, 1 info." {
			t.Errorf("Summary = %q", s)
		}
	})
}

func TestCheckVaultNotConfigured(t *testing.T) {
	t.Parallel()

	t.Run("configured", func(t *testing.T) {
		t.Parallel()
		f := checkVaultNotConfigured(AuditOptions{VaultConfigured: true})
		if f != nil {
			t.Errorf("expected nil, got %+v", f)
		}
	})

	t.Run("not configured", func(t *testing.T) {
		t.Parallel()
		f := checkVaultNotConfigured(AuditOptions{VaultConfigured: false})
		if f == nil {
			t.Fatal("expected finding, got nil")
		}
		if f.Severity != SeverityWarning {
			t.Errorf("Severity = %q, want %q", f.Severity, SeverityWarning)
		}
		if f.CheckID != "vault.not_configured" {
			t.Errorf("CheckID = %q", f.CheckID)
		}
	})
}

func TestCheckRawAPIKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		apiKey   string
		wantNil  bool
	}{
		{"empty key", "", true},
		{"vault reference", "vault:my_key", true},
		{"env reference", "${API_KEY}", true},
		{"short key", "test", true},
		{"too short for prefix check", "mykey1234567", true},
		{"test key prefix", "sk_test_1234567890abcdef", true},
		{"real openai key", "sk-1234567890abcdefghij", false},
		{"github token", "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef", false},
		{"slack token", "xoxb-1234567890-abcdefghij", false},
		{"high entropy long", "AbCdEfGh1234IjKlMnOp5678QrStUvWx", false},
		{"low entropy long", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := checkRawAPIKeys(AuditOptions{APIKey: tt.apiKey, Provider: "openai"})
			if tt.wantNil && f != nil {
				t.Errorf("expected nil, got %+v", f)
			}
			if !tt.wantNil && f == nil {
				t.Error("expected finding, got nil")
			}
			if !tt.wantNil && f != nil && f.Severity != SeverityCritical {
				t.Errorf("Severity = %q, want %q", f.Severity, SeverityCritical)
			}
		})
	}
}

func TestCheckConfigPermissions(t *testing.T) {
	t.Parallel()

	t.Run("empty path", func(t *testing.T) {
		t.Parallel()
		f := checkConfigPermissions(AuditOptions{ConfigPath: ""})
		if f != nil {
			t.Errorf("expected nil, got %+v", f)
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		t.Parallel()
		f := checkConfigPermissions(AuditOptions{ConfigPath: "/nonexistent/config.yaml"})
		if f != nil {
			t.Errorf("expected nil, got %+v", f)
		}
	})

	t.Run("world-readable", func(t *testing.T) {
		t.Parallel()
		tmp := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(tmp, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
		f := checkConfigPermissions(AuditOptions{ConfigPath: tmp})
		if f == nil {
			t.Fatal("expected finding, got nil")
		}
		if f.Severity != SeverityWarning {
			t.Errorf("Severity = %q, want %q", f.Severity, SeverityWarning)
		}
	})

	t.Run("owner-only", func(t *testing.T) {
		t.Parallel()
		tmp := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(tmp, []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}
		f := checkConfigPermissions(AuditOptions{ConfigPath: tmp})
		if f != nil {
			t.Errorf("expected nil, got %+v", f)
		}
	})
}

func TestCheckSessionsPermissions(t *testing.T) {
	t.Parallel()

	t.Run("empty path", func(t *testing.T) {
		t.Parallel()
		f := checkSessionsPermissions(AuditOptions{SessionsDir: ""})
		if f != nil {
			t.Errorf("expected nil, got %+v", f)
		}
	})

	t.Run("world-readable", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := os.Chmod(dir, 0755); err != nil {
			t.Fatal(err)
		}
		f := checkSessionsPermissions(AuditOptions{SessionsDir: dir})
		if f == nil {
			t.Fatal("expected finding, got nil")
		}
		if f.CheckID != "fs.sessions_permissions" {
			t.Errorf("CheckID = %q", f.CheckID)
		}
	})

	t.Run("owner-only", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := os.Chmod(dir, 0700); err != nil {
			t.Fatal(err)
		}
		f := checkSessionsPermissions(AuditOptions{SessionsDir: dir})
		if f != nil {
			t.Errorf("expected nil, got %+v", f)
		}
	})
}

func TestCheckGatewayBindNoAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    AuditOptions
		wantNil bool
	}{
		{"gateway disabled", AuditOptions{GatewayEnabled: false}, true},
		{"empty bind", AuditOptions{GatewayEnabled: true, GatewayBind: ""}, true},
		{"localhost", AuditOptions{GatewayEnabled: true, GatewayBind: "127.0.0.1:8080"}, true},
		{"all interfaces with auth", AuditOptions{GatewayEnabled: true, GatewayBind: "0.0.0.0:8080", GatewayAuth: true}, true},
		{"all interfaces no auth", AuditOptions{GatewayEnabled: true, GatewayBind: "0.0.0.0:8080", GatewayAuth: false}, false},
		{"colon prefix no auth", AuditOptions{GatewayEnabled: true, GatewayBind: ":8080", GatewayAuth: false}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := checkGatewayBindNoAuth(tt.opts)
			if tt.wantNil && f != nil {
				t.Errorf("expected nil, got %+v", f)
			}
			if !tt.wantNil && f == nil {
				t.Error("expected finding, got nil")
			}
			if !tt.wantNil && f != nil && f.Severity != SeverityCritical {
				t.Errorf("Severity = %q, want %q", f.Severity, SeverityCritical)
			}
		})
	}
}

func TestCheckCORSOpen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    AuditOptions
		wantNil bool
	}{
		{"gateway disabled", AuditOptions{GatewayEnabled: false, CORSOrigins: []string{"*"}}, true},
		{"empty cors", AuditOptions{GatewayEnabled: true, CORSOrigins: nil}, true},
		{"wildcard", AuditOptions{GatewayEnabled: true, CORSOrigins: []string{"*"}}, false},
		{"specific domain", AuditOptions{GatewayEnabled: true, CORSOrigins: []string{"https://example.com"}}, true},
		{"mixed with wildcard", AuditOptions{GatewayEnabled: true, CORSOrigins: []string{"https://example.com", "*"}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := checkCORSOpen(tt.opts)
			if tt.wantNil && f != nil {
				t.Errorf("expected nil, got %+v", f)
			}
			if !tt.wantNil && f == nil {
				t.Error("expected finding, got nil")
			}
		})
	}
}

func TestCheckSudoAllowed(t *testing.T) {
	t.Parallel()

	t.Run("not allowed", func(t *testing.T) {
		t.Parallel()
		f := checkSudoAllowed(AuditOptions{SudoAllowed: false})
		if f != nil {
			t.Errorf("expected nil, got %+v", f)
		}
	})

	t.Run("allowed", func(t *testing.T) {
		t.Parallel()
		f := checkSudoAllowed(AuditOptions{SudoAllowed: true})
		if f == nil {
			t.Fatal("expected finding, got nil")
		}
		if f.Severity != SeverityWarning {
			t.Errorf("Severity = %q, want %q", f.Severity, SeverityWarning)
		}
	})
}

func TestCheckSSRFDisabled(t *testing.T) {
	t.Parallel()

	t.Run("enabled", func(t *testing.T) {
		t.Parallel()
		f := checkSSRFDisabled(AuditOptions{SSRFEnabled: true})
		if f != nil {
			t.Errorf("expected nil, got %+v", f)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		t.Parallel()
		f := checkSSRFDisabled(AuditOptions{SSRFEnabled: false})
		if f == nil {
			t.Fatal("expected finding, got nil")
		}
		if f.Severity != SeverityWarning {
			t.Errorf("Severity = %q, want %q", f.Severity, SeverityWarning)
		}
	})
}

func TestCheckEmbeddingNoKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider string
		apiKey   string
		wantNil  bool
	}{
		{"no provider", "", "", true},
		{"none provider", "none", "", true},
		{"provider with key", "openai", "sk-123", true},
		{"provider without key", "openai", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := checkEmbeddingNoKey(AuditOptions{
				EmbeddingProvider: tt.provider,
				EmbeddingAPIKey:   tt.apiKey,
			})
			if tt.wantNil && f != nil {
				t.Errorf("expected nil, got %+v", f)
			}
			if !tt.wantNil && f == nil {
				t.Error("expected finding, got nil")
			}
			if !tt.wantNil && f != nil && f.Severity != SeverityInfo {
				t.Errorf("Severity = %q, want %q", f.Severity, SeverityInfo)
			}
		})
	}
}

func TestCheckVaultFilePermissions(t *testing.T) {
	t.Parallel()

	t.Run("empty path", func(t *testing.T) {
		t.Parallel()
		f := checkVaultFilePermissions(AuditOptions{VaultPath: ""})
		if f != nil {
			t.Errorf("expected nil, got %+v", f)
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		t.Parallel()
		f := checkVaultFilePermissions(AuditOptions{VaultPath: "/nonexistent/vault"})
		if f != nil {
			t.Errorf("expected nil, got %+v", f)
		}
	})

	t.Run("open permissions", func(t *testing.T) {
		t.Parallel()
		tmp := filepath.Join(t.TempDir(), ".devclaw.vault")
		if err := os.WriteFile(tmp, []byte("encrypted"), 0644); err != nil {
			t.Fatal(err)
		}
		f := checkVaultFilePermissions(AuditOptions{VaultPath: tmp})
		if f == nil {
			t.Fatal("expected finding, got nil")
		}
		if f.Severity != SeverityCritical {
			t.Errorf("Severity = %q, want %q", f.Severity, SeverityCritical)
		}
	})

	t.Run("owner-only", func(t *testing.T) {
		t.Parallel()
		tmp := filepath.Join(t.TempDir(), ".devclaw.vault")
		if err := os.WriteFile(tmp, []byte("encrypted"), 0600); err != nil {
			t.Fatal(err)
		}
		f := checkVaultFilePermissions(AuditOptions{VaultPath: tmp})
		if f != nil {
			t.Errorf("expected nil, got %+v", f)
		}
	})
}

func TestRunSecurityAudit_MultipleFindings(t *testing.T) {
	t.Parallel()

	opts := AuditOptions{
		VaultConfigured:   false,                // warning
		APIKey:            "sk-1234567890abcdef", // critical
		Provider:          "openai",
		SSRFEnabled:       false,                 // warning
		SudoAllowed:       true,                  // warning
		GatewayEnabled:    true,
		GatewayBind:       "0.0.0.0:8080",
		GatewayAuth:       false,                 // critical
		CORSOrigins:       []string{"*"},          // warning
		EmbeddingProvider: "openai",
		EmbeddingAPIKey:   "",                    // info
	}
	report := RunSecurityAudit(opts)

	if report.TotalChecks != len(SecurityChecks) {
		t.Errorf("TotalChecks = %d, want %d", report.TotalChecks, len(SecurityChecks))
	}
	if report.CriticalCount != 2 {
		t.Errorf("CriticalCount = %d, want 2", report.CriticalCount)
	}
	if report.WarningCount != 4 {
		t.Errorf("WarningCount = %d, want 4", report.WarningCount)
	}
	if report.InfoCount != 1 {
		t.Errorf("InfoCount = %d, want 1", report.InfoCount)
	}
	if len(report.Findings) != 7 {
		t.Errorf("len(Findings) = %d, want 7", len(report.Findings))
	}
}
