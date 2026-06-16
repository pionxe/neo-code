package components

import (
	"strings"
	"testing"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/state"
)

func TestSoftInspectorRendersSessionList(t *testing.T) {
	viewState := state.NewViewState()
	viewState.Layout.Width = 120
	viewState.Layout.Height = 30
	viewState.Layout.ShowInspector = true
	viewState.Layout.InspectorWidth = 30
	viewState.Gateway.Sessions = []gateway.SessionSummary{
		{ID: "s1", Title: "debug-session"},
		{ID: "s2", Title: "refactor-task"},
	}

	view := NewSoftInspector(viewState).View()
	for _, want := range []string{
		"Soft Inspector",
		"debug-session",
		"refactor-task",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q in:\n%s", want, view)
		}
	}
}

func TestSoftInspectorRendersTokenUsage(t *testing.T) {
	viewState := state.NewViewState()
	viewState.Layout.Width = 120
	viewState.Layout.Height = 30
	viewState.Layout.ShowInspector = true
	viewState.Layout.InspectorWidth = 30
	viewState.Runtime.Tokens = state.TokenUsage{Input: 1024, Output: 512, Total: 1536}

	view := NewSoftInspector(viewState).View()
	for _, want := range []string{
		"Token Usage",
		"1024",
		"512",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q in:\n%s", want, view)
		}
	}
}

func TestSoftInspectorHiddenWhenDisabled(t *testing.T) {
	viewState := state.NewViewState()
	viewState.Layout.ShowInspector = false

	view := NewSoftInspector(viewState).View()
	if view != "" {
		t.Fatalf("View() = %q, want empty when inspector hidden", view)
	}
}

func TestSoftInspectorRendersActiveTools(t *testing.T) {
	viewState := state.NewViewState()
	viewState.Layout.Width = 120
	viewState.Layout.Height = 30
	viewState.Layout.ShowInspector = true
	viewState.Layout.InspectorWidth = 30
	viewState.Stream = []state.StreamEntry{
		{ID: "ts1", Type: "tool_start", ToolName: "read_file", Content: "main.go"},
		{ID: "ts2", Type: "tool_start", ToolName: "write_file", Content: "app.go"},
		{ID: "te1", Type: "tool_end", ToolName: "read_file"},
	}

	view := NewSoftInspector(viewState).View()
	for _, want := range []string{
		"Active Tools",
		"tool.write_file",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q in:\n%s", want, view)
		}
	}
	if strings.Contains(view, "tool.read_file") {
		t.Fatalf("View() should not show completed tool read_file in:\n%s", view)
	}
}

func TestSoftInspectorRendersFiles(t *testing.T) {
	viewState := state.NewViewState()
	viewState.Layout.Width = 120
	viewState.Layout.Height = 30
	viewState.Layout.ShowInspector = true
	viewState.Layout.InspectorWidth = 30
	viewState.Stream = []state.StreamEntry{
		{ID: "te1", Type: "tool_end", ToolName: "read_file", Content: "src/main.go"},
		{ID: "te2", Type: "tool_end", ToolName: "write_file", Content: "delete config.yaml"},
	}

	view := NewSoftInspector(viewState).View()
	for _, want := range []string{
		"Files",
		"main.go",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q in:\n%s", want, view)
		}
	}
}
