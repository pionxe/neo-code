package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
)

func TestNormalizeFullAccessPromptSelectionWrap(t *testing.T) {
	if got := normalizeFullAccessPromptSelection(-1); got != len(fullAccessPromptOptions)-1 {
		t.Fatalf("expected -1 to wrap to last index, got %d", got)
	}
	if got := normalizeFullAccessPromptSelection(len(fullAccessPromptOptions)); got != 0 {
		t.Fatalf("expected overflow index to wrap to 0, got %d", got)
	}
}

func TestParseFullAccessPromptShortcut(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{input: "y", want: true},
		{input: "yes", want: true},
		{input: " enable ", want: true},
		{input: "n", want: false},
		{input: "no", want: false},
		{input: "CaNcEl", want: false},
	}
	for _, tt := range tests {
		got, ok := parseFullAccessPromptShortcut(tt.input)
		if !ok || got != tt.want {
			t.Fatalf("parseFullAccessPromptShortcut(%q)=(%v,%v), want (%v,true)", tt.input, got, ok, tt.want)
		}
	}
	if _, ok := parseFullAccessPromptShortcut("unknown"); ok {
		t.Fatal("expected unknown shortcut to fail")
	}
}

func TestFormatFullAccessPromptLines(t *testing.T) {
	lines := formatFullAccessPromptLines(fullAccessPromptState{Selected: 1})
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Enable Full Access Mode?") {
		t.Fatalf("expected header in full access prompt, got %q", joined)
	}
	if !strings.Contains(joined, "> No") {
		t.Fatalf("expected selected option marker, got %q", joined)
	}
}

func TestRenderFullAccessPrompt(t *testing.T) {
	app := App{
		appComponents: appComponents{input: textarea.New()},
		appRuntimeState: appRuntimeState{
			pendingFullAccessPrompt: &fullAccessPromptState{Selected: 0},
			layoutCached:            true,
			cachedWidth:             128,
			cachedHeight:            40,
		},
	}
	rendered := app.renderFullAccessPrompt()
	if !strings.Contains(rendered, "Enable Full Access Mode?") {
		t.Fatalf("expected full access prompt content, got %q", rendered)
	}
}

func TestNormalizeFullAccessPromptSelectionWithEmptyOptions(t *testing.T) {
	original := fullAccessPromptOptions
	fullAccessPromptOptions = nil
	defer func() {
		fullAccessPromptOptions = original
	}()

	if got := normalizeFullAccessPromptSelection(5); got != 0 {
		t.Fatalf("expected empty options normalize to 0, got %d", got)
	}
}

func TestFullAccessPromptOptionAtUsesNormalizedSelection(t *testing.T) {
	option := fullAccessPromptOptionAt(-1)
	if option.Label != "No" || option.Enable {
		t.Fatalf("expected wrapped option to select No(false), got %+v", option)
	}
}

func TestRenderFullAccessPromptFallsBackToInputView(t *testing.T) {
	input := textarea.New()
	input.SetValue("fallback-input")
	app := App{
		appComponents: appComponents{
			input: input,
		},
	}

	rendered := app.renderFullAccessPrompt()
	if !strings.Contains(rendered, "fallback-input") {
		t.Fatalf("expected input view when prompt is nil, got %q", rendered)
	}
}

func TestRenderPromptWithPendingFullAccessPrompt(t *testing.T) {
	input := textarea.New()
	input.SetValue("normal message")
	app := App{
		appComponents: appComponents{
			input: input,
		},
		styles: newStyles(),
		appRuntimeState: appRuntimeState{
			pendingFullAccessPrompt: &fullAccessPromptState{Selected: 0},
			layoutCached:            true,
			cachedWidth:             128,
			cachedHeight:            40,
		},
	}
	rendered := app.renderPrompt(80)
	if !strings.Contains(rendered, "Enable Full Access Mode?") {
		t.Fatalf("expected full access prompt rendering branch, got %q", rendered)
	}

	app.pendingFullAccessPrompt = nil
	rendered = app.renderPrompt(80)
	if !strings.Contains(rendered, "normal message") {
		t.Fatalf("expected normal input rendering branch, got %q", rendered)
	}
}
