package state

import "neo-code/internal/tuiv2/gateway"

// 本文件定义内核服务的跨插件消息类型（规范 §6/ADR-012）：
// 类型收敛在 state 包，插件之间互不 import 也能共享同一套消息词汇。

// ConfirmRequest 是一次危险操作确认请求（ADR-012：确认框为内核服务）。
// 插件经 Host.Confirm 发起，内核弹出确认浮层；用户应答后内核广播
// ConfirmResult，发起方在自己的 React 中按 ID 消费——禁止回调闭包，
// 以免制造内核看不见的隐式控制流。
type ConfirmRequest struct {
	ID      string         // 请求 ID：由内核生成，结果回传时原样带回
	Title   string         // 弹窗标题（如 "⚠ 删除会话"）
	Message string         // 说明文本：后果与建议操作
	Action  string         // 语义动作名（如 "delete_session"），供发起方路由
	Data    map[string]any // 附带数据（如目标会话 ID），回传时原样带回
}

// ConfirmResult 是确认框的用户应答，由内核广播、发起方按 ID 消费。
type ConfirmResult struct {
	ID     string
	Action string
	Data   map[string]any
	Yes    bool // true = 确认，false = 取消
}

// UserSubmitted 表示用户提交了一条消息（prompt 插件在 Input 模式 Enter 时
// 广播）：chat 插件订阅它更新 /retry 的 lastText 并追加 role=user 的
// Stream 条目（旧路径 app.go:429 行为等价）。
type UserSubmitted struct {
	Text string
}

// SessionLoaded 表示一次会话切换完成（sessions 插件经 GoCmd 执行
// LoadSession + 重订阅后广播）：chat 插件订阅它重载 Stream 槽。
type SessionLoaded struct {
	Session gateway.SessionSummary
	Detail  *gateway.SessionDetail
}

// SessionDeleted 表示一次会话删除（本地合成事件：契约无 deleteSession RPC，
// 复用 reducer 既有 EventSessionDeleted 分支迁移 Gateway.Sessions——
// issue #27 S3-2 / 审计 P2-⑩；"重启复活"差距已文档化）。
type SessionDeleted struct {
	ID string
}
