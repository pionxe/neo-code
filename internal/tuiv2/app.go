// Package tuiv2 实现 Ghost Console TUI v2 的应用骨架。
package tuiv2

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"neo-code/internal/tuiv2/gateway"
)

const (
	modeInput       = "input"
	statusIdle      = "idle"
	surfaceName     = "ghost-console"
	defaultTerminal = "0x0"
)

// StartupConfig 承载 TUI v2 独立入口解析出的启动参数和 Gateway 客户端。
type StartupConfig struct {
	Backend  string
	Scenario string
	Debug    bool
	Client   gateway.Client
}

type appModel struct {
	cfg    StartupConfig
	width  int
	height int
	events int
}

var (
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7"))
	idleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a"))
	mutedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
	debugStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#e0af68"))
	promptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#bb9af7"))
)

// NewApp 创建最小 Bubble Tea Model，当前只渲染 Ghost Console 状态行、Prompt 和调试信息。
func NewApp(cfg StartupConfig) tea.Model {
	return appModel{cfg: cfg}
}

// Init 返回空命令，Phase 1 不主动拉取事件，所有数据后续都从 Gateway 客户端进入。
func (m appModel) Init() tea.Cmd {
	return nil
}

// Update 处理终端尺寸和退出按键，后续阶段会在这里接入 Gateway 事件消息路由。
func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			return m, tea.Quit
		}
	}
	return m, nil
}

// View 渲染无边框的 Ghost Console 占位界面，用状态符号、颜色和缩进表达层级。
func (m appModel) View() string {
	lines := []string{
		m.statusLine(),
		"",
		promptStyle.Render("› "),
	}
	if m.cfg.Debug {
		lines = append(lines, "", debugStyle.Render(m.debugLine()))
	}
	return strings.Join(lines, "\n")
}

// statusLine 渲染 Ghost Console 顶部状态，保持无边框并用状态符号表达运行态。
func (m appModel) statusLine() string {
	parts := []string{
		statusStyle.Render("NEOCODE"),
		idleStyle.Render("○ " + statusIdle),
		mutedStyle.Render(m.cfg.Backend),
		mutedStyle.Render(surfaceName),
	}
	return strings.Join(parts, "   ")
}

// debugLine 渲染调试模式下的最小运行信息，便于后续阶段观察事件与窗口尺寸。
func (m appModel) debugLine() string {
	size := defaultTerminal
	if m.width > 0 || m.height > 0 {
		size = fmt.Sprintf("%dx%d", m.width, m.height)
	}
	return fmt.Sprintf(
		"[debug] mode:%s  scenario:%s  events:%d  size:%s",
		modeInput,
		m.cfg.Scenario,
		m.events,
		size,
	)
}
