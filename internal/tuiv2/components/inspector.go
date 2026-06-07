package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"neo-code/internal/tuiv2/state"
	"neo-code/internal/tuiv2/theme"
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

// View 渲染会话、上下文、token、工具和文件详情；窄屏隐藏由 App 布局控制。
func (c *SoftInspector) View() string {
	if !c.state.Layout.ShowInspector {
		return ""
	}
	width := c.state.Layout.InspectorWidth
	if width <= 0 {
		width = c.state.Layout.Width
	}
	lines := []string{theme.MutedStyle().Render("Soft Inspector")}
	lines = append(lines, c.sessionLines()...)
	lines = append(lines, "", theme.MutedStyle().Render("Context"))
	lines = append(lines, c.contextLine())
	lines = append(lines, "", theme.MutedStyle().Render("Token Usage"))
	lines = append(lines, fmt.Sprintf("  ↑ %d ↓ %d", c.state.Runtime.Tokens.Input, c.state.Runtime.Tokens.Output))
	lines = append(lines, c.toolLines()...)
	lines = append(lines, c.fileLines()...)
	content := strings.Join(lines, "\n")
	if width > 0 {
		return fitBlock(content, width, true)
	}
	return content
}

// sessionLines 渲染会话列表的压缩摘要。
func (c *SoftInspector) sessionLines() []string {
	lines := []string{"", theme.MutedStyle().Render("Session")}
	if len(c.state.Gateway.Sessions) == 0 {
		return append(lines, "  "+theme.Separator()+" none")
	}
	for index, session := range c.state.Gateway.Sessions {
		if index >= 3 {
			lines = append(lines, fmt.Sprintf("  %s +%d more", theme.Separator(), len(c.state.Gateway.Sessions)-index))
			break
		}
		lines = append(lines, "  "+theme.Separator()+" "+session.Title)
	}
	return lines
}

// contextLine 渲染上下文用量占位文本。
func (c *SoftInspector) contextLine() string {
	total := c.state.Runtime.Tokens.Total
	if total == 0 {
		return "  " + theme.AccentBar() + "0/100k"
	}
	return fmt.Sprintf("  %s%d/100k", theme.AccentBar(), total)
}

// toolLines 渲染当前活跃工具（已启动但未完成的工具调用）。
func (c *SoftInspector) toolLines() []string {
	lines := []string{"", theme.MutedStyle().Render("Active Tools")}
	activeTools := c.activeToolNames()
	if len(activeTools) == 0 {
		return append(lines, "  "+theme.Separator()+" idle")
	}
	for _, name := range activeTools {
		lines = append(lines, "  "+theme.Separator()+" "+theme.ToolNameStyle().Render("tool."+name))
	}
	return lines
}

// activeToolNames 扫描 Stream 查找已启动但未完成的工具调用。
func (c *SoftInspector) activeToolNames() []string {
	ended := make(map[string]bool)
	var started []string
	for _, entry := range c.state.Stream {
		switch entry.Type {
		case "tool_start":
			if entry.ToolName != "" {
				started = append(started, entry.ToolName)
			}
		case "tool_end":
			if entry.ToolName != "" {
				ended[entry.ToolName] = true
			}
		}
	}
	var active []string
	for _, name := range started {
		if !ended[name] {
			active = append(active, name)
		}
	}
	return active
}

// fileLines 渲染工具修改过的文件路径列表，使用 DiffAdd/DiffDel 配色。
func (c *SoftInspector) fileLines() []string {
	lines := []string{"", theme.MutedStyle().Render("Files")}
	fileEntries := c.fileEntries()
	if len(fileEntries) == 0 {
		return append(lines, "  "+theme.Separator()+" none")
	}
	for _, fe := range fileEntries {
		lines = append(lines, "  "+theme.Separator()+" "+fe)
	}
	return lines
}

// fileEntries 扫描 Stream 中的 tool_end 条目，提取文件路径并使用 DiffAdd/DiffDel 配色。
func (c *SoftInspector) fileEntries() []string {
	seen := make(map[string]bool)
	var entries []string
	palette := theme.TokyoNight
	for _, entry := range c.state.Stream {
		if entry.Type != "tool_end" || entry.Content == "" {
			continue
		}
		path := extractFilePath(entry.Content)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		color := palette.DiffAdd
		if strings.Contains(entry.Content, "delete") || strings.Contains(entry.Content, "remove") {
			color = palette.DiffDel
		}
		styled := lipgloss.NewStyle().Foreground(color).Render(path)
		entries = append(entries, styled)
	}
	return entries
}

// extractFilePath 从工具输出内容中提取文件路径（包含 / 或 . 的首段）。
func extractFilePath(content string) string {
	for _, part := range strings.Fields(content) {
		if strings.Contains(part, "/") || (strings.Contains(part, ".") && len(part) > 1) {
			return part
		}
	}
	return ""
}
