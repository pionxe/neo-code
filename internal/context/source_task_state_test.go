package context

import (
	"strings"
	"testing"

	agentsession "neo-code/internal/session"
)

func TestRenderTaskStateSectionSanitizesValues(t *testing.T) {
	t.Parallel()

	section := renderTaskStateSection(agentsession.TaskState{
		Goal:            "  finish\n\tmigration  ",
		Progress:        []string{" first\nitem ", "\t", "second\x00item"},
		OpenItems:       []string{" review\r\ncomment "},
		NextStep:        "  run\t tests\r\nnow ",
		Blockers:        []string{" none\x1fneeded "},
		KeyArtifacts:    []string{" internal/context/source_task_state.go\t"},
		Decisions:       []string{" keep\nsingle-line format "},
		UserConstraints: []string{"  do-not\tmigrate\r\nold-data  "},
	})

	want := strings.Join([]string{
		"- goal: finish migration",
		"- progress: first item | second item",
		"- open_items: review comment",
		"- next_step: run tests now",
		"- blockers: none needed",
		"- key_artifacts: internal/context/source_task_state.go",
		"- decisions: keep single-line format",
		"- user_constraints: do-not migrate old-data",
	}, "\n")

	if section.Title != "Task State" {
		t.Fatalf("expected title %q, got %q", "Task State", section.Title)
	}
	if section.Content != want {
		t.Fatalf("unexpected section content:\nwant:\n%s\n\ngot:\n%s", want, section.Content)
	}
}

func TestRenderTaskStateSectionUsesNonePlaceholdersAndStableOrder(t *testing.T) {
	t.Parallel()

	section := renderTaskStateSection(agentsession.TaskState{})

	want := strings.Join([]string{
		"- goal: none",
		"- progress: none",
		"- open_items: none",
		"- next_step: none",
		"- blockers: none",
		"- key_artifacts: none",
		"- decisions: none",
		"- user_constraints: none",
	}, "\n")

	if section.Content != want {
		t.Fatalf("unexpected section content:\nwant:\n%s\n\ngot:\n%s", want, section.Content)
	}
}
