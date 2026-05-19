package gateway

// EventType 标识 TUI v2 可消费的 Gateway 事件类型。
type EventType string

const (
	// EventSessionUpdated 表示会话摘要或详情发生变化。
	EventSessionUpdated EventType = "session_updated"
	// EventRunStarted 表示一次模型推理 run 已开始。
	EventRunStarted EventType = "run_started"
	// EventRunFinished 表示一次模型推理 run 已结束。
	EventRunFinished EventType = "run_finished"
	// EventRunCancelled 表示一次模型推理 run 已取消。
	EventRunCancelled EventType = "run_cancelled"
	// EventAssistantDelta 表示助手输出增量到达。
	EventAssistantDelta EventType = "assistant_delta"
	// EventToolStarted 表示工具调用开始。
	EventToolStarted EventType = "tool_started"
	// EventToolFinished 表示工具调用结束。
	EventToolFinished EventType = "tool_finished"
	// EventPermissionRequested 表示后端请求 UI 做工具权限决策。
	EventPermissionRequested EventType = "permission_requested"
	// EventUserQuestionRequested 表示后端请求 UI 回答 ask_user 问题。
	EventUserQuestionRequested EventType = "user_question_requested"
	// EventModelChanged 表示当前会话模型已切换。
	EventModelChanged EventType = "model_changed"
	// EventGatewayOffline 表示 Gateway 连接不可用。
	EventGatewayOffline EventType = "gateway_offline"
	// EventError 表示 Gateway 客户端可展示的错误通知。
	EventError EventType = "error"
)
