package components

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/state"
	"neo-code/internal/tuiv2/theme"
)

func TestCommandPromptMessageInputSubmitAndMultiline(t *testing.T) {
	viewState := promptState()
	prompt := NewCommandPrompt(viewState)

	_, cmd := prompt.Update(keyMsg("hello"))
	if cmd != nil {
		t.Fatal("typing returned command, want nil")
	}
	_, _ = prompt.Update(keyMsg("shift+enter"))
	_, _ = prompt.Update(keyMsg("world"))
	if viewState.Input.Text != "hello\nworld" {
		t.Fatalf("Input.Text = %q, want multiline text", viewState.Input.Text)
	}

	_, cmd = prompt.Update(keyType(tea.KeyEnter))
	got, ok := cmd().(SubmitMessageMsg)
	if !ok {
		t.Fatalf("submit msg = %T, want SubmitMessageMsg", cmd())
	}
	if got.Text != "hello\nworld" {
		t.Fatalf("SubmitMessageMsg.Text = %q", got.Text)
	}
	if viewState.Input.Text != "" || viewState.Input.Cursor != 0 {
		t.Fatalf("input not cleared after submit: %+v", viewState.Input)
	}
}

func TestCommandPromptPermissionSingleKeyActions(t *testing.T) {
	viewState := promptState()
	viewState.Input.Mode = state.InputStateModePermissionResponse
	viewState.Input.Prompt = "tool.write_file 请求写入 main.go (2.3k) — 是否允许?"
	prompt := NewCommandPrompt(viewState)

	view := prompt.View()
	for _, want := range []string{
		theme.StatusSymbol(theme.PhaseWaitingPermission),
		"tool.write_file 请求写入 main.go",
		"[Y] 允许",
		"[d] 查看 diff",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("permission view missing %q in:\n%s", want, view)
		}
	}

	_, cmd := prompt.Update(keyMsg("y"))
	got, ok := cmd().(PermissionActionMsg)
	if !ok {
		t.Fatalf("permission msg = %T, want PermissionActionMsg", cmd())
	}
	if got.Decision != "y" {
		t.Fatalf("Decision = %q, want y", got.Decision)
	}
}

func TestCommandPromptQuestionAnswerAndOptionWrapping(t *testing.T) {
	viewState := promptState()
	viewState.Layout.Width = 34
	viewState.Input.Mode = state.InputStateModeQuestionAnswer
	viewState.Input.Prompt = "请选择要使用的模块:"
	viewState.Input.Options = []string{
		"auth 模块 — 负责用户认证与授权",
		"api 模块 — REST API 接口层并包含非常长的描述",
		"db 模块 — 数据库访问层",
	}
	prompt := NewCommandPrompt(viewState)

	view := prompt.View()
	for _, want := range []string{
		theme.Separator() + " 请选择要使用的模块:",
		"  1. auth 模块",
		"  2. api 模块",
		"[1-3] 输入数字选择",
		"[Enter] 确认",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("question view missing %q in:\n%s", want, view)
		}
	}
	for index, line := range strings.Split(view, "\n") {
		if width := theme.DisplayWidth(line); width > 33 {
			t.Fatalf("line %d width = %d, want <= 33: %q", index, width, line)
		}
	}

	_, _ = prompt.Update(keyMsg("2"))
	_, cmd := prompt.Update(keyType(tea.KeyEnter))
	got, ok := cmd().(QuestionAnswerMsg)
	if !ok {
		t.Fatalf("question msg = %T, want QuestionAnswerMsg", cmd())
	}
	if got.Text != "2" {
		t.Fatalf("QuestionAnswerMsg.Text = %q, want 2", got.Text)
	}
}

func TestCommandPromptCursorMovementHistoryAndBlink(t *testing.T) {
	viewState := promptState()
	prompt := NewCommandPrompt(viewState)

	_, _ = prompt.Update(keyMsg("ab"))
	_, _ = prompt.Update(keyType(tea.KeyLeft))
	_, _ = prompt.Update(keyMsg("中"))
	if viewState.Input.Text != "a中b" {
		t.Fatalf("Input.Text = %q, want rune-safe insert", viewState.Input.Text)
	}
	visible := viewState.Input.CursorVisible
	_, cmd := prompt.Update(CursorBlinkMsg{})
	if cmd == nil {
		t.Fatal("CursorBlinkMsg returned nil command")
	}
	if viewState.Input.CursorVisible == visible {
		t.Fatal("CursorVisible did not toggle")
	}

	viewState.Input.History = []string{"first", "second"}
	viewState.Mode = state.NormalMode
	_, _ = prompt.Update(keyType(tea.KeyUp))
	if viewState.Input.Text != "second" {
		t.Fatalf("history up text = %q, want second", viewState.Input.Text)
	}
	_, _ = prompt.Update(keyType(tea.KeyDown))
	if viewState.Input.Text != "" {
		t.Fatalf("history down text = %q, want empty", viewState.Input.Text)
	}
}

func TestCommandPromptModeLineUsesSessionAndModel(t *testing.T) {
	viewState := promptState()
	viewState.Gateway.ActiveSess = &gateway.SessionSummary{ID: "s1", Title: "ghost-console"}
	viewState.Gateway.ActiveModel = "claude-sonnet-4-6"

	view := NewCommandPrompt(viewState).View()
	for _, want := range []string{"[input]", "ghost-console", "claude-sonnet-4-6"} {
		if !strings.Contains(view, want) {
			t.Fatalf("mode line missing %q in:\n%s", want, view)
		}
	}
}

func TestCommandPromptCtrlJInsertsNewline(t *testing.T) {
	viewState := promptState()
	prompt := NewCommandPrompt(viewState)

	_, _ = prompt.Update(keyMsg("hello"))
	_, cmd := prompt.Update(keyType(tea.KeyCtrlJ))
	if cmd != nil {
		t.Fatalf("ctrl+j returned command %T, want nil", cmd)
	}
	if viewState.Input.Text != "hello\n" {
		t.Fatalf("Input.Text = %q, want %q", viewState.Input.Text, "hello\n")
	}
}

func TestCommandPromptMultilineContinuationAligns(t *testing.T) {
	viewState := promptState()
	viewState.Input.Text = "first\nsecond"
	viewState.Input.Cursor = 0
	viewState.Input.CursorVisible = true
	prompt := NewCommandPrompt(viewState)

	// 用确定性双宽符号，确保续行缩进等于首行前缀显示宽度。
	const symbol = "你"
	wantIndent := theme.DisplayWidth(symbol + " ")
	rendered := prompt.renderPromptInput(symbol)
	for index, line := range strings.Split(ansi.Strip(rendered), "\n") {
		if index == 0 {
			continue
		}
		leading := len(line) - len(strings.TrimLeft(line, " "))
		if leading != wantIndent {
			t.Fatalf("line %d leading spaces = %d, want %d: %q", index, leading, wantIndent, line)
		}
	}
}

func promptState() *state.ViewState {
	viewState := state.NewViewState()
	viewState.Layout.Width = 90
	viewState.Layout.Height = 20
	viewState.Input.CursorVisible = true
	return viewState
}

func keyType(key tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: key}
}
