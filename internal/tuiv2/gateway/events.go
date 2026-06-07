package gateway

// EventType 标识 TUI v2 可消费的 Gateway 事件类型。
type EventType string

const (
	// EventAgentChunk 表示助手流式文本增量。
	EventAgentChunk EventType = "agent_chunk"
	// EventAgentMessageStart 表示助手消息开始。
	EventAgentMessageStart EventType = "agent_message_start"
	// EventAgentMessageEnd 表示助手消息结束。
	EventAgentMessageEnd EventType = "agent_message_end"
	// EventToolStart 表示工具调用开始。
	EventToolStart EventType = "tool_start"
	// EventToolEnd 表示工具调用结束。
	EventToolEnd EventType = "tool_end"
	// EventToolOutput 表示工具输出增量或片段。
	EventToolOutput EventType = "tool_output"
	// EventSessionUpdated 表示会话摘要或详情发生变化。
	EventSessionUpdated EventType = "session_updated"
	// EventSessionCreated 表示新会话已创建。
	EventSessionCreated EventType = "session_created"
	// EventSessionDeleted 表示会话已删除。
	EventSessionDeleted EventType = "session_deleted"
	// EventRunStarted 表示一次模型推理 run 已开始。
	EventRunStarted EventType = "run_started"
	// EventRunFinished 表示一次模型推理 run 已结束。
	EventRunFinished EventType = "run_finished"
	// EventRunError 表示一次模型推理 run 失败。
	EventRunError EventType = "run_error"
	// EventRunCancelled 表示一次模型推理 run 已取消。
	EventRunCancelled EventType = "run_cancelled"
	// EventTokenUsage 表示 token 用量更新。
	EventTokenUsage EventType = "token_usage"
	// EventPhaseChanged 表示运行阶段变化。
	EventPhaseChanged EventType = "phase_changed"
	// EventAssistantDelta 表示助手输出增量到达。
	EventAssistantDelta EventType = "assistant_delta"
	// EventToolStarted 表示工具调用开始。
	EventToolStarted EventType = "tool_started"
	// EventToolFinished 表示工具调用结束。
	EventToolFinished EventType = "tool_finished"
	// EventPermissionRequested 表示后端请求 UI 做工具权限决策。
	EventPermissionRequested EventType = "permission_requested"
	// EventPermissionResolved 表示工具权限请求已处理。
	EventPermissionResolved EventType = "permission_resolved"
	// EventAskUserQuestion 表示后端请求 UI 回答 ask_user 问题。
	EventAskUserQuestion EventType = "ask_user_question"
	// EventUserQuestionRequested 表示后端请求 UI 回答 ask_user 问题。
	EventUserQuestionRequested EventType = "user_question_requested"
	// EventUserQuestionAnswered 表示 ask_user 问题已由 UI 回答。
	EventUserQuestionAnswered EventType = "user_question_answered"
	// EventModelChanged 表示当前会话模型已切换。
	EventModelChanged EventType = "model_changed"
	// EventHealthChanged 表示 Gateway 健康状态发生变化。
	EventHealthChanged EventType = "health_changed"
	// EventGatewayOffline 表示 Gateway 连接不可用。
	EventGatewayOffline EventType = "gateway_offline"
	// EventError 表示 Gateway 客户端可展示的错误通知。
	EventError EventType = "error"
)
