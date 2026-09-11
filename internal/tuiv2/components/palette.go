package components

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"neo-code/internal/tuiv2/state"
	"neo-code/internal/tuiv2/theme"
)

// PaletteItem 是 CommandDef 的类型别名，保持向后兼容（picker_test 等仍可用）。
type PaletteItem = CommandDef

// PaletteCommandMsg 表示用户选择了某个命令面板项。
//
// 携带 Name（用户可见标识，用于日志/提示）与 Action（具名常量，用于 app 层路由），
// 消除 handlePaletteCommand 对字符串命令名的硬编码依赖。
type PaletteCommandMsg struct {
	Name   string
	Action PaletteAction
}

// Palette 是 Telescope 风格的命令面板组件。
type Palette struct {
	state *state.ViewState
}

var _ tea.Model = (*Palette)(nil)

// NewPalette 创建命令面板组件。
func NewPalette(viewState *state.ViewState) *Palette {
	return &Palette{state: viewState}
}

// Init 不启动额外命令。
func (p *Palette) Init() tea.Cmd {
	return nil
}

// Update 处理命令面板内的键盘、鼠标和导航。
func (p *Palette) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return p, p.handleKey(msg)
	case tea.MouseMsg:
		return p, p.handleMouse(msg)
	}
	return p, nil
}

func (p *Palette) handleKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "ctrl+c":
		p.state.Overlay.Active = state.OverlayNone
		p.state.Overlay.Query = ""
		p.state.Overlay.Selected = 0
		return nil
	case "enter", " ":
		matched := p.matchedItems()
		if len(matched) == 0 {
			return nil
		}
		idx := p.state.Overlay.Selected
		if idx >= len(matched) {
			idx = len(matched) - 1
		}
		selected := matched[idx]
		p.state.Overlay.Active = state.OverlayNone
		p.state.Overlay.Query = ""
		p.state.Overlay.Selected = 0
		return emitMsg(PaletteCommandMsg{Name: selected.Name, Action: selected.Action})
	case "up", "ctrl+k":
		if p.state.Overlay.Selected > 0 {
			p.state.Overlay.Selected--
		}
		return nil
	case "down", "ctrl+j":
		matched := p.matchedItems()
		if p.state.Overlay.Selected < len(matched)-1 {
			p.state.Overlay.Selected++
		}
		return nil
	case "backspace":
		// 用 deleteLastRune 正确处理多字节 UTF-8（中文/emoji），与 cmdline 一致。
		p.state.Overlay.Query = deleteLastRune(p.state.Overlay.Query)
		p.state.Overlay.Selected = 0
		return nil
	default:
		runes := msg.Runes
		if len(runes) > 0 && runes[0] >= 32 {
			p.state.Overlay.Query += string(runes)
			p.state.Overlay.Selected = 0
		}
		return nil
	}
}

// matchedItems 按确定性的优先级匹配命令：先精确名、再前缀、最后子串。
// 不使用模糊匹配，避免 "mode" 因为评分排序命中 /model 而不是 /mode。
func (p *Palette) matchedItems() []PaletteItem {
	query := strings.ToLower(strings.TrimPrefix(p.state.Overlay.Query, "/"))
	all := PaletteCommands()
	if query == "" {
		return all
	}
	var exact, prefix, substr []PaletteItem
	for _, item := range all {
		name := strings.ToLower(strings.TrimPrefix(item.Name, "/"))
		desc := strings.ToLower(item.Description)
		switch {
		case name == query:
			exact = append(exact, item)
		case strings.HasPrefix(name, query):
			prefix = append(prefix, item)
		case strings.Contains(name, query) || strings.Contains(desc, query):
			substr = append(substr, item)
		}
	}
	return append(append(exact, prefix...), substr...)
}

// handleMouse 处理鼠标滚轮和点击事件。
func (p *Palette) handleMouse(msg tea.MouseMsg) tea.Cmd {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if p.state.Overlay.Selected > 0 {
			p.state.Overlay.Selected--
		}
		return nil
	case tea.MouseButtonWheelDown:
		matched := p.matchedItems()
		if p.state.Overlay.Selected < len(matched)-1 {
			p.state.Overlay.Selected++
		}
		return nil
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return nil
		}
		// query + 空行 = 2 行头部
		itemIdx := msg.Y - 2
		matched := p.matchedItems()
		if itemIdx >= 0 && itemIdx < len(matched) {
			selected := matched[itemIdx]
			p.state.Overlay.Active = state.OverlayNone
			p.state.Overlay.Query = ""
			p.state.Overlay.Selected = 0
			return emitMsg(PaletteCommandMsg{Name: selected.Name, Action: selected.Action})
		}
		return nil
	}
	return nil
}

// View 渲染 Telescope 风格的命令面板。
func (p *Palette) View() string {
	width := p.state.Layout.Width
	height := p.state.Layout.Height
	if width <= 0 {
		width = 60
	}
	if height <= 0 {
		height = 24
	}

	matched := p.matchedItems()
	boxW := min(width-4, 60)
	boxH := height - 4
	if boxH < 8 {
		boxH = 8
	}
	maxItems := boxH - 5 // title + query + hint + padding
	if maxItems < 1 {
		maxItems = 1
	}

	var lines []string

	// 搜索输入行
	queryLine := "> " + p.state.Overlay.Query
	queryLine = theme.AccentStyle().Render(queryLine)
	lines = append(lines, queryLine, "")

	// 选项列表：铺平展示，每行 = 选中标记 + Name + Description + 右对齐 Shortcut
	for i, item := range matched {
		if i >= maxItems {
			break
		}
		prefix := "  "
		name := item.Name
		desc := theme.MutedStyle().Render(item.Description)
		shortcut := theme.SubtleStyle().Render(item.Shortcut)

		if i == p.state.Overlay.Selected {
			prefix = theme.AccentStyle().Render("▎ ")
			name = theme.AccentStyle().Bold(true).Render(name)
		}
		// 左侧内容（标记+名+描述），右侧快捷键右对齐
		left := prefix + name + "  " + desc
		leftW := theme.DisplayWidth(left)
		shortcutW := theme.DisplayWidth(item.Shortcut)
		// 计算填充：boxW - 左侧 - 快捷键 - 间隔
		gap := (boxW - 2) - leftW - shortcutW - 2
		if gap < 1 {
			gap = 1
		}
		// 若空间不足，先截断左侧
		if leftW+shortcutW+3 > boxW-2 {
			left = theme.Truncate(left, (boxW-2)-shortcutW-3)
		}
		line := left + strings.Repeat(" ", gap) + shortcut
		lines = append(lines, line)
	}

	if len(matched) == 0 {
		lines = append(lines, theme.MutedStyle().Render("  No matches found"))
	}

	// 底部提示行
	hint := "  ⏎ / ␣ : execute   ␛ : dismiss"
	lines = append(lines, "", theme.MutedStyle().Render(hint))

	// 边框容器
	content := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Width(boxW).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Render(content)

	// 居中
	outerH := max(boxH, 10)
	return lipgloss.NewStyle().
		Width(width).
		Height(outerH).
		Align(lipgloss.Center, lipgloss.Center).
		Render(box)
}
