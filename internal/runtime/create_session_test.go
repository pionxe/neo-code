package runtime

import (
	"context"
	"fmt"
	"testing"

	agentsession "neo-code/internal/session"
)

type createSessionUpsertStore struct {
	*memoryStore
	missingErr error
}

func (s *createSessionUpsertStore) LoadSession(ctx context.Context, id string) (agentsession.Session, error) {
	if err := ctx.Err(); err != nil {
		return agentsession.Session{}, err
	}
	s.memoryStore.mu.Lock()
	_, exists := s.memoryStore.sessions[id]
	s.memoryStore.mu.Unlock()
	if !exists {
		return agentsession.Session{}, s.missingErr
	}
	return s.memoryStore.LoadSession(ctx, id)
}

func TestServiceCreateSessionUpsertWhenMissing(t *testing.T) {
	t.Parallel()

	store := &createSessionUpsertStore{
		memoryStore: newMemoryStore(),
		missingErr:  fmt.Errorf("load session row: %w", agentsession.ErrSessionNotFound),
	}
	service := &Service{
		configManager: newRuntimeConfigManager(t),
		sessionStore:  store,
	}

	created, err := service.CreateSession(context.Background(), "session-upsert")
	if err != nil {
		t.Fatalf("CreateSession() upsert error = %v", err)
	}
	if created.ID != "session-upsert" {
		t.Fatalf("created session id = %q, want %q", created.ID, "session-upsert")
	}
	if created.Title != "New Session" {
		t.Fatalf("created session title = %q, want %q", created.Title, "New Session")
	}

	savesAfterCreate := store.memoryStore.saves
	loaded, err := service.CreateSession(context.Background(), "session-upsert")
	if err != nil {
		t.Fatalf("CreateSession() load existing error = %v", err)
	}
	if loaded.ID != "session-upsert" {
		t.Fatalf("loaded session id = %q, want %q", loaded.ID, "session-upsert")
	}
	if store.memoryStore.saves != savesAfterCreate {
		t.Fatalf("unexpected additional create, saves=%d want %d", store.memoryStore.saves, savesAfterCreate)
	}
}

func TestServiceCreateSessionReturnsOriginalErrorWhenMissingErrorIsNotSentinel(t *testing.T) {
	t.Parallel()

	store := &createSessionUpsertStore{
		memoryStore: newMemoryStore(),
		missingErr:  fmt.Errorf("dependency not found"),
	}
	service := &Service{
		configManager: newRuntimeConfigManager(t),
		sessionStore:  store,
	}

	_, err := service.CreateSession(context.Background(), "session-upsert")
	if err == nil {
		t.Fatalf("CreateSession() expected error when missing error is not sentinel")
	}
	if err.Error() != "dependency not found" {
		t.Fatalf("CreateSession() error = %v, want dependency not found", err)
	}
	if store.memoryStore.saves != 0 {
		t.Fatalf("CreateSession() should not create on non-sentinel error, saves=%d", store.memoryStore.saves)
	}
}
