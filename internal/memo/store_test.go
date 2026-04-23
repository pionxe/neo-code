package memo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentsession "neo-code/internal/session"
)

func TestNewFileStore(t *testing.T) {
	tmp := t.TempDir()
	store := NewFileStore(tmp, "/workspace/project")
	if store == nil {
		t.Fatal("NewFileStore() returned nil")
	}
	if store.baseDir != tmp {
		t.Fatalf("baseDir = %q, want %q", store.baseDir, tmp)
	}
	if store.workspaceRoot != "/workspace/project" {
		t.Fatalf("workspaceRoot = %q, want %q", store.workspaceRoot, "/workspace/project")
	}
}

func TestFileStoreSaveAndLoadIndexByScope(t *testing.T) {
	store := NewFileStore(t.TempDir(), "/workspace/project")
	index := &Index{
		Entries: []Entry{{
			ID:        "user_001",
			Type:      TypeUser,
			Title:     "user pref",
			Content:   "content",
			Keywords:  []string{"tabs"},
			Source:    SourceUserManual,
			TopicFile: "user.md",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}},
		UpdatedAt: time.Now(),
	}

	if err := store.SaveIndex(context.Background(), ScopeUser, index); err != nil {
		t.Fatalf("SaveIndex() error = %v", err)
	}
	loaded, err := store.LoadIndex(context.Background(), ScopeUser)
	if err != nil {
		t.Fatalf("LoadIndex() error = %v", err)
	}
	if len(loaded.Entries) != 1 || loaded.Entries[0].Title != "user pref" {
		t.Fatalf("loaded entries = %#v", loaded.Entries)
	}
}

func TestFileStoreLoadIndexReturnsClonedCacheContent(t *testing.T) {
	store := NewFileStore(t.TempDir(), "/workspace/project")
	if err := store.SaveIndex(context.Background(), ScopeUser, &Index{
		Entries: []Entry{{Type: TypeUser, Title: "original", Content: "body", Keywords: []string{"go", "tabs"}}},
	}); err != nil {
		t.Fatalf("SaveIndex() error = %v", err)
	}

	first, err := store.LoadIndex(context.Background(), ScopeUser)
	if err != nil {
		t.Fatalf("LoadIndex(first) error = %v", err)
	}
	first.Entries[0].Title = "mutated in memory"
	first.Entries[0].Keywords[0] = "mutated-keyword"

	second, err := store.LoadIndex(context.Background(), ScopeUser)
	if err != nil {
		t.Fatalf("LoadIndex(second) error = %v", err)
	}
	if got := second.Entries[0].Title; got != "original" {
		t.Fatalf("cached title = %q, want %q", got, "original")
	}
	if got := second.Entries[0].Keywords[0]; got != "go" {
		t.Fatalf("cached keyword = %q, want %q", got, "go")
	}
}

func TestFileStoreSaveIndexCachesCloneOfInput(t *testing.T) {
	store := NewFileStore(t.TempDir(), "/workspace/project")
	index := &Index{
		Entries: []Entry{{Type: TypeUser, Title: "saved", Content: "body", Keywords: []string{"persisted"}}},
	}
	if err := store.SaveIndex(context.Background(), ScopeUser, index); err != nil {
		t.Fatalf("SaveIndex() error = %v", err)
	}

	index.Entries[0].Title = "changed after save"
	index.Entries[0].Keywords[0] = "changed-keyword"

	loaded, err := store.LoadIndex(context.Background(), ScopeUser)
	if err != nil {
		t.Fatalf("LoadIndex() error = %v", err)
	}
	if got := loaded.Entries[0].Title; got != "saved" {
		t.Fatalf("loaded title = %q, want %q", got, "saved")
	}
	if got := loaded.Entries[0].Keywords[0]; got != "persisted" {
		t.Fatalf("loaded keyword = %q, want %q", got, "persisted")
	}
}

func TestFileStoreLoadIndexRefreshesCacheWhenMemoFileMtimeChanges(t *testing.T) {
	store := NewFileStore(t.TempDir(), "/workspace/project")
	if err := store.SaveIndex(context.Background(), ScopeUser, &Index{
		Entries: []Entry{{Type: TypeUser, Title: "first", Content: "body"}},
	}); err != nil {
		t.Fatalf("SaveIndex() error = %v", err)
	}

	first, err := store.LoadIndex(context.Background(), ScopeUser)
	if err != nil {
		t.Fatalf("LoadIndex(first) error = %v", err)
	}
	if len(first.Entries) != 1 || first.Entries[0].Title != "first" {
		t.Fatalf("first load entries = %#v", first.Entries)
	}

	indexPath := filepath.Join(store.scopeDir(ScopeUser), memoFileName)
	updated := &Index{Entries: []Entry{{Type: TypeUser, Title: "second", Content: "updated"}}}
	if err := os.WriteFile(indexPath, []byte(RenderIndex(updated)), 0o644); err != nil {
		t.Fatalf("WriteFile(updated index) error = %v", err)
	}
	newModTime := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(indexPath, newModTime, newModTime); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}

	second, err := store.LoadIndex(context.Background(), ScopeUser)
	if err != nil {
		t.Fatalf("LoadIndex(second) error = %v", err)
	}
	if len(second.Entries) != 1 || second.Entries[0].Title != "second" {
		t.Fatalf("second load entries = %#v, want refreshed disk content", second.Entries)
	}
}

func TestFileStoreSaveAndLoadTopicByScope(t *testing.T) {
	store := NewFileStore(t.TempDir(), "/workspace/project")
	content := "---\ntype: user\n---\n\nbody\n"

	if err := store.SaveTopic(context.Background(), ScopeUser, "user.md", content); err != nil {
		t.Fatalf("SaveTopic() error = %v", err)
	}
	loaded, err := store.LoadTopic(context.Background(), ScopeUser, "user.md")
	if err != nil {
		t.Fatalf("LoadTopic() error = %v", err)
	}
	if loaded != content {
		t.Fatalf("LoadTopic() = %q, want %q", loaded, content)
	}
}

func TestFileStoreDeleteTopic(t *testing.T) {
	store := NewFileStore(t.TempDir(), "/workspace/project")

	if err := store.SaveTopic(context.Background(), ScopeProject, "p.md", "content"); err != nil {
		t.Fatalf("SaveTopic() error = %v", err)
	}
	if err := store.DeleteTopic(context.Background(), ScopeProject, "p.md"); err != nil {
		t.Fatalf("DeleteTopic() error = %v", err)
	}
	if _, err := store.LoadTopic(context.Background(), ScopeProject, "p.md"); err == nil {
		t.Fatal("expected deleted topic to be missing")
	}
}

func TestFileStoreListTopics(t *testing.T) {
	store := NewFileStore(t.TempDir(), "/workspace/project")

	if err := store.SaveTopic(context.Background(), ScopeProject, "a.md", "a"); err != nil {
		t.Fatalf("SaveTopic(a) error = %v", err)
	}
	if err := store.SaveTopic(context.Background(), ScopeProject, "b.md", "b"); err != nil {
		t.Fatalf("SaveTopic(b) error = %v", err)
	}

	topics, err := store.ListTopics(context.Background(), ScopeProject)
	if err != nil {
		t.Fatalf("ListTopics() error = %v", err)
	}
	if len(topics) != 2 {
		t.Fatalf("len(topics) = %d, want 2", len(topics))
	}
}

func TestFileStoreUserScopeIsGlobal(t *testing.T) {
	tmp := t.TempDir()
	storeA := NewFileStore(tmp, "/workspace/a")
	storeB := NewFileStore(tmp, "/workspace/b")

	if err := storeA.SaveIndex(context.Background(), ScopeUser, &Index{Entries: []Entry{{Type: TypeUser, Title: "A"}}}); err != nil {
		t.Fatalf("SaveIndex() error = %v", err)
	}
	index, err := storeB.LoadIndex(context.Background(), ScopeUser)
	if err != nil {
		t.Fatalf("LoadIndex() error = %v", err)
	}
	if len(index.Entries) != 1 || index.Entries[0].Title != "A" {
		t.Fatalf("global user scope failed, got %#v", index.Entries)
	}
}

func TestFileStoreProjectScopeIsWorkspaceIsolated(t *testing.T) {
	tmp := t.TempDir()
	storeA := NewFileStore(tmp, "/workspace/a")
	storeB := NewFileStore(tmp, "/workspace/b")

	if err := storeA.SaveIndex(context.Background(), ScopeProject, &Index{Entries: []Entry{{Type: TypeProject, Title: "A"}}}); err != nil {
		t.Fatalf("SaveIndex() error = %v", err)
	}
	index, err := storeB.LoadIndex(context.Background(), ScopeProject)
	if err != nil {
		t.Fatalf("LoadIndex() error = %v", err)
	}
	if len(index.Entries) != 0 {
		t.Fatalf("workspace isolation failed, got %#v", index.Entries)
	}
}

func TestFileStoreRejectsUnsupportedScope(t *testing.T) {
	store := NewFileStore(t.TempDir(), "/workspace/project")
	if _, err := store.LoadIndex(context.Background(), ScopeAll); err == nil {
		t.Fatal("expected ScopeAll load to fail")
	}
}

func TestFileStoreCancelContext(t *testing.T) {
	store := NewFileStore(t.TempDir(), "/workspace/project")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := store.LoadIndex(ctx, ScopeUser); err == nil {
		t.Fatal("expected LoadIndex() to fail on canceled context")
	}
	if err := store.SaveIndex(ctx, ScopeUser, &Index{}); err == nil {
		t.Fatal("expected SaveIndex() to fail on canceled context")
	}
	if _, err := store.LoadTopic(ctx, ScopeUser, "x.md"); err == nil {
		t.Fatal("expected LoadTopic() to fail on canceled context")
	}
	if err := store.SaveTopic(ctx, ScopeUser, "x.md", "body"); err == nil {
		t.Fatal("expected SaveTopic() to fail on canceled context")
	}
	if err := store.DeleteTopic(ctx, ScopeUser, "x.md"); err == nil {
		t.Fatal("expected DeleteTopic() to fail on canceled context")
	}
	if _, err := store.ListTopics(ctx, ScopeUser); err == nil {
		t.Fatal("expected ListTopics() to fail on canceled context")
	}
}

func TestFileStoreAtomicWriteLeavesNoTempFiles(t *testing.T) {
	store := NewFileStore(t.TempDir(), "/workspace/project")
	if err := store.SaveIndex(context.Background(), ScopeUser, &Index{Entries: []Entry{{Type: TypeUser, Title: "test"}}}); err != nil {
		t.Fatalf("SaveIndex() error = %v", err)
	}

	entries, err := os.ReadDir(store.scopeDir(ScopeUser))
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("unexpected temp file %s", entry.Name())
		}
	}
}

func TestGlobalMemoDirectory(t *testing.T) {
	got := globalMemoDirectory("/base")
	want := filepath.Join("/base", "memo")
	if got != want {
		t.Fatalf("globalMemoDirectory() = %q, want %q", got, want)
	}
}

func TestProjectMemoDirectory(t *testing.T) {
	got := projectMemoDirectory("/base", "/workspace")
	want := filepath.Join("/base", "projects", agentsession.HashWorkspaceRoot("/workspace"), "memo")
	if got != want {
		t.Fatalf("projectMemoDirectory() = %q, want %q", got, want)
	}
}

func TestFileStoreWritesScopesToExpectedDirectories(t *testing.T) {
	baseDir := t.TempDir()
	store := NewFileStore(baseDir, "/workspace/project")

	if err := store.SaveIndex(context.Background(), ScopeUser, &Index{Entries: []Entry{{Type: TypeUser, Title: "user"}}}); err != nil {
		t.Fatalf("SaveIndex(user) error = %v", err)
	}
	if err := store.SaveIndex(context.Background(), ScopeProject, &Index{Entries: []Entry{{Type: TypeProject, Title: "project"}}}); err != nil {
		t.Fatalf("SaveIndex(project) error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(baseDir, "memo", "user", memoFileName)); err != nil {
		t.Fatalf("expected global user memo to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "projects", agentsession.HashWorkspaceRoot("/workspace/project"), "memo", "project", memoFileName)); err != nil {
		t.Fatalf("expected project memo to exist: %v", err)
	}
}
