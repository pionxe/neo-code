package todo

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// Todo 模块相关常量与配置
const (
	// API 路径（由于目前是本地服务，这里作为逻辑标识）
	APIPathList   = "/api/todo/list"
	APIPathAdd    = "/api/todo/add"
	APIPathUpdate = "/api/todo/update"
	APIPathRemove = "/api/todo/remove"

	// UI 分页大小
	PageSize = 10

	// 状态文本
	StatusPending    = "Pending"
	StatusInProgress = "In Progress"
	StatusCompleted  = "Completed"

	// 优先级文本
	PriorityHigh   = "High"
	PriorityMedium = "Medium"
	PriorityLow    = "Low"

	// 状态图标
	IconPending    = "[ ]"
	IconInProgress = "[-]"
	IconCompleted  = "[x]"
	IconCursor     = "> "
	IconNoCursor   = "  "
)

// UI 颜色配置
var (
	ColorPending      = lipgloss.Color("#E5C07B") // 黄色
	ColorInProgress   = lipgloss.Color("#61AFEF") // 蓝色
	ColorCompleted    = lipgloss.Color("#98C379") // 绿色
	ColorPriorityHigh = lipgloss.Color("#E06C75") // 红色
	ColorSelection    = lipgloss.Color("#C678DD") // 紫色
	ColorDim          = lipgloss.Color("#5C6370") // 灰色
	ColorTitle        = lipgloss.Color("#61AFEF") // 蓝色
)

// 文本配置
const (
	TitleText      = "--- Todo List ---"
	EmptyText      = "There are no todos yet. Use /todo add <content> [priority] or press 'a' in todo mode to add one."
	HelpFooterText = "↑/↓: move  Enter: toggle status  x: delete  a: return and add  Esc: back to chat"

	// 交互提示消息
	MsgUsageAdd      = "Usage: /todo add <content> [priority(high/medium/low)]"
	MsgAddSuccess    = "Added todo: %s"
	MsgAddFailed     = "Failed to add todo: %v"
	MsgUnknownSubCmd = "Unknown todo subcommand: %s"
	MsgPromptAdd     = "Use /todo add <content> [priority] to add a new task."
)

// 按键绑定
type KeyMap struct {
	Up     key.Binding
	Down   key.Binding
	Add    key.Binding
	Done   key.Binding
	Delete key.Binding
	Back   key.Binding
}

var Keys = KeyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Add: key.NewBinding(
		key.WithKeys("a", "n"),
		key.WithHelp("a/n", "add"),
	),
	Done: key.NewBinding(
		key.WithKeys("enter", " "),
		key.WithHelp("enter/space", "toggle"),
	),
	Delete: key.NewBinding(
		key.WithKeys("x", "delete"),
		key.WithHelp("x/del", "delete"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc", "q"),
		key.WithHelp("esc/q", "back"),
	),
}
