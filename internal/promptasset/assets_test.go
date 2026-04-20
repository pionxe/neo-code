package promptasset

import (
	"strings"
	"testing"
)

func TestCoreSections(t *testing.T) {
	t.Parallel()

	sections := CoreSections()
	if len(sections) != 4 {
		t.Fatalf("expected 4 core sections, got %d", len(sections))
	}

	wantTitles := []string{
		"Agent Identity",
		"Tool Usage",
		"Failure Recovery",
		"Response Style",
	}
	for index, want := range wantTitles {
		if sections[index].Title != want {
			t.Fatalf("section %d title = %q, want %q", index, sections[index].Title, want)
		}
		if strings.TrimSpace(sections[index].Content) == "" {
			t.Fatalf("section %q content should not be empty", want)
		}
	}
}

func TestRuntimeReminderTemplates(t *testing.T) {
	t.Parallel()

	if !strings.Contains(NoProgressReminder(), "multiple consecutive attempts") {
		t.Fatalf("expected no-progress reminder guidance, got %q", NoProgressReminder())
	}
	if !strings.Contains(RepeatCycleReminder(), "exact same arguments") {
		t.Fatalf("expected repeat-cycle reminder guidance, got %q", RepeatCycleReminder())
	}
}

func TestCompactSystemPromptInterpolatesPlaceholders(t *testing.T) {
	t.Parallel()

	prompt := CompactSystemPrompt(`{"task_state":{}}`, "[compact_summary]\ndone:\n- none")
	if strings.Contains(prompt, compactTaskStateContractPlaceholder) {
		t.Fatalf("expected task state placeholder to be replaced, got %q", prompt)
	}
	if strings.Contains(prompt, compactSummaryFormatTemplatePlaceholder) {
		t.Fatalf("expected summary format placeholder to be replaced, got %q", prompt)
	}
	if !strings.Contains(prompt, `{"task_state":{}}`) {
		t.Fatalf("expected injected task state contract, got %q", prompt)
	}
	if !strings.Contains(prompt, "[compact_summary]") {
		t.Fatalf("expected injected summary format, got %q", prompt)
	}
}

func TestSubagentRolePrompts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		prompt string
		want   string
	}{
		{name: "researcher", prompt: ResearcherRolePrompt(), want: "研究型子代理"},
		{name: "coder", prompt: CoderRolePrompt(), want: "实现型子代理"},
		{name: "reviewer", prompt: ReviewerRolePrompt(), want: "审查型子代理"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if strings.TrimSpace(tt.prompt) == "" {
				t.Fatalf("prompt should not be empty")
			}
			if !strings.Contains(tt.prompt, tt.want) {
				t.Fatalf("prompt = %q, want substring %q", tt.prompt, tt.want)
			}
		})
	}
}
