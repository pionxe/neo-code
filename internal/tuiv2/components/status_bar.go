// Package components 提供 TUI v2 Ghost Console 的静态布局组件。
package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"neo-code/internal/tuiv2/state"
)

const surfaceName = "ghost-console"

var (
	statusBlue  = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7"))
	statusGreen = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a"))
	statusRed   = lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e"))
	statusMuted = lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
)

// AmbientStatus 渲染连接状态、会话名、模型名、token 用量和运行态摘要。
type AmbientStatus struct {
	state *state.ViewState
}

var _ tea.Model = (*AmbientStatus)(nil)

// NewAmbientStatus 创建顶部环境状态组件。
func NewAmbientStatus(viewState *state.ViewState) *AmbientStatus {
	return &AmbientStatus{state: viewState}
}

// Init 不启动额外命令，组件只读取共享 ViewState。
func (c *AmbientStatus) Init() tea.Cmd {
	return nil
}

// Update 当前不维护组件私有业务状态，只保留 tea.Model 契约。
func (c *AmbientStatus) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return c, nil
}

// View 渲染单行 Ambient Status，不使用边框或药丸标签。
func (c *AmbientStatus) View() string {
	parts := []string{
		statusBlue.Render("NEOCODE"),
		c.phase(),
		statusMuted.Render(surfaceName),
		statusMuted.Render(c.model()),
		statusMuted.Render(c.tokens()),
	}
	if c.state.Gateway.ActiveSess != nil {
		parts = append(parts, statusMuted.Render(c.state.Gateway.ActiveSess.Title))
	}
	line := strings.Join(parts, "   ")
	if c.state.Layout.Width > 0 {
		return fitBlock(line, c.state.Layout.Width, true)
	}
	return line
}

// phase 根据 Runtime phase 渲染顶部运行态。
func (c *AmbientStatus) phase() string {
	phase := c.state.Runtime.Phase
	switch phase {
	case state.RuntimePhaseRunning, state.RuntimePhaseWaitingPermission, state.RuntimePhaseWaitingUser:
		return statusBlue.Render("◉ " + phase)
	case state.RuntimePhaseError:
		return statusRed.Render("× " + phase)
	case state.RuntimePhaseCancelled:
		return statusMuted.Render("◌ " + phase)
	default:
		return statusGreen.Render("○ " + phase)
	}
}

// model 返回当前活动模型的显示文本。
func (c *AmbientStatus) model() string {
	if c.state.Gateway.ActiveModel != "" {
		return c.state.Gateway.ActiveModel
	}
	return "model:-"
}

// tokens 返回 token 用量的紧凑显示文本。
func (c *AmbientStatus) tokens() string {
	tokens := c.state.Runtime.Tokens
	return fmt.Sprintf("↑ %d ↓ %d · %d", tokens.Input, tokens.Output, tokens.Total)
}
