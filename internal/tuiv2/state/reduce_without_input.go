package state

import "neo-code/internal/tuiv2/gateway"

// 对话类事件白名单（issue #23/#25）：只有这些事件允许经 Reduce 写
// Stream/Runtime 槽。session_*/model_changed 归对应插件、health_changed
// 归 health 插件（S6 移交，issue #48）；
// gateway_offline 由 chat bespoke 处理（Phase/流条目；Connected 归 health）。
var conversationEvents = map[gateway.EventType]struct{}{
	gateway.EventAgentChunk:            {},
	gateway.EventAgentMessageStart:     {},
	gateway.EventAgentMessageEnd:       {},
	gateway.EventToolStart:             {},
	gateway.EventToolResult:            {},
	gateway.EventToolOutput:            {},
	gateway.EventPermissionRequested:   {},
	gateway.EventPermissionResolved:    {},
	gateway.EventUserQuestionRequested: {},
	gateway.EventUserQuestionAnswered:  {},
	gateway.EventPhaseChanged:          {},
	gateway.EventRunStarted:            {},
	gateway.EventRunFinished:           {},
	gateway.EventRunError:              {},
	gateway.EventError:                 {},
	gateway.EventRunCanceled:           {},
	gateway.EventTokenUsage:            {},
}

// ReduceWithoutInput 是对话类事件的槽纪律外壳（chat 插件路径）：
// 白名单事件转发 Reduce 但**跳过 Input 槽写入**——Input 写权归 prompt
// 插件（经 ApplyInputForEvent，由 prompt React 调用）；其余事件原样返回
// 同一指针。指针全程稳定语义与 Reduce 一致（ADR-001）。
//
// 与 Reduce 的逐槽一致性（除 Input 外）由 TestReduceWithoutInputMatchesReduce
// 表驱动钉死（审计 P2-a）。
func ReduceWithoutInput(current *ViewState, event gateway.GatewayEvent) *ViewState {
	if _, ok := conversationEvents[event.Type]; !ok {
		return current
	}
	return reduceCore(current, event, false)
}
