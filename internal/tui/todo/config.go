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
	StatusPending    = "待办"
	StatusInProgress = "进行中"
	StatusCompleted  = "已完成"

	// 优先级文本
	PriorityHigh   = "高"
	PriorityMedium = "中"
	PriorityLow    = "低"

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
	TitleText      = "--- 待办清单 ---"
	EmptyText      = "当前没有任何待办事项。使用 /todo add <内容> [优先级] 或在待办模式下按 'a' 新增。"
	HelpFooterText = "↑/↓: 浏览  Enter: 切换状态  x: 删除  a: 返回并新增  Esc: 返回聊天"

	// 交互提示消息
	MsgUsageAdd      = "用法: /todo add <内容> [优先级(high/medium/low)]"
	MsgAddSuccess    = "已添加待办: %s"
	MsgAddFailed     = "添加待办失败: %v"
	MsgUnknownSubCmd = "未知待办子命令: %s"
	MsgPromptAdd     = "请使用 /todo add <内容> [优先级] 来添加新任务。"
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
		key.WithHelp("↑/k", "上移"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "下移"),
	),
	Add: key.NewBinding(
		key.WithKeys("a", "n"),
		key.WithHelp("a/n", "新增"),
	),
	Done: key.NewBinding(
		key.WithKeys("enter", " "),
		key.WithHelp("enter/space", "切换状态"),
	),
	Delete: key.NewBinding(
		key.WithKeys("x", "delete"),
		key.WithHelp("x/del", "删除"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc", "q"),
		key.WithHelp("esc/q", "返回"),
	),
}
