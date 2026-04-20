package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"neo-code/internal/config"
)

func newSessionLogTestService(t *testing.T) *Service {
	t.Helper()
	cfg := config.StaticDefaults()
	cfg.Workdir = t.TempDir()
	manager := config.NewManager(config.NewLoader(cfg.Workdir, cfg))
	if _, err := manager.Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return &Service{configManager: manager}
}

func TestSessionLogEntriesPathAndSanitizePrefix(t *testing.T) {
	service := newSessionLogTestService(t)

	pathA, err := service.sessionLogEntriesPath("a:b")
	if err != nil || pathA == "" {
		t.Fatalf("sessionLogEntriesPath(a:b) err=%v path=%q", err, pathA)
	}
	pathB, err := service.sessionLogEntriesPath("a/b")
	if err != nil || pathB == "" {
		t.Fatalf("sessionLogEntriesPath(a/b) err=%v path=%q", err, pathB)
	}
	if pathA == pathB {
		t.Fatalf("expected different file names for potential sanitize collision ids, got %q", pathA)
	}
	if got := sanitizeSessionLogPrefix(" /a:b?c* "); got == "" {
		t.Fatal("expected sanitizeSessionLogPrefix to produce fallback prefix")
	}
	if got := sanitizeSessionLogPrefix("___"); got != "session" {
		t.Fatalf("sanitizeSessionLogPrefix(___)=%q, want session", got)
	}
}

func TestLoadAndSaveSessionLogEntries(t *testing.T) {
	service := newSessionLogTestService(t)
	sessionID := "session-one"
	source := []SessionLogEntry{
		{Timestamp: time.Unix(1700000000, 0), Level: "info", Source: "tool", Message: "ok"},
	}

	if err := service.SaveSessionLogEntries(context.Background(), sessionID, source); err != nil {
		t.Fatalf("SaveSessionLogEntries() error = %v", err)
	}
	loaded, err := service.LoadSessionLogEntries(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("LoadSessionLogEntries() error = %v", err)
	}
	if len(loaded) != 1 || loaded[0].Message != "ok" {
		t.Fatalf("unexpected loaded entries: %+v", loaded)
	}

	missing, err := service.LoadSessionLogEntries(context.Background(), "missing-session")
	if err != nil {
		t.Fatalf("LoadSessionLogEntries(missing) error = %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("expected missing session to return empty entries, got %+v", missing)
	}
}

func TestSessionLogEntriesErrorBranches(t *testing.T) {
	service := newSessionLogTestService(t)

	if err := service.SaveSessionLogEntries(context.Background(), "", nil); err != nil {
		t.Fatalf("SaveSessionLogEntries(blank) should skip, got err=%v", err)
	}
	if _, err := service.LoadSessionLogEntries(context.Background(), ""); err != nil {
		t.Fatalf("LoadSessionLogEntries(blank) should skip, got err=%v", err)
	}

	path, err := service.sessionLogEntriesPath("bad-json")
	if err != nil {
		t.Fatalf("sessionLogEntriesPath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("{invalid"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := service.LoadSessionLogEntries(context.Background(), "bad-json"); err == nil {
		t.Fatal("expected invalid json load error")
	}

	brokenService := &Service{configManager: nil}
	if err := brokenService.SaveSessionLogEntries(context.Background(), "id", nil); err == nil {
		t.Fatal("expected save error when config manager is nil")
	}
	if _, err := brokenService.LoadSessionLogEntries(context.Background(), "id"); err == nil {
		t.Fatal("expected load error when config manager is nil")
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.SaveSessionLogEntries(cancelled, "id", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled error on save, got %v", err)
	}
	if _, err := service.LoadSessionLogEntries(cancelled, "id"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled error on load, got %v", err)
	}
}
