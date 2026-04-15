package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"

	providertypes "neo-code/internal/provider/types"
	agentsession "neo-code/internal/session"
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

func TestRenderPickerSessionMode(t *testing.T) {
	app, _ := newTestApp(t)
	app.state.ActivePicker = pickerSession
	app.sessionPicker.SetItems([]list.Item{
		sessionItem{Summary: agentsession.Summary{
			ID:        "session-1",
			Title:     "Session One",
			UpdatedAt: time.Now(),
		}},
	})

	view := app.renderPicker(48, 14)
	if !strings.Contains(view, sessionPickerTitle) {
		t.Fatalf("expected session picker title in view")
	}
	if !strings.Contains(view, sessionPickerSubtitle) {
		t.Fatalf("expected session picker subtitle in view")
	}
	if !strings.Contains(view, "Session One") {
		t.Fatalf("expected session item in picker body")
	}
}

func TestRenderPickerProviderAndFileMode(t *testing.T) {
	app, _ := newTestApp(t)

	app.state.ActivePicker = pickerProvider
	app.providerPicker.SetItems([]list.Item{selectionItem{id: "p1", name: "Provider 1"}})
	providerView := app.renderPicker(48, 14)
	if !strings.Contains(providerView, providerPickerTitle) {
		t.Fatalf("expected provider picker title")
	}

	app.state.ActivePicker = pickerFile
	fileView := app.renderPicker(48, 14)
	if !strings.Contains(fileView, filePickerTitle) {
		t.Fatalf("expected file picker title")
	}
}

func TestBuildPickerLayoutExpandsPopupSpace(t *testing.T) {
	app, _ := newTestApp(t)

	got := app.buildPickerLayout(100, 30)
	if got.panelHeight < 20 {
		t.Fatalf("expected expanded picker panel height, got %d", got.panelHeight)
	}
	if got.listHeight < pickerListMinHeight {
		t.Fatalf("expected picker list height >= %d, got %d", pickerListMinHeight, got.listHeight)
	}
	if got.listWidth < pickerListMinWidth {
		t.Fatalf("expected picker list width >= %d, got %d", pickerListMinWidth, got.listWidth)
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

func TestRenderWaterfallThinkingState(t *testing.T) {
	app, _ := newTestApp(t)
	app.state.ActivePicker = pickerNone
	app.state.IsAgentRunning = true
	app.state.StatusText = statusThinking

	view := app.renderWaterfall(80, 24)
	if !strings.Contains(view, "Thinking...") {
		t.Fatalf("expected thinking hint in waterfall view")
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

func TestComputeLayoutUsesRenderedHeaderHeight(t *testing.T) {
	app, _ := newTestApp(t)
	app.width = 100
	app.height = 30

	lay := app.computeLayout()
	header := app.renderHeader(lay.contentWidth)
	if got := lipgloss.Height(header); got != headerBarHeight {
		t.Fatalf("expected header height %d, got %d", headerBarHeight, got)
	}
	if strings.Contains(header, "\x1b[") {
		t.Fatalf("expected header to avoid ANSI escapes, got %q", header)
	}
}

func TestRenderUserMessageKeepsTagAndBodyRightAligned(t *testing.T) {
	app, _ := newTestApp(t)

	block, _ := app.renderMessageBlockWithCopy(providertypes.Message{
		Role:    roleUser,
		Content: "hello right aligned",
	}, 72, 1)

	plain := copyCodeANSIPattern.ReplaceAllString(block, "")
	lines := strings.Split(plain, "\n")

	var (
		tagLine     string
		contentLine string
	)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.Contains(line, messageTagUser) {
			tagLine = line
		}
		if strings.Contains(line, "hello right aligned") {
			contentLine = line
		}
	}
	if tagLine == "" || contentLine == "" {
		t.Fatalf("expected user tag and content lines, got %q", plain)
	}

	tagRightEdge := lipgloss.Width(strings.TrimRight(tagLine, " "))
	bodyRightEdge := lipgloss.Width(strings.TrimRight(contentLine, " "))
	if tagRightEdge != bodyRightEdge {
		t.Fatalf("expected user tag and body right edges to match, got tag=%d body=%d\n%q\n%q", tagRightEdge, bodyRightEdge, tagLine, contentLine)
	}
}

func TestBuildPickerLayoutClampMin(t *testing.T) {
	app, _ := newTestApp(t)
	got := app.buildPickerLayout(10, 8)
	if got.panelWidth != pickerPanelMinWidth {
		t.Fatalf("expected panel width clamp to min %d, got %d", pickerPanelMinWidth, got.panelWidth)
	}
	if got.panelHeight != pickerPanelMinHeight {
		t.Fatalf("expected panel height clamp to min %d, got %d", pickerPanelMinHeight, got.panelHeight)
	}
}

func TestRenderWaterfallWithActivePicker(t *testing.T) {
	app, _ := newTestApp(t)
	app.state.ActivePicker = pickerSession
	app.sessionPicker.SetItems([]list.Item{
		sessionItem{Summary: agentsession.Summary{
			ID:        "session-1",
			Title:     "Session One",
			UpdatedAt: time.Now(),
		}},
	})

	view := app.renderWaterfall(90, 24)
	if !strings.Contains(view, sessionPickerTitle) {
		t.Fatalf("expected picker waterfall view to include session picker title")
	}
}

func TestRenderBody(t *testing.T) {
	app, _ := newTestApp(t)
	out := app.renderBody(layout{contentWidth: 90, contentHeight: 24})
	if strings.TrimSpace(out) == "" {
		t.Fatalf("expected renderBody output")
	}
}
