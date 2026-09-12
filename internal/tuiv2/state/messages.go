package state

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
