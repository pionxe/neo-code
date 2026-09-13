// Package components 提供 TUI v2 Ghost Console 的静态布局组件。
package components

import (
	"fmt"
	"strings"

	"neo-code/internal/tuiv2/state"
	"neo-code/internal/tuiv2/theme"
)

const surfaceName = "ghost-console"

// AmbientStatus 渲染连接状态、会话名、模型名、token 用量和运行态摘要。
// 非 tea.Model：内核路径下仅为 statusbar 插件的渲染委托（S7 真相源
// 统一——宽度来自 Render 参数，与 kernel.go 传参同源，修首帧错位）。
type AmbientStatus struct {
	state *state.ViewState
}

// NewAmbientStatus 创建顶部环境状态组件。
func NewAmbientStatus(viewState *state.ViewState) *AmbientStatus {
	return &AmbientStatus{state: viewState}
}

// View 渲染单行 Ambient Status，不使用边框或药丸标签。
// width 为唯一宽度真相源（statusbar 插件自 kernel Render 参数透传——
// S7 真相源统一，消除 Layout.Width 首帧零值错位）。
func (c *AmbientStatus) View(width int) string {
	parts := []string{
		theme.AccentStyle().Render("NEOCODE"),
		c.phase(),
		theme.MutedStyle().Render(surfaceName),
		theme.MutedStyle().Render(c.model()),
		theme.MutedStyle().Render(c.tokens()),
	}
	if c.state.Gateway.ActiveSess != nil {
		parts = append(parts, theme.MutedStyle().Render(c.state.Gateway.ActiveSess.Title))
	}
	// 断连持久指示（ADR-005 断连视觉：Connected 槽写权归 health 插件）：
	// 离线时以错误样式显示 offline 段，恢复后自然消失。
	if !c.state.Gateway.Connected {
		parts = append(parts, theme.ErrorStyle().Render("offline"))
	}
	// 弱提示（ADR-012）：内核经 Host.Notify 写入 Notify 槽，Text 非空即渲染。
	// 组件不自算过期——到期清除责在内核 notifyExpiryMsg（issue #41 审计 P2-1
	// 裁定：过期判断收在内核单一出处，组件保持纯读）。
	if c.state.Notify.Text != "" {
		parts = append(parts, theme.AccentStyle().Render(c.state.Notify.Text))
	}
	line := strings.Join(parts, "   ")
	if width > 0 {
		return fitBlock(line, width, true)
	}
	return line
}

// phase 根据 Runtime phase 渲染顶部运行态。
func (c *AmbientStatus) phase() string {
	phase := c.state.Runtime.Phase
	switch phase {
	case state.RuntimePhaseRunning, state.RuntimePhaseWaitingPermission, state.RuntimePhaseWaitingUser:
		return theme.AccentStyle().Render(theme.StatusSymbol(theme.PhaseRunning) + " " + phase)
	case state.RuntimePhaseError:
		return theme.ErrorStyle().Render(theme.StatusSymbol(theme.PhaseError) + " " + phase)
	case state.RuntimePhaseCancelled:
		return theme.MutedStyle().Render(theme.StatusSymbol(theme.PhaseCancelled) + " " + phase)
	default:
		return theme.SuccessStyle().Render(theme.StatusSymbol(theme.PhaseIdle) + " " + phase)
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
	return fmt.Sprintf("↑ %d ↓ %d %s %d", tokens.Input, tokens.Output, theme.Separator(), tokens.Total)
}
