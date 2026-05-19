package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"neo-code/internal/tuiv2/state"
)

// CommandPrompt 渲染命令和消息输入区域。
type CommandPrompt struct {
	state *state.ViewState
}

var _ tea.Model = (*CommandPrompt)(nil)

// NewCommandPrompt 创建命令输入组件。
func NewCommandPrompt(viewState *state.ViewState) *CommandPrompt {
	return &CommandPrompt{state: viewState}
}

// Init 不启动额外命令，组件只读取共享 ViewState。
func (c *CommandPrompt) Init() tea.Cmd {
	return nil
}

// Update 当前不维护组件私有业务状态，只保留 tea.Model 契约。
func (c *CommandPrompt) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return c, nil
}

// View 渲染底部输入区和模式提示。
func (c *CommandPrompt) View() string {
	lines := []string{statusMuted.Render("Command Prompt")}
	prompt := "› " + c.state.Input.Text
	if c.state.Input.Text == "" {
		prompt += "_"
	}
	lines = append(lines, statusBlue.Render(prompt))
	if c.state.Input.Prompt != "" {
		lines = append(lines, statusMuted.Render("  "+c.state.Input.Prompt))
	}
	lines = append(lines, c.modeLine())
	content := strings.Join(lines, "\n")
	if c.state.Layout.Width > 0 {
		return fitBlock(content, c.state.Layout.Width, true)
	}
	return content
}

// modeLine 渲染输入模式、界面名和当前模型。
func (c *CommandPrompt) modeLine() string {
	return statusMuted.Render(fmt.Sprintf(
		"[%s]   %s   %s",
		inputModeName(c.state.Mode),
		surfaceName,
		stringOrDash(c.state.Gateway.ActiveModel),
	))
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
