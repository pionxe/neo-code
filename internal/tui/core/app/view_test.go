package tui

import (
	"strings"
	"testing"
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
