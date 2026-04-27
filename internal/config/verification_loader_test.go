package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoaderNormalizesLegacyVerificationSchemaInMemory(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	loader := NewLoader(baseDir, testDefaultConfig())
	raw := strings.TrimSpace(`
selected_provider: openai
current_model: gpt-4.1
shell: powershell
runtime:
  verification:
    enabled: false
    final_intercept: false
    default_task_policy: edit_code
    verifiers:
      test:
        enabled: true
        required: true
        command: go test ./...
`) + "\n"
	path := filepath.Join(baseDir, configName)
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Runtime.Verification.Verifiers["test"].Command[0] != "go" {
		t.Fatalf("expected argv command migration, got %+v", cfg.Runtime.Verification.Verifiers["test"].Command)
	}

	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(stored)
	if !strings.Contains(text, "enabled: false") || !strings.Contains(text, "command: go test ./...") {
		t.Fatalf("expected loader to avoid rewriting file, got:\n%s", text)
	}
}

func TestLoaderRejectsUnsafeLegacyVerificationCommandString(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	loader := NewLoader(baseDir, testDefaultConfig())
	raw := strings.TrimSpace(`
selected_provider: openai
current_model: gpt-4.1
shell: powershell
runtime:
  verification:
    verifiers:
      test:
        command: sh -c 'go test ./...'
`) + "\n"
	path := filepath.Join(baseDir, configName)
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := loader.Load(context.Background())
	if err == nil || !strings.Contains(err.Error(), "rewrite it as argv") {
		t.Fatalf("expected unsupported shell syntax error, got %v", err)
	}
}
