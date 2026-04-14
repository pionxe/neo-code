package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"

	tuistate "neo-code/internal/tui/state"
)

func TestRenderPickerHelpMode(t *testing.T) {
	app, _ := newTestApp(t)
	app.refreshHelpPicker()
	app.state.ActivePicker = pickerHelp

	view := app.renderPicker(48, 14)
	if !strings.Contains(view, helpPickerTitle) {
		t.Fatalf("expected help picker title in view")
	}
	if !strings.Contains(view, helpPickerSubtitle) {
		t.Fatalf("expected help picker subtitle in view")
	}
}

func TestRenderWaterfallUsesDynamicTranscriptHeight(t *testing.T) {
	app, _ := newTestApp(t)
	app.state.ActivePicker = pickerNone
	app.state.InputText = "test"
	app.input.SetValue("test")
	app.transcript.SetContent("line1\nline2")

	view := app.renderWaterfall(80, 24)
	if strings.TrimSpace(view) == "" {
		t.Fatalf("expected non-empty waterfall view")
	}
}

func TestApplyComponentLayoutKeepsTranscriptHeightInSyncWithWaterfall(t *testing.T) {
	app, _ := newTestApp(t)
	app.width = 100
	app.height = 24
	app.focus = panelInput
	app.activities = []tuistate.ActivityEntry{{Kind: "tool", Title: "running", Detail: "tool call"}}
	app.commandMenu.SetItems([]list.Item{
		commandMenuItem{title: "/help", description: "show help"},
		commandMenuItem{title: "/model", description: "switch model"},
	})
	app.commandMenuMeta = tuistate.CommandMenuMeta{Title: commandMenuTitle}
	app.input.SetValue(strings.Repeat("line\n", 5))
	app.input.SetHeight(app.composerHeight())

	app.applyComponentLayout(false)

	lay := app.computeLayout()
	wantTranscriptHeight, activityHeight, menuHeight, _ := app.waterfallMetrics(app.transcript.Width, lay.contentHeight)
	if app.transcript.Height != wantTranscriptHeight {
		t.Fatalf("expected transcript height %d, got %d", wantTranscriptHeight, app.transcript.Height)
	}

	_, transcriptY, _, transcriptHeight := app.transcriptBounds()
	_, activityY, _, gotActivityHeight := app.activityBounds()
	_, inputY, _, _ := app.inputBounds()
	if transcriptHeight != wantTranscriptHeight {
		t.Fatalf("expected transcript bounds height %d, got %d", wantTranscriptHeight, transcriptHeight)
	}
	if activityY != transcriptY+wantTranscriptHeight {
		t.Fatalf("expected activity Y %d, got %d", transcriptY+wantTranscriptHeight, activityY)
	}
	if gotActivityHeight != activityHeight {
		t.Fatalf("expected activity height %d, got %d", activityHeight, gotActivityHeight)
	}
	if inputY != transcriptY+wantTranscriptHeight+activityHeight+menuHeight {
		t.Fatalf("expected input Y %d, got %d", transcriptY+wantTranscriptHeight+activityHeight+menuHeight, inputY)
	}
}
