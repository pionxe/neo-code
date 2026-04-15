package context

import (
	stdcontext "context"
	"strings"
	"testing"

	providertypes "neo-code/internal/provider/types"
	"neo-code/internal/skills"
)

func TestDefaultBuilderBuildInjectsSkillsInStableOrder(t *testing.T) {
	t.Parallel()

	builder := NewBuilder()
	result, err := builder.Build(stdcontext.Background(), BuildInput{
		Messages: []providertypes.Message{{Role: "user", Content: "hello"}},
		ActiveSkills: []skills.Skill{
			{
				Descriptor: skills.Descriptor{ID: "zeta", Name: "Zeta"},
				Content:    skills.Content{Instruction: "second"},
			},
			{
				Descriptor: skills.Descriptor{ID: "go_review", Name: "Go Review"},
				Content: skills.Content{
					Instruction: "first",
					ToolHints:   []string{"read docs", "run tests", "inspect code", "open diff"},
					References: []skills.Reference{
						{Title: "Ref A", Summary: "summary-a"},
						{Title: "Ref B", Summary: "summary-b"},
						{Title: "Ref C", Summary: "summary-c"},
						{Title: "Ref D", Summary: "summary-d"},
					},
					Examples: []string{"example-1", "example-1", "example-2", "example-3"},
				},
			},
			{
				Descriptor: skills.Descriptor{ID: "go-review", Name: "Go Review Duplicate"},
				Content:    skills.Content{Instruction: "duplicate"},
			},
		},
		Metadata: testMetadata(t.TempDir()),
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if !strings.Contains(result.SystemPrompt, "## Skills") {
		t.Fatalf("expected skills section, got %q", result.SystemPrompt)
	}
	goReviewIndex := strings.Index(result.SystemPrompt, "go_review")
	if goReviewIndex < 0 {
		goReviewIndex = strings.Index(result.SystemPrompt, "go-review")
	}
	zetaIndex := strings.Index(result.SystemPrompt, "zeta")
	if goReviewIndex < 0 || zetaIndex < 0 || goReviewIndex > zetaIndex {
		t.Fatalf("expected normalized stable order, got %q", result.SystemPrompt)
	}
	if strings.Count(result.SystemPrompt, "- skill: Go Review") != 1 {
		t.Fatalf("expected duplicate skill injection to be deduplicated, got %q", result.SystemPrompt)
	}
	if strings.Contains(result.SystemPrompt, "summary-d") {
		t.Fatalf("expected references to be truncated, got %q", result.SystemPrompt)
	}
	if strings.Contains(result.SystemPrompt, "example-3") {
		t.Fatalf("expected examples to be truncated, got %q", result.SystemPrompt)
	}
	if strings.Contains(result.SystemPrompt, "open diff") {
		t.Fatalf("expected tool hints to be truncated, got %q", result.SystemPrompt)
	}
}

func TestDefaultBuilderBuildSkipsSkillsSectionWhenNoActiveSkills(t *testing.T) {
	t.Parallel()

	builder := NewBuilder()
	result, err := builder.Build(stdcontext.Background(), BuildInput{
		Messages: []providertypes.Message{{Role: "user", Content: "hello"}},
		Metadata: testMetadata(t.TempDir()),
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if strings.Contains(result.SystemPrompt, "## Skills") {
		t.Fatalf("did not expect skills section without active skills, got %q", result.SystemPrompt)
	}
}
