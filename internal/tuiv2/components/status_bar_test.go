package components

import (
	"strings"
	"testing"

	"neo-code/internal/tuiv2/state"
	"neo-code/internal/tuiv2/theme"
)

func TestAmbientStatusRendersPhaseInfo(t *testing.T) {
	viewState := state.NewViewState()
	viewState.Layout.Width = 120
	viewState.Layout.Height = 20

	view := NewAmbientStatus(viewState).View()
	for _, want := range []string{
		"NEOCODE",
		theme.StatusSymbol(theme.PhaseIdle),
		"ghost-console",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q in:\n%s", want, view)
		}
	}
}

func TestAmbientStatusRendersRunningPhase(t *testing.T) {
	viewState := state.NewViewState()
	viewState.Layout.Width = 120
	viewState.Layout.Height = 20
	viewState.Runtime.Phase = state.RuntimePhaseRunning

	view := NewAmbientStatus(viewState).View()
	if !strings.Contains(view, state.RuntimePhaseRunning) {
		t.Fatalf("View() missing running phase in:\n%s", view)
	}
}

func TestAmbientStatusWidthIsSafe(t *testing.T) {
	viewState := state.NewViewState()
	viewState.Layout.Width = 50
	viewState.Layout.Height = 10
	viewState.Runtime.Phase = state.RuntimePhaseRunning
	viewState.Gateway.ActiveModel = "claude-sonnet-4-6-very-long-model-name"

	view := NewAmbientStatus(viewState).View()
	for index, line := range strings.Split(view, "\n") {
		if width := theme.DisplayWidth(line); width > 49 {
			t.Fatalf("line %d width = %d, want <= 49: %q", index, width, line)
		}
	}
}
