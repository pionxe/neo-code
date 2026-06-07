// Package state 定义 TUI v2 的集中式 ViewModel 状态。
package state

import (
	"time"

	"neo-code/internal/tuiv2/gateway"
)

// ViewState 是 TUI v2 的单一界面状态源，所有子组件从这里读取渲染数据。
type ViewState struct {
	Gateway GatewayState
	Runtime RuntimeState
	Stream  []StreamEntry
	Input   InputState
	Layout  LayoutState
	Mode    InputMode
	Overlay OverlayState
}

// OverlayState 描述当前浮层显示状态。
type OverlayState struct {
	Active   string // "", "palette", "help", "session_picker", "confirm"
	Query    string // 搜索文本
	Selected int    // 当前选中索引
}

// GatewayState 描述 Gateway 连接、会话和模型选择状态。
type GatewayState struct {
	Connected   bool
	Sessions    []gateway.SessionSummary
	ActiveSess  *gateway.SessionSummary
	Models      []gateway.ModelInfo
	ActiveModel string
}

// RuntimeState 描述当前 run 的运行阶段、ID 和 token 用量。
type RuntimeState struct {
	Phase  string
	RunID  string
	Tokens TokenUsage
}

// TokenUsage 描述 ViewState 中展示所需的 token 用量。
type TokenUsage struct {
	Input  int
	Output int
	Total  int
}

// StreamEntry 描述 Agent 对话和状态流中的不可变条目。
type StreamEntry struct {
	ID        string
	Type      string
	Timestamp time.Time
	Content   string
	ToolName  string
	ToolInput string
	Metadata  map[string]any
}

// InputState 描述输入区文本、光标和当前输入任务。
type InputState struct {
	Text          string
	Cursor        int
	Mode          string
	Prompt        string
	Options       []string
	History       []string
	HistoryIndex  int
	CursorVisible bool
}

// LayoutState 描述终端布局尺寸和 Soft Inspector 显示状态。
type LayoutState struct {
	Width          int
	Height         int
	InspectorWidth int
	ShowInspector  bool
	ScrollOffset   int
	AutoScroll     bool
}

// InputMode 表示 TUI v2 顶层输入模式。
type InputMode int

const (
	// InputModeInput 是默认输入模式，用于打字和发送消息。
	InputModeInput InputMode = iota
	// NormalMode 是导航与搜索模式。
	NormalMode
	// LeaderMode 是命令模式。
	LeaderMode
)

const (
	// RuntimePhaseIdle 表示当前没有运行中的 run。
	RuntimePhaseIdle = "idle"
	// RuntimePhaseRunning 表示当前 run 正在执行。
	RuntimePhaseRunning = "running"
	// RuntimePhaseWaitingPermission 表示当前 run 正在等待工具权限决策。
	RuntimePhaseWaitingPermission = "waiting_permission"
	// RuntimePhaseWaitingUser 表示当前 run 正在等待用户回答。
	RuntimePhaseWaitingUser = "waiting_user"
	// RuntimePhaseCancelled 表示当前 run 已取消。
	RuntimePhaseCancelled = "cancelled"
	// RuntimePhaseError 表示当前 run 或 Gateway 处于错误态。
	RuntimePhaseError = "error"
)

const (
	// InputStateModeMessage 表示输入区当前编辑普通消息。
	InputStateModeMessage = "message"
	// InputStateModeCommand 表示输入区当前编辑命令。
	InputStateModeCommand = "command"
	// InputStateModePermissionResponse 表示输入区当前用于回答权限请求。
	InputStateModePermissionResponse = "permission_response"
	// InputStateModeQuestionAnswer 表示输入区当前用于回答 ask_user 问题。
	InputStateModeQuestionAnswer = "question_answer"
)

// NewViewState 创建 TUI v2 初始 ViewState。
func NewViewState() *ViewState {
	return &ViewState{
		Runtime: RuntimeState{Phase: RuntimePhaseIdle},
		Input:   InputState{Mode: InputStateModeMessage, HistoryIndex: -1, CursorVisible: true},
		Layout:  LayoutState{AutoScroll: true},
		Mode:    InputModeInput,
	}
}
