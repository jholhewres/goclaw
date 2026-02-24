package copilot

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyPatch(t *testing.T) {
	tmpDir := t.TempDir()

	// Initial files
	err := os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("line 1\nline 2\nline 3\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("old content\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	patch := `*** Begin Patch
*** Add File: new_file.go
+package main
+
+func main() {}
*** Delete File: file2.txt
*** Update File: file1.txt
@@ 
 line 1
-line 2
+new line 2
+another line
 line 3
*** End Patch`

	res, err := applyPatch(context.Background(), patch, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Added) != 1 || res.Added[0] != "new_file.go" {
		t.Errorf("expected 1 added file, got %v", res.Added)
	}
	if len(res.Deleted) != 1 || res.Deleted[0] != "file2.txt" {
		t.Errorf("expected 1 deleted file, got %v", res.Deleted)
	}
	if len(res.Modified) != 1 || res.Modified[0] != "file1.txt" {
		t.Errorf("expected 1 modified file, got %v", res.Modified)
	}

	// Verify add
	content, err := os.ReadFile(filepath.Join(tmpDir, "new_file.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "package main\n\nfunc main() {}\n" {
		t.Errorf("unexpected content in new_file.go: %q", string(content))
	}

	// Verify delete
	if _, err := os.Stat(filepath.Join(tmpDir, "file2.txt")); !os.IsNotExist(err) {
		t.Error("file2.txt should have been deleted")
	}

	// Verify update
	content, err = os.ReadFile(filepath.Join(tmpDir, "file1.txt"))
	if err != nil {
		t.Fatal(err)
	}
	expected := "line 1\nnew line 2\nanother line\nline 3\n"
	if string(content) != expected {
		t.Errorf("unexpected content in file1.txt: got %q, want %q", string(content), expected)
	}
}

func TestApplyPatch_Move(t *testing.T) {
	tmpDir := t.TempDir()

	err := os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("hello world\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	patch := `*** Begin Patch
*** Update File: file1.txt
*** Move to: renamed.txt
@@ 
-hello world
+hello go
*** End Patch`

	res, err := applyPatch(context.Background(), patch, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Modified) != 1 || res.Modified[0] != "renamed.txt" {
		t.Errorf("expected 1 modified file renamed.txt, got %v", res.Modified)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "file1.txt")); !os.IsNotExist(err) {
		t.Error("file1.txt should have been deleted")
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, "renamed.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello go\n" {
		t.Errorf("unexpected content in renamed.txt: %q", string(content))
	}
}

func TestApplyPatch_Invalid(t *testing.T) {
	tmpDir := t.TempDir()

	patch := `*** Add File: oops.txt
+fail
*** End Patch`

	_, err := applyPatch(context.Background(), patch, tmpDir)
	if err == nil || !strings.Contains(err.Error(), "must be '*** Begin Patch'") {
		t.Errorf("expected missing begin marker error, got %v", err)
	}
}

// Security tests for path traversal prevention

func TestApplyPatch_AbsolutePathRejected(t *testing.T) {
	tmpDir := t.TempDir()

	patch := `*** Begin Patch
*** Add File: /etc/malicious.txt
+evil content
*** End Patch`

	_, err := applyPatch(context.Background(), patch, tmpDir)
	if err == nil {
		t.Error("expected error for absolute path, got nil")
	}
	if !strings.Contains(err.Error(), "absolute paths not allowed") {
		t.Errorf("expected absolute path error, got: %v", err)
	}
}

func TestApplyPatch_PathTraversalRejected(t *testing.T) {
	tmpDir := t.TempDir()

	patch := `*** Begin Patch
*** Add File: ../../../etc/passwd
+stolen
*** End Patch`

	_, err := applyPatch(context.Background(), patch, tmpDir)
	if err == nil {
		t.Error("expected error for path traversal, got nil")
	}
	if !strings.Contains(err.Error(), "escapes workspace") {
		t.Errorf("expected path escapes error, got: %v", err)
	}
}

func TestApplyPatch_DeletePathTraversalRejected(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file to ensure the patch tries to delete
	err := os.WriteFile(filepath.Join(tmpDir, "safe.txt"), []byte("safe"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	patch := `*** Begin Patch
*** Delete File: ../../safe.txt
*** End Patch`

	_, err = applyPatch(context.Background(), patch, tmpDir)
	if err == nil {
		t.Error("expected error for path traversal in delete, got nil")
	}
	if !strings.Contains(err.Error(), "escapes workspace") {
		t.Errorf("expected path escapes error, got: %v", err)
	}
}

func TestApplyPatch_MovePathTraversalRejected(t *testing.T) {
	tmpDir := t.TempDir()

	err := os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("content\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	patch := `*** Begin Patch
*** Update File: file.txt
*** Move to: ../../malicious.txt
@@
-content
+evil
*** End Patch`

	_, err = applyPatch(context.Background(), patch, tmpDir)
	if err == nil {
		t.Error("expected error for path traversal in move, got nil")
	}
	if !strings.Contains(err.Error(), "escapes workspace") {
		t.Errorf("expected path escapes error, got: %v", err)
	}
}
