package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEffectiveWorkdir(t *testing.T) {
	tests := []struct {
		name           string
		sessionWorkdir string
		defaultWorkdir string
		want           string
	}{
		{"prefer session workdir", "/session", "/default", "/session"},
		{"fallback to default", "", "/default", "/default"},
		{"both empty", "", "", ""},
		{"session with whitespace", "  /session  ", "/default", "/session"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EffectiveWorkdir(tt.sessionWorkdir, tt.defaultWorkdir); got != tt.want {
				t.Errorf("EffectiveWorkdir() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveExistingDir(t *testing.T) {
	t.Run("resolves absolute directory", func(t *testing.T) {
		dir := t.TempDir()
		got, err := ResolveExistingDir(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != filepath.Clean(dir) {
			t.Fatalf("expected %q, got %q", filepath.Clean(dir), got)
		}
	})

	t.Run("resolves relative directory", func(t *testing.T) {
		base := t.TempDir()
		sub := filepath.Join(base, "sub")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		got, err := ResolveExistingDir(sub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != filepath.Clean(sub) {
			t.Fatalf("expected %q, got %q", filepath.Clean(sub), got)
		}
	})

	t.Run("rejects empty", func(t *testing.T) {
		_, err := ResolveExistingDir("")
		if err == nil || !strings.Contains(err.Error(), "workdir is empty") {
			t.Fatalf("expected empty error, got %v", err)
		}
	})

	t.Run("rejects whitespace", func(t *testing.T) {
		_, err := ResolveExistingDir("   ")
		if err == nil || !strings.Contains(err.Error(), "workdir is empty") {
			t.Fatalf("expected empty error, got %v", err)
		}
	})

	t.Run("rejects non-existent path", func(t *testing.T) {
		_, err := ResolveExistingDir(filepath.Join(t.TempDir(), "missing"))
		if err == nil {
			t.Fatalf("expected error for missing path")
		}
	})

	t.Run("rejects file path", func(t *testing.T) {
		filePath := filepath.Join(t.TempDir(), "note.txt")
		if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		_, err := ResolveExistingDir(filePath)
		if err == nil || !strings.Contains(err.Error(), "is not a directory") {
			t.Fatalf("expected not-a-directory error, got %v", err)
		}
	})
}
