package components

import (
	"strings"
	"testing"
	"time"

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

// TestAmbientStatusRendersNotifyText 验证弱提示渲染（issue #41 A5）：
// Notify.Text 非空即渲染进状态栏；空文本不产生残留（组件不自算过期，
// 清除责在内核 notifyExpiryMsg——审计 P2-1 裁定）。
func TestAmbientStatusRendersNotifyText(t *testing.T) {
	viewState := state.NewViewState()
	viewState.Layout.Width = 120
	// 空文本：状态栏不含弱提示段（此处以 "Debug:" 作哨兵串断言不出现）。
	view := NewAmbientStatus(viewState).View()
	if strings.Contains(view, "Debug:") {
		t.Fatalf("empty notify should not render, got:\n%s", view)
	}
	// 非空文本：渲染进状态栏。
	viewState.Notify = state.NotifyState{Text: "Debug: true", At: time.Now()}
	view = NewAmbientStatus(viewState).View()
	if !strings.Contains(view, "Debug: true") {
		t.Fatalf("notify text should render, got:\n%s", view)
	}
	// 内核清除（Text 置空）后消失。
	viewState.Notify = state.NotifyState{}
	view = NewAmbientStatus(viewState).View()
	if strings.Contains(view, "Debug: true") {
		t.Fatalf("cleared notify should disappear, got:\n%s", view)
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
