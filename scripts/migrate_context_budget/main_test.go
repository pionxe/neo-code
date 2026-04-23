package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neo-code/internal/config"
)

func TestDefaultBaseDirReturnsPath(t *testing.T) {
	t.Parallel()

	if got := defaultBaseDir(); got == "" {
		t.Fatal("expected non-empty default base dir")
	}
}

func TestPrintMigrationResultChangedDryRun(t *testing.T) {
	output := captureStdout(t, func() {
		printMigrationResult(config.ContextBudgetMigrationResult{
			Path:    "/tmp/config.yaml",
			Changed: true,
		}, true)
	})

	if !strings.Contains(output, "[DRY-RUN] 将迁移 /tmp/config.yaml") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestPrintMigrationResultChangedWithBackup(t *testing.T) {
	output := captureStdout(t, func() {
		printMigrationResult(config.ContextBudgetMigrationResult{
			Path:    "/tmp/config.yaml",
			Changed: true,
			Backup:  "/tmp/config.yaml.bak",
		}, false)
	})

	if !strings.Contains(output, "已迁移 /tmp/config.yaml (备份: /tmp/config.yaml.bak)") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestPrintMigrationResultNotChangedWithNotes(t *testing.T) {
	output := captureStdout(t, func() {
		printMigrationResult(config.ContextBudgetMigrationResult{
			Path:   "/tmp/config.yaml",
			Reason: "未检测到 context.auto_compact",
			Notes:  []string{" note-a ", "note-b"},
		}, false)
	})

	if !strings.Contains(output, "说明: note-a") {
		t.Fatalf("missing note-a in output: %q", output)
	}
	if !strings.Contains(output, "说明: note-b") {
		t.Fatalf("missing note-b in output: %q", output)
	}
	if !strings.Contains(output, "跳过: /tmp/config.yaml (未检测到 context.auto_compact)") {
		t.Fatalf("missing skip line in output: %q", output)
	}
}

func TestDefaultBaseDirUsesHome(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	want := filepath.Join(tempHome, ".neocode")
	if got := defaultBaseDir(); got != want {
		t.Fatalf("defaultBaseDir() = %q, want %q", got, want)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = originalStdout
	}()

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, reader)
		done <- buf.String()
	}()

	fn()
	_ = writer.Close()
	output := <-done
	_ = reader.Close()
	return output
}
