package infra

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"unicode/utf16"

	"neo-code/internal/config"
)

func TestDefaultWorkspaceCommandExecutor(t *testing.T) {
	workdir := t.TempDir()
	cfg := config.Config{
		Workdir:        workdir,
		ToolTimeoutSec: 15,
	}

	command := "pwd"
	if goruntime.GOOS == "windows" {
		cfg.Shell = "powershell"
		command = "$PWD.Path"
	} else {
		cfg.Shell = "sh"
	}

	output, err := DefaultWorkspaceCommandExecutor(context.Background(), cfg, "", command)
	if err != nil {
		t.Fatalf("DefaultWorkspaceCommandExecutor() error = %v", err)
	}
	normalizedOutput := strings.ToLower(filepath.Clean(strings.TrimSpace(output)))
	normalizedWorkdir := strings.ToLower(filepath.Clean(workdir))
	if !strings.Contains(normalizedOutput, normalizedWorkdir) {
		t.Fatalf("expected output %q to contain resolved workdir %q", output, workdir)
	}

	// Empty command rejected.
	if _, err := DefaultWorkspaceCommandExecutor(context.Background(), cfg, "", "   "); err == nil {
		t.Fatalf("expected empty command error")
	}

	// Default timeout used when ToolTimeoutSec <= 0.
	cfg.ToolTimeoutSec = 0
	output, err = DefaultWorkspaceCommandExecutor(context.Background(), cfg, "", command)
	if err != nil {
		t.Fatalf("DefaultWorkspaceCommandExecutor() with default timeout error = %v", err)
	}
	if strings.TrimSpace(output) == "" {
		t.Fatalf("expected non-empty output with default timeout")
	}
}

func TestDefaultWorkspaceCommandExecutorUsesDefaultTimeout(t *testing.T) {
	workdir := t.TempDir()
	cfg := config.Config{
		Workdir:        workdir,
		ToolTimeoutSec: 0,
	}
	if goruntime.GOOS == "windows" {
		cfg.Shell = "powershell"
	} else {
		cfg.Shell = "sh"
	}

	command := "echo hello"
	if goruntime.GOOS == "windows" {
		command = "Write-Output hello"
	}

	output, err := DefaultWorkspaceCommandExecutor(context.Background(), cfg, "", command)
	if err != nil {
		t.Fatalf("DefaultWorkspaceCommandExecutor() error = %v", err)
	}
	if !strings.Contains(strings.ToLower(output), "hello") {
		t.Fatalf("expected output to contain hello, got %q", output)
	}
}

func TestShellArgs(t *testing.T) {
	if got := ShellArgs("bash", "pwd"); len(got) != 3 || got[0] != "bash" || got[2] != "pwd" {
		t.Fatalf("unexpected bash args: %+v", got)
	}
	if got := ShellArgs("sh", "pwd"); len(got) != 3 || got[0] != "sh" || got[2] != "pwd" {
		t.Fatalf("unexpected sh args: %+v", got)
	}
	if got := ShellArgs("unknown", "git status"); len(got) != 4 || got[0] != "powershell" {
		t.Fatalf("expected powershell fallback, got %+v", got)
	}
}

func TestSanitizeWorkspaceOutput(t *testing.T) {
	raw := []byte("\x1b[31mERROR\x1b[0m\r\nok\t\x00")
	got := SanitizeWorkspaceOutput(raw)
	if strings.Contains(got, "\x1b[31m") {
		t.Fatalf("expected ansi removed, got %q", got)
	}
	if !strings.Contains(got, "ERROR") || !strings.Contains(got, "ok") {
		t.Fatalf("expected content preserved, got %q", got)
	}
}

func TestDecodeWorkspaceOutputUTF16LE(t *testing.T) {
	utf16Data := utf16.Encode([]rune("PowerShell 输出"))
	buf := make([]byte, 2+len(utf16Data)*2)
	buf[0], buf[1] = 0xFF, 0xFE
	for i, word := range utf16Data {
		binary.LittleEndian.PutUint16(buf[2+i*2:], word)
	}

	got := DecodeWorkspaceOutput(buf)
	if !strings.Contains(got, "PowerShell") {
		t.Fatalf("expected decoded utf16 content, got %q", got)
	}
}

func TestCollectWorkspaceFiles(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(rel string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(rel), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	mustWrite("README.md")
	mustWrite("internal/tui/update.go")
	mustWrite(".git/config")
	mustWrite("node_modules/skip.js")

	files, err := CollectWorkspaceFiles(root, 10)
	if err != nil {
		t.Fatalf("CollectWorkspaceFiles() error = %v", err)
	}
	got := strings.Join(files, ",")
	if strings.Contains(got, ".git") || strings.Contains(got, "node_modules") {
		t.Fatalf("expected ignored dirs skipped, got %v", files)
	}
	if !strings.Contains(got, "README.md") || !strings.Contains(got, "internal/tui/update.go") {
		t.Fatalf("expected workspace files included, got %v", files)
	}
}

func TestCopyTextUsesInjectedWriter(t *testing.T) {
	original := clipboardWriteAll
	t.Cleanup(func() { clipboardWriteAll = original })

	captured := ""
	clipboardWriteAll = func(text string) error {
		captured = text
		return nil
	}
	if err := CopyText("hello"); err != nil {
		t.Fatalf("CopyText() error = %v", err)
	}
	if captured != "hello" {
		t.Fatalf("expected captured clipboard text, got %q", captured)
	}
}

func TestCachedMarkdownRendererBasic(t *testing.T) {
	renderer := NewCachedMarkdownRenderer("dark", 4, "(empty)")

	empty, err := renderer.Render(" \n\t ", 20)
	if err != nil {
		t.Fatalf("Render(empty) error = %v", err)
	}
	if empty != "(empty)" {
		t.Fatalf("expected empty placeholder, got %q", empty)
	}

	out, err := renderer.Render("# Title\n\n- one", 40)
	if err != nil {
		t.Fatalf("Render(markdown) error = %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatalf("expected non-empty rendered markdown")
	}
	if renderer.RendererCount() != 1 || renderer.CacheCount() != 1 {
		t.Fatalf("expected renderer and cache entries, got renderers=%d cache=%d", renderer.RendererCount(), renderer.CacheCount())
	}
}

func TestCachedMarkdownRendererCacheEviction(t *testing.T) {
	renderer := NewCachedMarkdownRenderer("dark", 1, "(empty)")

	if _, err := renderer.Render("first", 20); err != nil {
		t.Fatalf("Render(first) error = %v", err)
	}
	if _, err := renderer.Render("second", 20); err != nil {
		t.Fatalf("Render(second) error = %v", err)
	}
	if renderer.CacheOrderCount() != 1 || renderer.CacheCount() != 1 {
		t.Fatalf("expected single cache entry after eviction, got order=%d cache=%d", renderer.CacheOrderCount(), renderer.CacheCount())
	}
}

func TestCachedMarkdownRendererEdgeCases(t *testing.T) {
	// Zero max entries means no caching.
	renderer := NewCachedMarkdownRenderer("dark", 0, "(empty)")
	if _, err := renderer.Render("# test", 20); err != nil {
		t.Fatalf("Render error = %v", err)
	}
	if renderer.CacheCount() != 0 {
		t.Fatalf("expected no cache entries with max=0, got %d", renderer.CacheCount())
	}

	// Negative max entries clamped to 0.
	renderer2 := NewCachedMarkdownRenderer("dark", -5, "(empty)")
	if _, err := renderer2.Render("# test", 20); err != nil {
		t.Fatalf("Render error = %v", err)
	}
	if renderer2.CacheCount() != 0 {
		t.Fatalf("expected no cache entries with max=-5, got %d", renderer2.CacheCount())
	}

	// Cache hit returns cached value.
	renderer3 := NewCachedMarkdownRenderer("dark", 4, "(empty)")
	out1, _ := renderer3.Render("# hello", 30)
	out2, _ := renderer3.Render("# hello", 30)
	if out1 != out2 {
		t.Fatalf("expected cache hit to return same result")
	}
	if renderer3.RendererCount() != 1 {
		t.Fatalf("expected only one render call, got %d", renderer3.RendererCount())
	}

	// SetMaxCacheEntries shrinks and evicts.
	renderer4 := NewCachedMarkdownRenderer("dark", 10, "(empty)")
	for i := 0; i < 5; i++ {
		_, _ = renderer4.Render("item"+string(rune('a'+i)), 20)
	}
	if renderer4.CacheCount() != 5 {
		t.Fatalf("expected 5 entries, got %d", renderer4.CacheCount())
	}
	renderer4.SetMaxCacheEntries(2)
	if renderer4.CacheCount() != 2 {
		t.Fatalf("expected 2 entries after shrink, got %d", renderer4.CacheCount())
	}
}

func TestDecodeWorkspaceOutputEdgeCases(t *testing.T) {
	// Empty input returns empty.
	if got := DecodeWorkspaceOutput(nil); got != "" {
		t.Fatalf("expected empty for nil input, got %q", got)
	}
	if got := DecodeWorkspaceOutput([]byte{}); got != "" {
		t.Fatalf("expected empty for empty slice, got %q", got)
	}

	// UTF-16 BE BOM.
	utf16Data := utf16.Encode([]rune("hello"))
	buf := make([]byte, 2+len(utf16Data)*2)
	buf[0], buf[1] = 0xFE, 0xFF
	for i, word := range utf16Data {
		buf[2+i*2] = byte(word >> 8)
		buf[2+i*2+1] = byte(word & 0xFF)
	}
	if got := DecodeWorkspaceOutput(buf); !strings.Contains(got, "hello") {
		t.Fatalf("expected BE BOM decode, got %q", got)
	}

	// Odd-length raw bytes falls back to string.
	if got := DecodeWorkspaceOutput([]byte{0x61, 0x62, 0x63}); got != "abc" {
		t.Fatalf("expected odd-length fallback to string, got %q", got)
	}
}

func TestDecodeUTF16EdgeCases(t *testing.T) {
	if got := decodeUTF16(nil, true); got != "" {
		t.Fatalf("expected empty for nil, got %q", got)
	}
	if got := decodeUTF16([]byte{0x61}, true); got != "a" {
		t.Fatalf("expected single byte handling, got %q", got)
	}
}

func TestSanitizeWorkspaceOutputEdgeCases(t *testing.T) {
	// Empty input.
	if got := SanitizeWorkspaceOutput(nil); got != "" {
		t.Fatalf("expected empty for nil, got %q", got)
	}

	// \r-only line endings.
	if got := SanitizeWorkspaceOutput([]byte("a\r\rb")); !strings.Contains(got, "a") {
		t.Fatalf("expected content preserved with \\r, got %q", got)
	}

	// Control characters below 0x20 (except \n and \t) are stripped.
	got := SanitizeWorkspaceOutput([]byte("hello\x01world"))
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
		t.Fatalf("expected control chars removed but content preserved, got %q", got)
	}
	if strings.Contains(got, "\x01") {
		t.Fatalf("expected \\x01 stripped, got %q", got)
	}
}

func TestShellArgsPowerShell(t *testing.T) {
	args := ShellArgs("powershell", "echo hi")
	if len(args) != 4 || args[0] != "powershell" || args[1] != "-NoProfile" {
		t.Fatalf("unexpected powershell args: %+v", args)
	}
	args = ShellArgs("pwsh", "echo hi")
	if len(args) != 4 || args[0] != "powershell" {
		t.Fatalf("unexpected pwsh args: %+v", args)
	}
}

func TestPowerShellUTF8Command(t *testing.T) {
	cmd := PowerShellUTF8Command("echo hi")
	if !strings.Contains(cmd, "chcp 65001") || !strings.Contains(cmd, "echo hi") {
		t.Fatalf("unexpected powershell UTF-8 command: %q", cmd)
	}
}

func TestDecodedTextScore(t *testing.T) {
	if got := decodedTextScore(""); got != 0 {
		t.Fatalf("expected 0 for empty, got %d", got)
	}
	if got := decodedTextScore("ab"); got <= 0 {
		t.Fatalf("expected positive score for printable, got %d", got)
	}
	if got := decodedTextScore("\ufffd"); got >= 0 {
		t.Fatalf("expected negative score for replacement char, got %d", got)
	}
}

func TestCollectWorkspaceFilesEdgeCases(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(rel string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(rel), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	mustWrite(".gocache/test.go")
	mustWrite("src/main.go")

	// .gocache should be skipped.
	files, _ := CollectWorkspaceFiles(root, 10)
	got := strings.Join(files, ",")
	if strings.Contains(got, ".gocache") {
		t.Fatalf("expected .gocache skipped, got %v", files)
	}

	// Zero limit means no cap.
	mustWrite("a.txt")
	mustWrite("b.txt")
	files, _ = CollectWorkspaceFiles(root, 0)
	if len(files) < 3 {
		t.Fatalf("expected no cap with limit=0, got %d files", len(files))
	}
}

func TestNewGlamourTermRenderer(t *testing.T) {
	r, err := NewGlamourTermRenderer("dark", 80)
	if err != nil {
		t.Fatalf("NewGlamourTermRenderer() error = %v", err)
	}
	if r == nil {
		t.Fatalf("expected non-nil renderer")
	}
}

func TestClipboardError(t *testing.T) {
	original := clipboardWriteAll
	t.Cleanup(func() { clipboardWriteAll = original })

	clipboardWriteAll = func(text string) error {
		return os.ErrPermission
	}
	if err := CopyText("hello"); err == nil {
		t.Fatalf("expected error from clipboard write")
	}
}
