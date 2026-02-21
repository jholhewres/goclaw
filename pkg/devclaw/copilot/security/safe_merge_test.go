package security

import (
	"testing"
)

func TestValidateMapKeys_SafeKeys(t *testing.T) {
	t.Parallel()
	safe := map[string]any{
		"name":   "test",
		"count":  42,
		"active": true,
	}
	if err := ValidateMapKeys(safe); err != nil {
		t.Errorf("expected safe keys to pass, got error: %v", err)
	}
}

func TestValidateMapKeys_BlockedKeys(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		m    map[string]any
	}{
		{"__proto__", map[string]any{"__proto__": "malicious"}},
		{"prototype", map[string]any{"prototype": "malicious"}},
		{"constructor", map[string]any{"constructor": "malicious"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateMapKeys(tt.m); err == nil {
				t.Errorf("expected blocked key %q to fail validation", tt.name)
			}
		})
	}
}

func TestValidateMapKeys_NestedBlockedKeys(t *testing.T) {
	t.Parallel()
	nested := map[string]any{
		"safe": "value",
		"nested": map[string]any{
			"__proto__": "malicious",
		},
	}
	if err := ValidateMapKeys(nested); err == nil {
		t.Error("expected nested blocked key to fail validation")
	}
}

func TestSafeMerge_Success(t *testing.T) {
	t.Parallel()
	dst := map[string]any{"a": 1}
	src := map[string]any{"b": 2}

	if err := SafeMerge(dst, src); err != nil {
		t.Errorf("expected merge to succeed, got error: %v", err)
	}
	if dst["a"] != 1 || dst["b"] != 2 {
		t.Errorf("expected dst to have both keys, got %v", dst)
	}
}

func TestSafeMerge_BlockedKey(t *testing.T) {
	t.Parallel()
	dst := map[string]any{"a": 1}
	src := map[string]any{"__proto__": "malicious"}

	if err := SafeMerge(dst, src); err == nil {
		t.Error("expected merge with blocked key to fail")
	}
}

func TestSafeMergeDeep_Success(t *testing.T) {
	t.Parallel()
	dst := map[string]any{
		"a": 1,
		"nested": map[string]any{
			"x": 10,
		},
	}
	src := map[string]any{
		"b": 2,
		"nested": map[string]any{
			"y": 20,
		},
	}

	if err := SafeMergeDeep(dst, src); err != nil {
		t.Errorf("expected deep merge to succeed, got error: %v", err)
	}

	nested := dst["nested"].(map[string]any)
	if nested["x"] != 10 || nested["y"] != 20 {
		t.Errorf("expected nested to have both keys, got %v", nested)
	}
}

func TestSafeMergeDeep_BlockedNestedKey(t *testing.T) {
	t.Parallel()
	dst := map[string]any{"a": 1}
	src := map[string]any{
		"nested": map[string]any{
			"__proto__": "malicious",
		},
	}

	if err := SafeMergeDeep(dst, src); err == nil {
		t.Error("expected deep merge with blocked nested key to fail")
	}
}

func TestIsBlockedKey(t *testing.T) {
	t.Parallel()
	blocked := []string{"__proto__", "prototype", "constructor"}
	safe := []string{"name", "data", "config", "proto"}

	for _, key := range blocked {
		if !IsBlockedKey(key) {
			t.Errorf("expected %q to be blocked", key)
		}
	}

	for _, key := range safe {
		if IsBlockedKey(key) {
			t.Errorf("expected %q to be safe", key)
		}
	}
}

func TestSafeMergeError_Error(t *testing.T) {
	err := &SafeMergeError{Key: "__proto__"}
	expected := `blocked key detected in merge operation: "__proto__"`
	if err.Error() != expected {
		t.Errorf("expected error message %q, got %q", expected, err.Error())
	}
}
