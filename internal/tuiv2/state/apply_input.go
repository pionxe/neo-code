package state

import "neo-code/internal/tuiv2/gateway"

// ApplyInputForEvent 将六类对话事件的 Input 槽写入部分应用到状态
// （issue #25 修订 v2 / 审计 P1-① 交汇点方案）：
//
//   - 它是 Input 写入的**单一出处**：旧路径 Reduce 内部调用本函数
//     （行为零变化），kernel 插件路径由 prompt 插件直接调用；
//   - 对非六类事件是 no-op（对账守卫并入 27 事件测试）；
//   - 调用方契约：仅在 Update 循环内、就地迁移（ADR-001）。
//
// 双轨期说明：旧路径 Reduce 在内核接线 PR 删除前仍会经本函数写 Input；
// 接线 PR 后 Input 写权归一于 prompt 插件。
func ApplyInputForEvent(s *ViewState, event gateway.GatewayEvent) {
	switch event.Type {
	case gateway.EventPermissionRequested:
		s.Input.Mode = InputStateModePermissionResponse
		s.Input.Prompt = payloadString(event.Payload, "prompt", "message", "tool")
	case gateway.EventPermissionResolved:
		s.Input.Mode = InputStateModeMessage
		s.Input.Prompt = ""
		s.Input.Options = nil
	case gateway.EventUserQuestionRequested:
		s.Input.Mode = InputStateModeQuestionAnswer
		s.Input.Prompt = payloadString(event.Payload, "question", "prompt", "message")
		s.Input.Options = payloadStringSlice(event.Payload, "options")
	case gateway.EventUserQuestionAnswered:
		s.Input.Mode = InputStateModeMessage
		s.Input.Text = ""
		s.Input.Cursor = 0
		s.Input.Prompt = ""
		s.Input.Options = nil
	case gateway.EventRunCanceled:
		s.Input.Mode = InputStateModeMessage
		s.Input.Prompt = ""
		s.Input.Options = nil
	}
}

// isInputWritingEvent 报告事件是否携带 Input 槽写入部分（对账守卫用）。
func isInputWritingEvent(t gateway.EventType) bool {
	switch t {
	case gateway.EventPermissionRequested,
		gateway.EventPermissionResolved,
		gateway.EventUserQuestionRequested,
		gateway.EventUserQuestionAnswered,
		gateway.EventRunCanceled:
		return true
	}
	return false
}
