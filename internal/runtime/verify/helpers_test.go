package verify

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestVerificationDeniedResult(t *testing.T) {
	t.Parallel()

	result := verificationDeniedResult("file_exists", "denied", "policy", map[string]any{"a": 1})
	if result.Status != VerificationFail || result.ErrorClass != ErrorClassPermissionDenied {
		t.Fatalf("unexpected denied result: %+v", result)
	}
}

func TestCollectArtifactTargets(t *testing.T) {
	t.Parallel()

	got := collectArtifactTargets(FinalVerifyInput{
		TaskState: TaskStateSnapshot{KeyArtifacts: []string{"README.md", "README.md"}},
		Todos: []TodoSnapshot{
			{ID: "todo-1", Required: true, Artifacts: []string{"main.go"}},
			{ID: "todo-2", Required: false, Artifacts: []string{"ignored.txt"}},
		},
	})
	want := []string{"README.md", "main.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("collectArtifactTargets() = %#v, want %#v", got, want)
	}
}

func TestCollectContentCheckRules(t *testing.T) {
	t.Parallel()

	got, err := collectContentCheckRules(FinalVerifyInput{
		Todos: []TodoSnapshot{
			{
				ID:       "todo-1",
				Required: true,
				ContentChecks: []TodoContentCheckSnapshot{
					{Artifact: "a.txt", Contains: []string{"hello", "world"}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("collectContentCheckRules() error = %v", err)
	}
	want := map[string][]string{"a.txt": []string{"hello", "world"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("collectContentCheckRules() = %#v, want %#v", got, want)
	}

	if _, err := collectContentCheckRules(FinalVerifyInput{
		Todos: []TodoSnapshot{{ID: "todo-1", Required: true, ContentChecks: []TodoContentCheckSnapshot{{Artifact: "a.txt"}}}},
	}); err == nil {
		t.Fatal("expected empty contains validation error")
	}
}

func TestResolvePathWithinWorkdir(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	child, err := resolvePathWithinWorkdir(workdir, "a/b.txt")
	if err != nil {
		t.Fatalf("resolvePathWithinWorkdir() error = %v", err)
	}
	if !strings.HasPrefix(child, workdir) {
		t.Fatalf("resolved path %q should be in workdir %q", child, workdir)
	}

	absChild, err := resolvePathWithinWorkdir(workdir, filepath.Join(workdir, "a.txt"))
	if err != nil {
		t.Fatalf("resolvePathWithinWorkdir(abs) error = %v", err)
	}
	if !strings.HasPrefix(absChild, workdir) {
		t.Fatalf("resolved abs path %q should be in workdir %q", absChild, workdir)
	}

	if _, err := resolvePathWithinWorkdir("", "a.txt"); err == nil {
		t.Fatalf("expected empty workdir error")
	}
	if _, err := resolvePathWithinWorkdir(workdir, ""); err == nil {
		t.Fatalf("expected empty path error")
	}
	if _, err := resolvePathWithinWorkdir(workdir, "../outside.txt"); err == nil {
		t.Fatalf("expected traversal error")
	}
}

func TestResolvePathWithinWorkdirRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workdir := filepath.Join(root, "work")
	outsideDir := filepath.Join(root, "outside")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workdir) error = %v", err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(outside) error = %v", err)
	}
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o644); err != nil {
		t.Fatalf("WriteFile(outside) error = %v", err)
	}
	linkPath := filepath.Join(workdir, "link.txt")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Skipf("symlink is not available on this platform: %v", err)
	}

	if _, err := resolvePathWithinWorkdir(workdir, "link.txt"); err == nil {
		t.Fatalf("expected symlink escape to be denied")
	}
}
