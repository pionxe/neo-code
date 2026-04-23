package context

import (
	"context"
	"errors"
	"strings"
	"testing"

	"neo-code/internal/repository"
)

func TestCollectSystemStateHandlesGitUnavailable(t *testing.T) {
	t.Parallel()

	state, err := collectSystemState(context.Background(), testMetadata("/workspace"), func(ctx context.Context, workdir string) (repository.Summary, error) {
		return repository.Summary{}, errors.New("git unavailable")
	})
	if err != nil {
		t.Fatalf("collectSystemState() error = %v", err)
	}

	if state.Git.Available {
		t.Fatalf("expected git to be unavailable")
	}

	section := renderPromptSection(renderSystemStateSection(state))
	if !strings.Contains(section, "- git: unavailable") {
		t.Fatalf("expected unavailable git section, got %q", section)
	}
}

func TestCollectSystemStateIncludesRepositorySummary(t *testing.T) {
	t.Parallel()

	callCount := 0
	provider := func(ctx context.Context, workdir string) (repository.Summary, error) {
		callCount++
		if workdir != "/workspace" {
			return repository.Summary{}, errors.New("unexpected workdir")
		}
		return repository.Summary{
			InGitRepo: true,
			Branch:    "feature/context",
			Dirty:     true,
			Ahead:     2,
			Behind:    1,
		}, nil
	}

	state, err := collectSystemState(context.Background(), testMetadata("/workspace"), provider)
	if err != nil {
		t.Fatalf("collectSystemState() error = %v", err)
	}

	if callCount != 1 {
		t.Fatalf("expected a single repository summary call, got %d", callCount)
	}
	if !state.Git.Available {
		t.Fatalf("expected git to be available")
	}
	if state.Git.Branch != "feature/context" {
		t.Fatalf("expected branch to be trimmed, got %q", state.Git.Branch)
	}
	if !state.Git.Dirty {
		t.Fatalf("expected dirty git state")
	}
	if state.Git.Ahead != 2 || state.Git.Behind != 1 {
		t.Fatalf("expected ahead=2 behind=1, got %+v", state.Git)
	}

	section := renderPromptSection(renderSystemStateSection(state))
	if !strings.Contains(section, "branch=`feature/context`") {
		t.Fatalf("expected branch in system section, got %q", section)
	}
	if !strings.Contains(section, "dirty=`dirty`") {
		t.Fatalf("expected dirty marker in system section, got %q", section)
	}
	if !strings.Contains(section, "ahead=`2`, behind=`1`") {
		t.Fatalf("expected ahead/behind counters in system section, got %q", section)
	}
}

func TestCollectSystemStateReturnsContextError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := collectSystemState(ctx, testMetadata("/workspace"), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
}

func TestSystemStateSourceSectionsReturnsRepositoryContextError(t *testing.T) {
	t.Parallel()

	source := &systemStateSource{
		summary: func(ctx context.Context, workdir string) (repository.Summary, error) {
			return repository.Summary{}, context.DeadlineExceeded
		},
	}

	_, err := source.Sections(context.Background(), BuildInput{
		Metadata: testMetadata("/workspace"),
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestCollectSystemStateSkipsSummaryWhenProviderUnavailableOrWorkdirBlank(t *testing.T) {
	t.Parallel()

	state, err := collectSystemState(context.Background(), Metadata{
		Workdir:  " /workspace ",
		Shell:    " powershell ",
		Provider: " openai ",
		Model:    " gpt-test ",
	}, nil)
	if err != nil {
		t.Fatalf("collectSystemState() error = %v", err)
	}
	if state.Git.Available {
		t.Fatalf("expected git to stay unavailable without provider")
	}
	if state.Workdir != "/workspace" {
		t.Fatalf("expected trimmed workdir, got %q", state.Workdir)
	}

	state, err = collectSystemState(context.Background(), Metadata{
		Workdir:  " ",
		Shell:    " bash ",
		Provider: " local ",
		Model:    " mini ",
	}, func(ctx context.Context, workdir string) (repository.Summary, error) {
		t.Fatalf("summary provider should not be called for blank workdir")
		return repository.Summary{}, nil
	})
	if err != nil {
		t.Fatalf("collectSystemState() blank workdir error = %v", err)
	}
	if state.Git.Available {
		t.Fatalf("expected git to stay unavailable for blank workdir")
	}
}

func TestToGitStateMapsRepositorySummary(t *testing.T) {
	t.Parallel()

	state := toGitState(repository.Summary{
		InGitRepo: true,
		Branch:    "main",
		Ahead:     2,
		Behind:    3,
	})
	if !state.Available || state.Branch != "main" || state.Dirty {
		t.Fatalf("unexpected mapped state: %+v", state)
	}
	if state.Ahead != 2 || state.Behind != 3 {
		t.Fatalf("unexpected ahead/behind mapping: %+v", state)
	}

	unavailable := toGitState(repository.Summary{})
	if unavailable.Available {
		t.Fatalf("expected unavailable state for empty summary, got %+v", unavailable)
	}
}
