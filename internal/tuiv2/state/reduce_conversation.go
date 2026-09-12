package state

import "neo-code/internal/tuiv2/gateway"

// conversationEvents 是对话类事件白名单（插件 chat 的槽纪律前提，issue #23）：
// 只有这些事件允许经 Reduce 写 Stream/Runtime 槽（部分还临时写 Input 槽，
// 见 viewstate.go Input 槽注释的移交标注）。session_*/model_changed/
// health_changed 归后续插件，gateway_offline 由 chat bespoke 处理
// （Gateway.Connected 归 health），均不在白名单。
var conversationEvents = map[gateway.EventType]struct{}{
	gateway.EventAgentChunk:            {},
	gateway.EventAssistantDelta:        {},
	gateway.EventAgentMessageStart:     {},
	gateway.EventAgentMessageEnd:       {},
	gateway.EventToolStart:             {},
	gateway.EventToolStarted:           {},
	gateway.EventToolEnd:               {},
	gateway.EventToolFinished:          {},
	gateway.EventToolOutput:            {},
	gateway.EventPermissionRequested:   {},
	gateway.EventPermissionResolved:    {},
	gateway.EventAskUserQuestion:       {},
	gateway.EventUserQuestionRequested: {},
	gateway.EventUserQuestionAnswered:  {},
	gateway.EventPhaseChanged:          {},
	gateway.EventRunStarted:            {},
	gateway.EventRunFinished:           {},
	gateway.EventRunError:              {},
	gateway.EventError:                 {},
	gateway.EventRunCancelled:          {},
	gateway.EventTokenUsage:            {},
}

// ReduceConversation 是对话类事件的槽纪律外壳：仅白名单事件转发 Reduce，
// 其余事件原样返回同一指针（不迁移任何槽）。指针全程稳定语义与 Reduce
// 一致（ADR-001）：返回值恒等于输入指针。
//
// 双轨期说明：permission_*、ask_user 系、run_cancelled 六类事件经 Reduce
// 会临时写 Input 槽（prompt 插件迁移前），移交标注见 viewstate.go。
func ReduceConversation(current *ViewState, event gateway.GatewayEvent) *ViewState {
	if _, ok := conversationEvents[event.Type]; ok {
		return Reduce(current, event)
	}
	return current
}
