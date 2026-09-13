package state

import "neo-code/internal/tuiv2/gateway"

// ApplyGatewayForEvent 将 Gateway 域事件的槽写入部分应用到状态
// （issue #27 S3-2 / 事件→槽写权对账表）：
//
//   - 单一出处：旧路径 Reduce 内部调用本函数（行为零变化），
//     kernel 插件路径由 sessions/models 插件直接调用；
//   - 对非 Gateway 域事件是 no-op（对账守卫并入 27 事件测试）；
//   - Gateway.Connected：S6 起由 health 插件直写（sessions 临时承接已移交，
//     issue #48）；本文件 health_changed 分支保留供 legacy Reduce 路径。
func ApplyGatewayForEvent(s *ViewState, event gateway.GatewayEvent) {
	switch event.Type {
	case gateway.EventSessionCreated:
		s.Gateway.Sessions = append(s.Gateway.Sessions, sessionFromPayload(event.Payload))
	case gateway.EventSessionDeleted:
		s.Gateway.Sessions = deleteSession(s.Gateway.Sessions, payloadString(event.Payload, "id", "session_id"))
	case gateway.EventSessionUpdated:
		s.Gateway.Sessions = upsertSession(s.Gateway.Sessions, sessionFromPayload(event.Payload))
	case gateway.EventModelChanged:
		s.Gateway.ActiveModel = payloadString(event.Payload, "model_id", "model", "id")
	case gateway.EventHealthChanged:
		s.Gateway.Connected = payloadBool(event.Payload, "connected", "ok")
	}
}

// IsGatewayEvent 报告事件是否属于 Gateway 域（对账守卫用）。
func IsGatewayEvent(t gateway.EventType) bool {
	switch t {
	case gateway.EventSessionCreated,
		gateway.EventSessionDeleted,
		gateway.EventSessionUpdated,
		gateway.EventModelChanged,
		gateway.EventHealthChanged:
		return true
	}
	return false
}
