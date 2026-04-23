package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neo-code/internal/config"
)

func TestMigrateContextBudgetCommandDryRunSkipsGlobalHooks(t *testing.T) {
	originalPreload := runGlobalPreload
	originalSilentCheck := runSilentUpdateCheck
	t.Cleanup(func() {
		runGlobalPreload = originalPreload
		runSilentUpdateCheck = originalSilentCheck
	})

	runGlobalPreload = func(context.Context) error {
		t.Fatal("migrate command must not run global preload")
		return nil
	}
	runSilentUpdateCheck = func(context.Context) {
		t.Fatal("migrate command must not run silent update check")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "config.yaml")
	original := "context:\n  auto_compact:\n    input_token_threshold: 120000\n"
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"migrate", "context-budget", "--config", target, "--dry-run"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "[DRY-RUN]") {
		t.Fatalf("expected dry-run output, got %q", stdout.String())
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(content) != original {
		t.Fatalf("dry-run mutated config:\n%s", content)
	}
}

func TestMigrateContextBudgetCommandWritesBackup(t *testing.T) {
	originalPreload := runGlobalPreload
	originalSilentCheck := runSilentUpdateCheck
	t.Cleanup(func() {
		runGlobalPreload = originalPreload
		runSilentUpdateCheck = originalSilentCheck
	})
	runGlobalPreload = func(context.Context) error { return nil }
	runSilentUpdateCheck = func(context.Context) {}

	dir := t.TempDir()
	target := filepath.Join(dir, "config.yaml")
	original := "context:\n  auto_compact:\n    reserve_tokens: 13000\n"
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"migrate", "context-budget", "--config", target})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "已迁移") {
		t.Fatalf("expected migrated output, got %q", stdout.String())
	}

	backup, err := os.ReadFile(target + ".bak")
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backup) != original {
		t.Fatalf("backup content mismatch:\n%s", backup)
	}
	migrated, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read migrated config: %v", err)
	}
	if strings.Contains(string(migrated), "auto_compact") || !strings.Contains(string(migrated), "budget:") {
		t.Fatalf("unexpected migrated config:\n%s", migrated)
	}
}

func TestMigrateContextBudgetCommandPrintsMigrationNotes(t *testing.T) {
	originalPreload := runGlobalPreload
	originalSilentCheck := runSilentUpdateCheck
	t.Cleanup(func() {
		runGlobalPreload = originalPreload
		runSilentUpdateCheck = originalSilentCheck
	})
	runGlobalPreload = func(context.Context) error { return nil }
	runSilentUpdateCheck = func(context.Context) {}

	dir := t.TempDir()
	target := filepath.Join(dir, "config.yaml")
	original := "context:\n  auto_compact:\n    enabled: false\n    reserve_tokens: 13000\n"
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"migrate", "context-budget", "--config", target})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "说明: "+config.ContextBudgetMigrationNoteEnabledDeprecated) {
		t.Fatalf("expected migration note in output, got %q", out)
	}
}
