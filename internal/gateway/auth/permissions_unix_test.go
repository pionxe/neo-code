//go:build !windows

package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyAuthPermissions(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(file, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	if err := applyAuthDirPermission(dir); err != nil {
		t.Fatalf("applyAuthDirPermission() error = %v", err)
	}
	if err := applyAuthFilePermission(file); err != nil {
		t.Fatalf("applyAuthFilePermission() error = %v", err)
	}
}

func TestApplyAuthPermissionsErrorBranches(t *testing.T) {
	if err := applyAuthDirPermission(filepath.Join(t.TempDir(), "missing-dir")); err == nil {
		t.Fatal("expected chmod missing dir error")
	}
	if err := applyAuthFilePermission(filepath.Join(t.TempDir(), "missing-file")); err == nil {
		t.Fatal("expected chmod missing file error")
	}
}
