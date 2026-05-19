package tuiv2

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAppViewShowsStatusAndPrompt(t *testing.T) {
	model := NewApp(StartupConfig{Backend: "fake", Scenario: "default"})
	view := model.View()

	for _, want := range []string{"NEOCODE", "○ idle", "fake", "ghost-console", "› "} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q in:\n%s", want, view)
		}
	}
	if strings.Contains(view, "[debug]") {
		t.Fatalf("View() contains debug line when Debug=false:\n%s", view)
	}
}

func TestAppViewShowsDebugLineWithSize(t *testing.T) {
	model := NewApp(StartupConfig{Backend: "fake", Scenario: "tool_approval", Debug: true})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	view := updated.View()

	for _, want := range []string{
		"[debug] mode:input",
		"scenario:tool_approval",
		"events:0",
		"size:120x30",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q in:\n%s", want, view)
		}
	}
}
