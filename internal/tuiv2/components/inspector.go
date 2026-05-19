package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"neo-code/internal/tuiv2/state"
)

// SoftInspector 渲染响应式右侧弱信息列。
type SoftInspector struct {
	state *state.ViewState
}

var _ tea.Model = (*SoftInspector)(nil)

// NewSoftInspector 创建 Soft Inspector 组件。
func NewSoftInspector(viewState *state.ViewState) *SoftInspector {
	return &SoftInspector{state: viewState}
}

// Init 不启动额外命令，组件只读取共享 ViewState。
func (c *SoftInspector) Init() tea.Cmd {
	return nil
}

// Update 当前不维护组件私有业务状态，只保留 tea.Model 契约。
func (c *SoftInspector) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return c, nil
}

// View 渲染会话、上下文和 token 详情；窄屏隐藏由 App 布局控制。
func (c *SoftInspector) View() string {
	if !c.state.Layout.ShowInspector {
		return ""
	}
	width := c.state.Layout.InspectorWidth
	if width <= 0 {
		width = c.state.Layout.Width
	}
	lines := []string{statusMuted.Render("Soft Inspector")}
	lines = append(lines, c.sessionLines()...)
	lines = append(lines, "", statusMuted.Render("Context"))
	lines = append(lines, c.contextLine())
	lines = append(lines, "", statusMuted.Render("Token Usage"))
	lines = append(lines, fmt.Sprintf("  ↑ %d ↓ %d", c.state.Runtime.Tokens.Input, c.state.Runtime.Tokens.Output))
	content := strings.Join(lines, "\n")
	if width > 0 {
		return fitBlock(content, width, true)
	}
	return content
}

// sessionLines 渲染会话列表的压缩摘要。
func (c *SoftInspector) sessionLines() []string {
	lines := []string{"", statusMuted.Render("Session")}
	if len(c.state.Gateway.Sessions) == 0 {
		return append(lines, "  · none")
	}
	for index, session := range c.state.Gateway.Sessions {
		if index >= 3 {
			lines = append(lines, fmt.Sprintf("  · +%d more", len(c.state.Gateway.Sessions)-index))
			break
		}
		lines = append(lines, "  · "+session.Title)
	}
	return lines
}

// contextLine 渲染上下文用量占位文本。
func (c *SoftInspector) contextLine() string {
	total := c.state.Runtime.Tokens.Total
	if total == 0 {
		return "  ▎0/100k"
	}
	return fmt.Sprintf("  ▎%d/100k", total)
}
