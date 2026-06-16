package components

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"neo-code/internal/tuiv2/state"
	"neo-code/internal/tuiv2/theme"
)

// ConfirmYesMsg 表示用户确认了危险操作。
type ConfirmYesMsg struct{}

// ConfirmNoMsg 表示用户取消了危险操作。
type ConfirmNoMsg struct{}

// ConfirmOverlay 是危险操作确认弹窗组件。
type ConfirmOverlay struct {
	state *state.ViewState
}

var _ tea.Model = (*ConfirmOverlay)(nil)

// NewConfirmOverlay 创建确认弹窗组件。
func NewConfirmOverlay(viewState *state.ViewState) *ConfirmOverlay {
	return &ConfirmOverlay{state: viewState}
}

// Init 不启动额外命令。
func (c *ConfirmOverlay) Init() tea.Cmd {
	return nil
}

// Update 处理确认弹窗的键盘输入。
func (c *ConfirmOverlay) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return c, nil
	}
	switch key.String() {
	case "y", "enter":
		return c, func() tea.Msg { return ConfirmYesMsg{} }
	case "n", "esc", "ctrl+c":
		return c, func() tea.Msg { return ConfirmNoMsg{} }
	}
	return c, nil
}

// View 渲染确认弹窗。
func (c *ConfirmOverlay) View() string {
	width := c.state.Layout.Width
	height := c.state.Layout.Height
	if width <= 0 {
		width = 60
	}
	if height <= 0 {
		height = 24
	}

	boxW := min(width-4, 48)

	confirm := c.state.Confirm

	var lines []string
	lines = append(lines, theme.WarningStyle().Bold(true).Render("  "+confirm.Title))
	lines = append(lines, "")
	lines = append(lines, "  "+confirm.Message)
	lines = append(lines, "")
	lines = append(lines, theme.MutedStyle().Render("  [Y] confirm   [N] cancel"))

	content := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Width(boxW).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("196")).
		Padding(0, 1).
		Render(content)

	boxH := min(height-2, 14)
	if boxH < 6 {
		boxH = 6
	}
	return lipgloss.NewStyle().
		Width(width).
		Height(boxH).
		Align(lipgloss.Center, lipgloss.Center).
		Render(box)
}
