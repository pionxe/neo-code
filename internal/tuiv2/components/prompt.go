package components

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"neo-code/internal/tuiv2/state"
	"neo-code/internal/tuiv2/theme"
)

const (
	cursorBlinkInterval = 500 * time.Millisecond
	promptWrapIndent    = "    "
)

// SubmitMessageMsg 表示用户在消息模式下提交了一条待发送文本。
type SubmitMessageMsg struct {
	Text string
}

// PermissionActionMsg 表示用户在权限模式下选择了 y/n/d/a 之一。
type PermissionActionMsg struct {
	Decision string
}

// QuestionAnswerMsg 表示用户在 ask_user 模式下提交了回答文本。
type QuestionAnswerMsg struct {
	Text string
}

// SlashCommandMsg 表示用户提交了一条 Slash 命令。
type SlashCommandMsg struct {
	Command string
	Args    string
}

// PromptCancelMsg 表示用户取消了当前内联交互。
type PromptCancelMsg struct {
	Mode string
}

// CursorBlinkMsg 驱动 Prompt 光标在 Bubble Tea 更新循环中闪烁。
type CursorBlinkMsg struct{}

// CommandPrompt 渲染命令、消息输入、权限确认和 ask_user 内联交互区域。
type CommandPrompt struct {
	state *state.ViewState
}

var _ tea.Model = (*CommandPrompt)(nil)

// NewCommandPrompt 创建命令输入组件。
func NewCommandPrompt(viewState *state.ViewState) *CommandPrompt {
	return &CommandPrompt{state: viewState}
}

// Init 启动光标闪烁时钟，后续 tick 仍通过 Update 返回命令续订。
func (c *CommandPrompt) Init() tea.Cmd {
	return cursorBlinkCmd()
}

// Update 根据当前 InputState.Mode 路由按键，业务动作以 tea.Msg 形式返回给 App。
func (c *CommandPrompt) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case CursorBlinkMsg:
		c.state.Input.CursorVisible = !c.state.Input.CursorVisible
		return c, cursorBlinkCmd()
	case tea.KeyMsg:
		switch c.state.Input.Mode {
		case state.InputStateModePermissionResponse:
			return c, c.handlePermissionKey(msg)
		case state.InputStateModeQuestionAnswer:
			return c, c.handleQuestionKey(msg)
		default:
			return c, c.handleInputKey(msg)
		}
	default:
		return c, nil
	}
}

// View 根据输入模式渲染底部 Prompt，保持无边框、内联和定宽安全。
func (c *CommandPrompt) View() string {
	lines := []string{theme.MutedStyle().Render("Command Prompt")}
	switch c.state.Input.Mode {
	case state.InputStateModePermissionResponse:
		lines = append(lines, c.permissionLines()...)
	case state.InputStateModeQuestionAnswer:
		lines = append(lines, c.questionLines()...)
	default:
		lines = append(lines, c.messageLines()...)
	}
	lines = append(lines, c.modeLine())
	content := strings.Join(lines, "\n")
	if c.state.Layout.Width > 0 {
		return fitBlock(content, c.state.Layout.Width, true)
	}
	return content
}

// handleInputKey 处理普通消息输入、历史切换、换行和提交。
func (c *CommandPrompt) handleInputKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		c.state.Mode = state.NormalMode
	case "i":
		if c.state.Mode == state.NormalMode {
			c.state.Mode = state.InputModeInput
		} else {
			c.insertRunes(msg.Runes)
		}
	case "left":
		c.moveCursor(-1)
	case "right":
		c.moveCursor(1)
	case "home":
		c.state.Input.Cursor = 0
	case "end":
		c.state.Input.Cursor = runeLen(c.state.Input.Text)
	case "backspace", "ctrl+h":
		c.deleteBeforeCursor()
	case "delete":
		c.deleteAtCursor()
	case "shift+enter", "alt+enter":
		c.insertText("\n")
	case "enter":
		text := strings.TrimSpace(c.state.Input.Text)
		if text == "" {
			return nil
		}
		c.pushHistory(text)
		c.clearText()
		// Slash 命令拦截
		if strings.HasPrefix(text, "/") {
			cmd, args, _ := strings.Cut(text, " ")
			return emitMsg(SlashCommandMsg{Command: cmd, Args: args})
		}
		return emitMsg(SubmitMessageMsg{Text: text})
	case "up":
		if c.state.Mode == state.NormalMode {
			c.previousHistory()
		}
	case "down":
		if c.state.Mode == state.NormalMode {
			c.nextHistory()
		}
	default:
		if c.state.Mode == state.InputModeInput && len(msg.Runes) > 0 {
			c.insertRunes(msg.Runes)
		}
	}
	return nil
}

// handlePermissionKey 处理权限模式的一键响应，不要求用户再按 Enter。
func (c *CommandPrompt) handlePermissionKey(msg tea.KeyMsg) tea.Cmd {
	switch strings.ToLower(msg.String()) {
	case "y", "n", "d", "a":
		decision := strings.ToLower(msg.String())
		c.clearText()
		return emitMsg(PermissionActionMsg{Decision: decision})
	case "esc":
		c.clearText()
		return emitMsg(PromptCancelMsg{Mode: state.InputStateModePermissionResponse})
	case "left":
		c.moveCursor(-1)
	case "right":
		c.moveCursor(1)
	case "backspace", "ctrl+h":
		c.deleteBeforeCursor()
	default:
		if len(msg.Runes) > 0 {
			c.insertRunes(msg.Runes)
		}
	}
	return nil
}

// handleQuestionKey 处理 ask_user 回答输入、数字快捷选择和确认。
func (c *CommandPrompt) handleQuestionKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		c.clearText()
		return emitMsg(PromptCancelMsg{Mode: state.InputStateModeQuestionAnswer})
	case "enter":
		text := strings.TrimSpace(c.state.Input.Text)
		if text == "" {
			return nil
		}
		c.clearText()
		return emitMsg(QuestionAnswerMsg{Text: text})
	case "left":
		c.moveCursor(-1)
	case "right":
		c.moveCursor(1)
	case "backspace", "ctrl+h":
		c.deleteBeforeCursor()
	case "delete":
		c.deleteAtCursor()
	default:
		if len(msg.Runes) > 0 {
			c.insertRunes(msg.Runes)
		}
	}
	return nil
}

// messageLines 渲染命令和普通消息模式下的输入行。
func (c *CommandPrompt) messageLines() []string {
	return []string{c.renderPromptInput("›")}
}

// permissionLines 渲染权限确认的提示、输入和快捷操作栏。
func (c *CommandPrompt) permissionLines() []string {
	prompt := c.state.Input.Prompt
	if prompt == "" {
		prompt = "permission requested"
	}
	return []string{
		theme.WarningStyle().Render(theme.StatusSymbol(theme.PhaseWaitingPermission)+" ") +
			theme.SubtleStyle().Render(prompt),
		c.renderPromptInput("›"),
		c.renderShortcutBar([]shortcutItem{
			{Key: "Y", Text: "允许"},
			{Key: "n", Text: "拒绝"},
			{Key: "d", Text: "查看 diff"},
			{Key: "a", Text: "允许全部"},
		}),
	}
}

// questionLines 渲染 ask_user 问题、输入框、选项和快捷操作栏。
func (c *CommandPrompt) questionLines() []string {
	prompt := c.state.Input.Prompt
	if prompt == "" {
		prompt = "question"
	}
	lines := []string{
		theme.AccentStyle().Render(theme.Separator()+" ") + theme.BaseStyle().Render(prompt),
		c.renderPromptInput("›"),
	}
	if len(c.state.Input.Options) > 0 {
		lines = append(lines, "")
		lines = append(lines, c.optionLines()...)
	}
	lines = append(lines, c.renderQuestionHint())
	return lines
}

// renderPromptInput 渲染带光标的输入文本，多行输入用缩进保持视觉连续。
func (c *CommandPrompt) renderPromptInput(symbol string) string {
	text := c.textWithCursor()
	rawLines := strings.Split(text, "\n")
	if len(rawLines) == 0 {
		rawLines = []string{""}
	}
	lines := make([]string, 0, len(rawLines))
	for index, line := range rawLines {
		if index == 0 {
			lines = append(lines, theme.AccentStyle().Render(symbol+" ")+theme.BaseStyle().Render(line))
			continue
		}
		lines = append(lines, theme.MutedStyle().Render("  ")+theme.BaseStyle().Render(line))
	}
	return strings.Join(lines, "\n")
}

type shortcutItem struct {
	Key  string
	Text string
}

// renderShortcutBar 渲染权限快捷键提示，括号和按键使用强调色。
func (c *CommandPrompt) renderShortcutBar(items []shortcutItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, theme.AccentStyle().Render("["+item.Key+"]")+" "+theme.SubtleStyle().Render(item.Text))
	}
	return strings.Join(parts, "  ")
}

// optionLines 渲染 ask_user 选项，并让长文本换行后对齐到选项文本起始处。
func (c *CommandPrompt) optionLines() []string {
	width := c.contentWidth()
	lines := make([]string, 0, len(c.state.Input.Options))
	for index, option := range c.state.Input.Options {
		number := strconv.Itoa(index + 1)
		prefix := "  " + number + ". "
		available := width - theme.DisplayWidth(prefix)
		if available < 8 {
			available = 8
		}
		wrapped := wrapText(option, available)
		if len(wrapped) == 0 {
			wrapped = []string{""}
		}
		lines = append(lines, theme.AccentStyle().Render(prefix)+theme.BaseStyle().Render(wrapped[0]))
		continuation := strings.Repeat(" ", theme.DisplayWidth(prefix))
		for _, line := range wrapped[1:] {
			lines = append(lines, theme.MutedStyle().Render(continuation)+theme.BaseStyle().Render(line))
		}
	}
	return lines
}

// renderQuestionHint 渲染 ask_user 的提交和取消提示。
func (c *CommandPrompt) renderQuestionHint() string {
	limit := len(c.state.Input.Options)
	rangeText := "输入"
	if limit > 0 {
		rangeText = fmt.Sprintf("1-%d", limit)
	}
	return theme.AccentStyle().Render("["+rangeText+"]") + " " + theme.SubtleStyle().Render("输入数字选择") +
		"  " + theme.AccentStyle().Render("[Enter]") + " " + theme.SubtleStyle().Render("确认") +
		"  " + theme.AccentStyle().Render("[Esc]") + " " + theme.SubtleStyle().Render("取消")
}

// modeLine 渲染输入模式、会话名和当前模型，并把右侧信息固定到行尾。
func (c *CommandPrompt) modeLine() string {
	left := fmt.Sprintf("[%s]", inputModeName(c.state.Mode))
	right := strings.TrimSpace(sessionTitle(c.state) + "   " + stringOrDash(c.state.Gateway.ActiveModel))
	width := c.contentWidth()
	if width <= 0 {
		return theme.SubtleStyle().Render(left + "   " + right)
	}
	gap := width - theme.DisplayWidth(left) - theme.DisplayWidth(right)
	if gap < 1 {
		return theme.SubtleStyle().Render(left + " " + right)
	}
	return theme.SubtleStyle().Render(left + strings.Repeat(" ", gap) + right)
}

// textWithCursor 返回在当前光标位置插入闪烁光标后的文本。
func (c *CommandPrompt) textWithCursor() string {
	runes := []rune(c.state.Input.Text)
	cursor := clampInt(c.state.Input.Cursor, 0, len(runes))
	symbol := " "
	if c.state.Input.CursorVisible {
		symbol = "_"
	}
	runes = append(runes[:cursor], append([]rune(symbol), runes[cursor:]...)...)
	return string(runes)
}

// insertRunes 在当前光标处插入可打印字符。
func (c *CommandPrompt) insertRunes(runes []rune) {
	if len(runes) == 0 {
		return
	}
	c.insertText(string(runes))
}

// insertText 在当前光标处插入文本，并按 rune 位置推进光标。
func (c *CommandPrompt) insertText(text string) {
	runes := []rune(c.state.Input.Text)
	cursor := clampInt(c.state.Input.Cursor, 0, len(runes))
	inserted := []rune(text)
	next := make([]rune, 0, len(runes)+len(inserted))
	next = append(next, runes[:cursor]...)
	next = append(next, inserted...)
	next = append(next, runes[cursor:]...)
	c.state.Input.Text = string(next)
	c.state.Input.Cursor = cursor + len(inserted)
	c.state.Input.CursorVisible = true
	c.state.Input.HistoryIndex = -1
}

// deleteBeforeCursor 删除光标前一个 rune。
func (c *CommandPrompt) deleteBeforeCursor() {
	runes := []rune(c.state.Input.Text)
	cursor := clampInt(c.state.Input.Cursor, 0, len(runes))
	if cursor == 0 {
		return
	}
	next := append([]rune(nil), runes[:cursor-1]...)
	next = append(next, runes[cursor:]...)
	c.state.Input.Text = string(next)
	c.state.Input.Cursor = cursor - 1
	c.state.Input.CursorVisible = true
}

// deleteAtCursor 删除光标所在 rune。
func (c *CommandPrompt) deleteAtCursor() {
	runes := []rune(c.state.Input.Text)
	cursor := clampInt(c.state.Input.Cursor, 0, len(runes))
	if cursor >= len(runes) {
		return
	}
	next := append([]rune(nil), runes[:cursor]...)
	next = append(next, runes[cursor+1:]...)
	c.state.Input.Text = string(next)
	c.state.Input.Cursor = cursor
	c.state.Input.CursorVisible = true
}

// moveCursor 按 rune 宽度移动光标，避免中文输入时 byte 位置错乱。
func (c *CommandPrompt) moveCursor(delta int) {
	c.state.Input.Cursor = clampInt(c.state.Input.Cursor+delta, 0, runeLen(c.state.Input.Text))
	c.state.Input.CursorVisible = true
}

// clearText 清空当前输入文本并重置光标。
func (c *CommandPrompt) clearText() {
	c.state.Input.Text = ""
	c.state.Input.Cursor = 0
	c.state.Input.CursorVisible = true
	c.state.Input.HistoryIndex = -1
}

// pushHistory 把已提交输入追加到历史，并避免连续重复项。
func (c *CommandPrompt) pushHistory(text string) {
	if text == "" {
		return
	}
	history := c.state.Input.History
	if len(history) == 0 || history[len(history)-1] != text {
		c.state.Input.History = append(history, text)
	}
	c.state.Input.HistoryIndex = -1
}

// previousHistory 在 Normal Mode 下切换到上一条输入历史。
func (c *CommandPrompt) previousHistory() {
	history := c.state.Input.History
	if len(history) == 0 {
		return
	}
	if c.state.Input.HistoryIndex < 0 {
		c.state.Input.HistoryIndex = len(history) - 1
	} else if c.state.Input.HistoryIndex > 0 {
		c.state.Input.HistoryIndex--
	}
	c.setText(history[c.state.Input.HistoryIndex])
}

// nextHistory 在 Normal Mode 下切换到下一条输入历史，越过尾部时清空输入。
func (c *CommandPrompt) nextHistory() {
	history := c.state.Input.History
	if len(history) == 0 || c.state.Input.HistoryIndex < 0 {
		return
	}
	c.state.Input.HistoryIndex++
	if c.state.Input.HistoryIndex >= len(history) {
		c.state.Input.HistoryIndex = -1
		c.setText("")
		return
	}
	c.setText(history[c.state.Input.HistoryIndex])
}

// setText 设置输入文本，并把光标放到文本末尾。
func (c *CommandPrompt) setText(text string) {
	c.state.Input.Text = text
	c.state.Input.Cursor = runeLen(text)
	c.state.Input.CursorVisible = true
}

// contentWidth 返回 Prompt 可用宽度，减一以避免终端自动换行。
func (c *CommandPrompt) contentWidth() int {
	if c.state.Layout.Width <= 0 {
		return 80
	}
	return c.state.Layout.Width - 1
}

// cursorBlinkCmd 创建下一次光标闪烁消息。
func cursorBlinkCmd() tea.Cmd {
	return tea.Tick(cursorBlinkInterval, func(time.Time) tea.Msg {
		return CursorBlinkMsg{}
	})
}

// emitMsg 把组件业务动作包装为 Bubble Tea 命令，让 App 统一处理。
func emitMsg(msg tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return msg
	}
}

// wrapText 按显示宽度把文本切为多行，保留中文和 ANSI 宽度安全。
func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	var lines []string
	remaining := text
	for remaining != "" {
		if theme.DisplayWidth(remaining) <= width {
			lines = append(lines, remaining)
			break
		}
		piece := theme.Truncate(remaining, width)
		if piece == "" {
			break
		}
		lines = append(lines, strings.TrimRight(piece, " "))
		remaining = strings.TrimLeft(strings.TrimPrefix(remaining, piece), " ")
		if remaining == text {
			_, size := utf8.DecodeRuneInString(remaining)
			remaining = remaining[size:]
		}
	}
	return lines
}

// sessionTitle 返回当前会话标题，缺失时回退为 Ghost Console 名称。
func sessionTitle(viewState *state.ViewState) string {
	if viewState.Gateway.ActiveSess != nil && viewState.Gateway.ActiveSess.Title != "" {
		return viewState.Gateway.ActiveSess.Title
	}
	return surfaceName
}

// inputModeName 将输入模式转换为稳定显示文本。
func inputModeName(mode state.InputMode) string {
	switch mode {
	case state.NormalMode:
		return "normal"
	case state.LeaderMode:
		return "leader"
	default:
		return "input"
	}
}

// runeLen 返回字符串的 rune 数量。
func runeLen(text string) int {
	return len([]rune(text))
}

// clampInt 将整数限制在给定闭区间内。
func clampInt(value int, min int, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
