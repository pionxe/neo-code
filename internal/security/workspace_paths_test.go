package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWorkspacePathResolvesInsideWorkspace(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	targetDir := filepath.Join(root, "pkg")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	resolvedRoot, resolvedTarget, err := ResolveWorkspacePath(root, "pkg")
	if err != nil {
		t.Fatalf("ResolveWorkspacePath() error = %v", err)
	}
	if !samePathKey(resolvedRoot, root) {
		t.Fatalf("expected resolved root inside workspace, got %q", resolvedRoot)
	}
	if !samePathKey(resolvedTarget, targetDir) {
		t.Fatalf("expected resolved target %q, got %q", targetDir, resolvedTarget)
	}
}

func TestResolveWorkspacePathRejectsTraversal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if _, _, err := ResolveWorkspacePath(root, "..\\outside.txt"); err == nil {
		t.Fatalf("expected traversal path to be rejected")
	}
}

func TestResolveWorkspacePathRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	linkDir := filepath.Join(root, "pkg")
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	linkPath := filepath.Join(linkDir, "secret.txt")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Skipf("symlink not available: %v", err)
	}

	if _, _, err := ResolveWorkspacePath(root, "pkg/secret.txt"); err == nil {
		t.Fatalf("expected symlink escape to be rejected")
	}
}
