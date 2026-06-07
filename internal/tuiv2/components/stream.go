package components

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"neo-code/internal/tuiv2/state"
	"neo-code/internal/tuiv2/theme"
)

const (
	streamHeaderRows      = 1
	streamReservedRows    = 7
	streamTimeGap         = 5 * time.Minute
	streamVirtualOverscan = 20
)

// AgentStream 渲染 Agent 行为流，包括消息、工具调用和状态条目。
type AgentStream struct {
	state *state.ViewState
}

var _ tea.Model = (*AgentStream)(nil)

// NewAgentStream 创建 Agent Stream 组件。
func NewAgentStream(viewState *state.ViewState) *AgentStream {
	return &AgentStream{state: viewState}
}

// Init 不启动额外命令，组件只读取共享 ViewState。
func (c *AgentStream) Init() tea.Cmd {
	return nil
}

// Update 处理 Agent Stream 的滚动按键，不维护冗余业务状态。
func (c *AgentStream) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return c, nil
	}
	maxOffset := c.maxScrollOffset()
	switch key.String() {
	case "k", "up":
		c.state.Layout.ScrollOffset = clampScroll(c.state.Layout.ScrollOffset+1, maxOffset)
		c.state.Layout.AutoScroll = false
	case "j", "down":
		c.state.Layout.ScrollOffset = clampScroll(c.state.Layout.ScrollOffset-1, maxOffset)
		c.state.Layout.AutoScroll = c.state.Layout.ScrollOffset == 0
	case "g":
		c.state.Layout.ScrollOffset = maxOffset
		c.state.Layout.AutoScroll = false
	case "G":
		c.state.Layout.ScrollOffset = 0
		c.state.Layout.AutoScroll = true
	case "ctrl+u":
		halfPage := c.halfPageSize()
		c.state.Layout.ScrollOffset = clampScroll(c.state.Layout.ScrollOffset+halfPage, maxOffset)
		c.state.Layout.AutoScroll = false
	case "ctrl+d":
		halfPage := c.halfPageSize()
		c.state.Layout.ScrollOffset = clampScroll(c.state.Layout.ScrollOffset-halfPage, maxOffset)
		c.state.Layout.AutoScroll = c.state.Layout.ScrollOffset == 0
	}
	return c, nil
}

// View 渲染 Agent Stream，按滚动窗口选择可见条目并进行宽度安全截断。
func (c *AgentStream) View() string {
	width := c.streamWidth()
	lines := []string{theme.MutedStyle().Render(c.headerText())}
	rendered := c.renderAllEntries()
	if len(rendered) == 0 {
		rendered = []string{
			theme.AccentStyle().Render("  " + theme.StatusSymbol(theme.PhaseRunning) + " 我可以帮你做什么？"),
			theme.MutedStyle().Render("  " + theme.StatusSymbol(theme.PhaseIdle) + " " + surfaceName),
		}
	}
	lines = append(lines, c.visibleLines(rendered)...)
	content := strings.Join(lines, "\n")
	if width > 0 {
		return fitBlock(content, width, true)
	}
	return content
}

// headerText 渲染 Stream 标题，并在手动滚动时显示偏移量。
func (c *AgentStream) headerText() string {
	if c.state.Layout.AutoScroll {
		return "Agent Stream"
	}
	return fmt.Sprintf("Agent Stream  scroll:%d", c.state.Layout.ScrollOffset)
}

// streamWidth 根据布局断点计算 Agent Stream 可用宽度。
func (c *AgentStream) streamWidth() int {
	width := c.state.Layout.Width
	if width >= 100 && c.state.Layout.ShowInspector {
		return width - c.state.Layout.InspectorWidth - 3
	}
	return width
}

// visibleLineCount 根据终端高度估算可展示的流行数。
func (c *AgentStream) visibleLineCount() int {
	height := c.state.Layout.Height
	if height <= 0 {
		return 8
	}
	limit := height - streamReservedRows - streamHeaderRows
	if limit < 4 {
		return 4
	}
	return limit
}

// halfPageSize 返回半页滚动所需的行数，至少为 1。
func (c *AgentStream) halfPageSize() int {
	h := c.visibleLineCount() / 2
	if h < 1 {
		return 1
	}
	return h
}

// maxScrollOffset 计算当前渲染内容允许的最大手动滚动偏移。
func (c *AgentStream) maxScrollOffset() int {
	lines := len(c.renderAllEntries())
	visible := c.visibleLineCount()
	if lines <= visible {
		return 0
	}
	return lines - visible
}

// visibleLines 根据滚动状态截取最终可见行。
func (c *AgentStream) visibleLines(lines []string) []string {
	visible := c.visibleLineCount()
	if len(lines) <= visible {
		c.state.Layout.ScrollOffset = 0
		return lines
	}
	maxOffset := len(lines) - visible
	if c.state.Layout.AutoScroll {
		c.state.Layout.ScrollOffset = 0
	}
	c.state.Layout.ScrollOffset = clampScroll(c.state.Layout.ScrollOffset, maxOffset)
	end := len(lines) - c.state.Layout.ScrollOffset
	start := end - visible
	if start < 0 {
		start = 0
	}
	return lines[start:end]
}

// renderAllEntries 将 StreamEntry 序列转换为完整的待裁剪行集合。
func (c *AgentStream) renderAllEntries() []string {
	entries := c.virtualEntries()
	lines := make([]string, 0, len(entries)*2)
	var previous *state.StreamEntry
	for index := range entries {
		entry := entries[index]
		if shouldSeparate(previous, &entry) && len(lines) > 0 {
			lines = append(lines, "")
		}
		if shouldShowTimestamp(previous, &entry) {
			lines = append(lines, c.renderTimestamp(entry.Timestamp))
		}
		lines = append(lines, c.renderEntry(entry)...)
		previous = &entry
	}
	return lines
}

// virtualEntries 为超长 stream 预留虚拟化窗口，避免每次渲染处理全部历史。
func (c *AgentStream) virtualEntries() []state.StreamEntry {
	entries := c.state.Stream
	if len(entries) <= 1000 {
		return entries
	}
	visible := c.visibleLineCount() + streamVirtualOverscan*2
	if visible > len(entries) {
		visible = len(entries)
	}
	end := len(entries) - c.state.Layout.ScrollOffset
	if end > len(entries) {
		end = len(entries)
	}
	if end < visible {
		end = visible
	}
	start := end - visible
	if start < 0 {
		start = 0
	}
	return entries[start:end]
}

// renderEntry 将单条 StreamEntry 渲染为一种或多种 Ghost Console 行。
func (c *AgentStream) renderEntry(entry state.StreamEntry) []string {
	switch entry.Type {
	case "message":
		return c.renderMessage(entry)
	case "tool_start":
		return c.renderToolStart(entry)
	case "tool_end":
		return c.renderToolEnd(entry)
	case "tool_output":
		return c.renderToolOutput(entry)
	case "permission", "permission_requested":
		return c.renderPermission(entry)
	case "question", "ask_user_question", "user_question_requested":
		return c.renderQuestion(entry)
	case "status", "run_started", "run_finished", "run_cancelled", "phase_changed", "session_updated", "model_changed", "health_changed":
		return c.renderStatus(entry)
	case "error", "run_error", "gateway_offline":
		return c.renderError(entry)
	default:
		return c.renderStatus(entry)
	}
}

// renderMessage 渲染角色感知的消息正文，支持换行。
func (c *AgentStream) renderMessage(entry state.StreamEntry) []string {
	role := ""
	if v, ok := entry.Metadata["role"].(string); ok {
		role = v
	}
	var label string
	var styledLines []string
	text := entry.Content
	if text == "" {
		text = "-"
	}
	switch role {
	case "user":
		label = "  " + theme.InfoStyle().Render("you") + " "
		styledLines = renderWrappedLines(text, "  ", theme.BaseStyle())
	default: // "assistant" or empty
		label = "  " + theme.AccentStyle().Render("neo") + " "
		styledLines = renderWrappedLines(text, "  ", theme.BaseStyle())
	}
	result := []string{label + strings.TrimPrefix(styledLines[0], "  ")}
	for _, line := range styledLines[1:] {
		result = append(result, line)
	}
	return result
}

// renderToolStart 渲染工具调用开始行。
func (c *AgentStream) renderToolStart(entry state.StreamEntry) []string {
	toolName := stringOrDash(entry.ToolName)
	content := entry.Content
	if content == "" {
		content = entry.ToolInput
	}
	line := theme.AccentStyle().Render("  "+theme.StreamPrefix("tool_start")+" tool."+toolName) +
		theme.MutedStyle().Render(" "+theme.Separator()+" ") +
		renderToolContent(content)
	return []string{line}
}

// renderToolEnd 渲染工具调用完成行。
func (c *AgentStream) renderToolEnd(entry state.StreamEntry) []string {
	line := theme.SuccessStyle().Render("  " + theme.StreamPrefix("tool_end") + " tool." + stringOrDash(entry.ToolName))
	if entry.Content != "" {
		line += theme.MutedStyle().Render(" " + theme.Separator() + " " + entry.Content)
	}
	return []string{line}
}

// renderToolOutput 渲染工具输出内容，使用缩进指示条。
func (c *AgentStream) renderToolOutput(entry state.StreamEntry) []string {
	content := entry.Content
	if content == "" {
		content = "-"
	}
	return renderWrappedLines(content, "  "+theme.AccentBar()+" ", theme.CodeBlockStyle())
}

// renderPermission 渲染权限请求状态行。
func (c *AgentStream) renderPermission(entry state.StreamEntry) []string {
	return []string{theme.WarningStyle().Render("  " + theme.StreamPrefix("permission_requested") + " " + stringOrDash(entry.Content))}
}

// renderQuestion 渲染 ask_user 问题行。
func (c *AgentStream) renderQuestion(entry state.StreamEntry) []string {
	return []string{theme.MutedStyle().Render("  " + theme.Separator() + " " + stringOrDash(entry.Content))}
}

// renderStatus 渲染普通状态变更行。
func (c *AgentStream) renderStatus(entry state.StreamEntry) []string {
	return []string{theme.MutedStyle().Render("  " + theme.StreamPrefix(entry.Type) + " " + stringOrDash(entry.Content))}
}

// renderError 渲染错误状态行。
func (c *AgentStream) renderError(entry state.StreamEntry) []string {
	return []string{theme.ErrorStyle().Render("  " + theme.StreamPrefix("error") + " " + stringOrDash(entry.Content))}
}

// renderTimestamp 渲染长时间间隔分隔时间戳。
func (c *AgentStream) renderTimestamp(timestamp time.Time) string {
	if timestamp.IsZero() {
		return ""
	}
	return theme.TimestampStyle().Render("  " + timestamp.Format("15:04"))
}

// renderWrappedLines 按内容换行拆分并为每行添加前缀和样式。
func renderWrappedLines(content string, prefix string, style interface{ Render(...string) string }) []string {
	parts := strings.Split(content, "\n")
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		lines = append(lines, style.Render(prefix+part))
	}
	return lines
}

// renderToolContent 根据内容形态选择文件路径或普通弱文本样式。
func renderToolContent(content string) string {
	if content == "" {
		return theme.MutedStyle().Render("-")
	}
	if strings.Contains(content, "/") || strings.Contains(content, ".") {
		return theme.FilePathStyle().Render(content)
	}
	return theme.MutedStyle().Render(content)
}

// shouldSeparate 判断相邻条目之间是否需要空行分组。
func shouldSeparate(previous *state.StreamEntry, current *state.StreamEntry) bool {
	if previous == nil || current == nil {
		return false
	}
	if previous.Type == "tool_start" && (current.Type == "tool_end" || current.Type == "tool_output") {
		return false
	}
	if previous.Type == "tool_output" && current.Type == "tool_end" {
		return false
	}
	return previous.Type != current.Type
}

// shouldShowTimestamp 判断相邻条目之间是否需要时间戳分隔。
func shouldShowTimestamp(previous *state.StreamEntry, current *state.StreamEntry) bool {
	if previous == nil || current == nil || previous.Timestamp.IsZero() || current.Timestamp.IsZero() {
		return false
	}
	return current.Timestamp.Sub(previous.Timestamp) > streamTimeGap
}

// clampScroll 将滚动偏移限制在可见窗口允许范围内。
func clampScroll(value int, max int) int {
	if value < 0 {
		return 0
	}
	if value > max {
		return max
	}
	return value
}

// stringOrDash 在占位布局中用短横线表示空值。
func stringOrDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
