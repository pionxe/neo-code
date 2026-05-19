package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"neo-code/internal/tuiv2/state"
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

// Update 当前不维护组件私有业务状态，只保留 tea.Model 契约。
func (c *AgentStream) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return c, nil
}

// View 渲染 Agent Stream，占位阶段用缩进和状态符号表达层级。
func (c *AgentStream) View() string {
	width := c.streamWidth()
	lines := []string{statusMuted.Render("Agent Stream")}
	if len(c.state.Stream) == 0 {
		lines = append(lines, statusBlue.Render("  ◉ 我可以帮你做什么？"))
		lines = append(lines, statusMuted.Render("  ○ "+surfaceName))
	} else {
		for _, entry := range tailEntries(c.state.Stream, c.visibleEntries()) {
			lines = append(lines, c.renderEntry(entry))
		}
	}
	content := strings.Join(lines, "\n")
	if width > 0 {
		return fitBlock(content, width, true)
	}
	return content
}

// streamWidth 根据布局断点计算 Agent Stream 可用宽度。
func (c *AgentStream) streamWidth() int {
	width := c.state.Layout.Width
	if width >= 100 && c.state.Layout.ShowInspector {
		return width - c.state.Layout.InspectorWidth - 3
	}
	return width
}

// visibleEntries 根据终端高度估算可展示的流条目数量。
func (c *AgentStream) visibleEntries() int {
	height := c.state.Layout.Height
	if height <= 0 {
		return 8
	}
	limit := height - 8
	if limit < 4 {
		return 4
	}
	return limit
}

// renderEntry 将单条 StreamEntry 渲染为带状态符号的文本行。
func (c *AgentStream) renderEntry(entry state.StreamEntry) string {
	content := entry.Content
	if content == "" {
		content = stringOrDash(entry.Type)
	}
	switch entry.Type {
	case "tool_started", "tool_start":
		return statusBlue.Render("  ◉ tool."+stringOrDash(entry.ToolName)) + statusMuted.Render(" · "+content)
	case "tool_finished", "tool_end":
		return statusGreen.Render("  ✓ tool."+stringOrDash(entry.ToolName)) + statusMuted.Render(" · "+content)
	case "permission_requested", "question":
		return statusBlue.Render("  ◉ "+entry.Type) + statusMuted.Render(" · "+content)
	case "error", "gateway_offline":
		return statusRed.Render("  × " + content)
	default:
		return statusMuted.Render(fmt.Sprintf("  %s %s", streamSymbol(entry.Type), content))
	}
}

// streamSymbol 返回指定流条目的语义状态符号。
func streamSymbol(entryType string) string {
	switch entryType {
	case "message":
		return "○"
	case "run_finished", "run_cancelled":
		return "✓"
	case "assistant_delta", "run_started":
		return "◉"
	default:
		return "◌"
	}
}

// tailEntries 返回最近的流条目，避免静态布局在小终端中过长。
func tailEntries(entries []state.StreamEntry, limit int) []state.StreamEntry {
	if len(entries) <= limit {
		return entries
	}
	return entries[len(entries)-limit:]
}

// stringOrDash 在占位布局中用短横线表示空值。
func stringOrDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
