package gateway

// EventType 标识 TUI v2 可消费的 Gateway 事件类型。
type EventType string

// 事件词汇收敛说明（S4 契约收敛，issue #44 / ADR-002）：
// 本表此前存在 4 对重复别名（tool_end/tool_finished、agent_chunk/
// assistant_delta、ask_user_question/user_question_requested、
// tool_start/tool_started）与拼写漂移（run_cancelled），已按
// runtime 权威词汇（internal/runtime/events.go）收敛为单一规范名。
// 常量按词汇权威源分三类（分类学见各分组注释）。

// A 组：runtime envelope 同名事件——名字与 runtime 事件词汇
// （internal/runtime/events.go）一一对应，S5 RealClient 翻译层
// 可机械映射（envelope 字段 → 同名常量）。
const (
	// EventAgentChunk 对应 runtime agent_chunk：助手流式文本增量。
	EventAgentChunk EventType = "agent_chunk"
	// EventToolStart 对应 runtime tool_start：工具开始执行。
	EventToolStart EventType = "tool_start"
	// EventToolResult 对应 runtime tool_result：工具执行完成并写回会话
	//（收敛自旧别名对 tool_end / tool_finished——两者均非 runtime 词汇，
	// 删除；UI 渲染词汇 StreamEntry.Type 的 "tool_end" Kind 不受影响，
	// 那是 state 归一化层的独立词汇）。
	EventToolResult EventType = "tool_result"
	// EventToolOutput 表示工具输出增量或片段。
	// 注意：runtime 对应词汇是 tool_chunk（命名不一致为历史遗留），
	// S5 翻译层将 runtime tool_chunk 映射到本常量（或届时改名对齐）。
	EventToolOutput EventType = "tool_output"
	// EventRunCanceled 对应 runtime run_canceled：运行被取消
	//（拼写收敛自旧 run_cancelled）。
	EventRunCanceled EventType = "run_canceled"
	// EventTokenUsage 对应 runtime token_usage：token 用量更新。
	EventTokenUsage EventType = "token_usage"
	// EventPhaseChanged 对应 runtime phase_changed：运行阶段迁移。
	EventPhaseChanged EventType = "phase_changed"
	// EventPermissionRequested 对应 runtime permission_requested：后端
	// 请求 UI 做工具权限决策。
	EventPermissionRequested EventType = "permission_requested"
	// EventPermissionResolved 对应 runtime permission_resolved：权限请求已处理。
	EventPermissionResolved EventType = "permission_resolved"
	// EventUserQuestionRequested 对应 runtime user_question_requested：
	// 后端请求 UI 回答 ask_user 问题（收敛自旧别名 ask_user_question）。
	EventUserQuestionRequested EventType = "user_question_requested"
	// EventUserQuestionAnswered 对应 runtime user_question_answered：
	// ask_user 问题已由 UI 回答。
	EventUserQuestionAnswered EventType = "user_question_answered"
	// EventError 对应 runtime error：错误通知。
	// 双角色（现状如实标注）：runtime envelope 会推送 error 事件；
	// 客户端亦会自 RPC 失败本地合成同型事件（prompt/sessions/models，
	// payload 键 message/error/text 宽容兜底——S5 须保持合成 payload
	// 最小键契约一致）。
	EventError EventType = "error"
)

// B 组：gateway 层自有词汇——来源是 gateway 连接/通知层状态
// （非 runtime envelope、非客户端合成），S5 由连接状态直接映射。
const (
	// EventHealthChanged 表示 Gateway 健康状态发生变化。
	EventHealthChanged EventType = "health_changed"
	// EventGatewayOffline 表示 Gateway 连接不可用。
	EventGatewayOffline EventType = "gateway_offline"
)

// C 组：客户端侧派生/合成事件——名字不在 runtime envelope 词汇内，
// S5 翻译层从 RPC 结果或 envelope 边界合成（派生来源见各常量注释）。
const (
	// EventSessionUpdated 表示会话摘要或详情发生变化。
	// 派生来源：listSessions/loadSession RPC 结果或 gateway 通知。
	EventSessionUpdated EventType = "session_updated"
	// EventSessionCreated 表示新会话已创建。
	// 派生来源：createSession RPC 结果。
	EventSessionCreated EventType = "session_created"
	// EventSessionDeleted 表示会话已删除。
	// 派生来源：本地合成（真实网关有 gateway.deleteSession RPC，
	// tuiv2 Client 接入后改为翻译层派生）。
	EventSessionDeleted EventType = "session_deleted"
	// EventRunStarted 表示一次模型推理 run 已开始。
	// 派生来源：gateway.run RPC ack。
	EventRunStarted EventType = "run_started"
	// EventRunFinished 表示一次模型推理 run 已结束。
	// 派生来源：gateway.event 通知外层 run_done / envelope agent_done 边界。
	EventRunFinished EventType = "run_finished"
	// EventRunError 表示一次模型推理 run 失败。
	// 派生来源：gateway.event 通知外层 run_error；与 A 组 EventError
	// 同分支消费（reducer），勿再造第二对别名。
	EventRunError EventType = "run_error"
	// EventAgentMessageStart 表示助手消息开始。
	// 派生来源：S5 由 envelope agent_done + 消息边界合成（UI 消息边界
	// 事件，非 envelope 1:1 映射——runtime 侧只有结束语义 agent_done）。
	EventAgentMessageStart EventType = "agent_message_start"
	// EventAgentMessageEnd 表示助手消息结束。
	// 派生来源：同 EventAgentMessageStart。
	EventAgentMessageEnd EventType = "agent_message_end"
	// EventModelChanged 表示当前会话模型已切换。
	// 派生来源：setSessionModel RPC ack 或 runtime snapshot 通知。
	EventModelChanged EventType = "model_changed"
)
