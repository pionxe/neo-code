package state

import (
	"reflect"
	"testing"

	"neo-code/internal/tuiv2/gateway"
)

// allEventTypes 穷举 gateway 包全部事件常量（issue #23 验收：白名单
// 穷举测试防"第 28 个事件静默漏过"——新增事件必须在此表加一行决定）。
func allEventTypes() []gateway.EventType {
	return []gateway.EventType{
		gateway.EventAgentChunk,
		gateway.EventAssistantDelta,
		gateway.EventAgentMessageStart,
		gateway.EventAgentMessageEnd,
		gateway.EventToolStart,
		gateway.EventToolStarted,
		gateway.EventToolEnd,
		gateway.EventToolFinished,
		gateway.EventToolOutput,
		gateway.EventPermissionRequested,
		gateway.EventPermissionResolved,
		gateway.EventAskUserQuestion,
		gateway.EventUserQuestionRequested,
		gateway.EventUserQuestionAnswered,
		gateway.EventPhaseChanged,
		gateway.EventRunStarted,
		gateway.EventRunFinished,
		gateway.EventRunError,
		gateway.EventError,
		gateway.EventRunCancelled,
		gateway.EventTokenUsage,
		gateway.EventSessionCreated,
		gateway.EventSessionDeleted,
		gateway.EventSessionUpdated,
		gateway.EventModelChanged,
		gateway.EventHealthChanged,
		gateway.EventGatewayOffline,
	}
}

// conversationForwarded 判定事件是否属于对话白名单（与 reduce_conversation.go
// 的决定表同步维护；穷举测试断言两者一致）。
func conversationForwarded(t gateway.EventType) bool {
	switch t {
	case gateway.EventAgentChunk, gateway.EventAssistantDelta,
		gateway.EventAgentMessageStart, gateway.EventAgentMessageEnd,
		gateway.EventToolStart, gateway.EventToolStarted,
		gateway.EventToolEnd, gateway.EventToolFinished, gateway.EventToolOutput,
		gateway.EventPermissionRequested, gateway.EventPermissionResolved,
		gateway.EventAskUserQuestion, gateway.EventUserQuestionRequested,
		gateway.EventUserQuestionAnswered, gateway.EventPhaseChanged,
		gateway.EventRunStarted, gateway.EventRunFinished,
		gateway.EventRunError, gateway.EventError,
		gateway.EventRunCancelled, gateway.EventTokenUsage:
		return true
	}
	return false
}

// TestReduceConversationExhaustive 穷举全部事件常量：
// 对话类必须真正迁移状态（Stream 或 Runtime 或 Input 有变化），
// 非对话类必须指针恒等且全槽快照零变化。
func TestReduceConversationExhaustive(t *testing.T) {
	for _, et := range allEventTypes() {
		t.Run(string(et), func(t *testing.T) {
			before := NewViewState()
			before.Gateway.Sessions = []gateway.SessionSummary{{ID: "s1"}}
			before.Gateway.Models = []gateway.ModelInfo{{ID: "m1"}}
			snapshot := *before

			after := ReduceConversation(before, event(et, map[string]any{"text": "x", "phase": "running", "id": "s2", "connected": true, "total_tokens": 9}))

			if after != before {
				t.Fatalf("pointer stability broken for %q", et)
			}
			forwarded := conversationForwarded(et)
			changed := !reflect.DeepEqual(snapshot, *before)
			if forwarded && !changed {
				t.Fatalf("event %q is whitelisted but mutated nothing", et)
			}
			if !forwarded && changed {
				t.Fatalf("event %q is not whitelisted but mutated state (slot discipline violation)", et)
			}
			// 非对话类：Gateway 槽（sessions/models/connected）必须零变化。
			if !forwarded && !reflect.DeepEqual(snapshot.Gateway, before.Gateway) {
				t.Fatalf("event %q touched Gateway slot", et)
			}
		})
	}
}

// TestReduceConversationInputTempWriteScope 锁定 6 类临时越权事件的变化范围：
// 仅 Input/Stream/Runtime 变化，其余槽（Gateway/Overlay/Search/Ex/Layout/Notify/Confirm/Mode）零变化。
func TestReduceConversationInputTempWriteScope(t *testing.T) {
	tempWrite := []gateway.EventType{
		gateway.EventPermissionRequested,
		gateway.EventPermissionResolved,
		gateway.EventAskUserQuestion,
		gateway.EventUserQuestionRequested,
		gateway.EventUserQuestionAnswered,
		gateway.EventRunCancelled,
	}
	for _, et := range tempWrite {
		t.Run(string(et), func(t *testing.T) {
			before := NewViewState()
			before.Search = SearchState{Active: true, Query: "q"}
			before.Ex = ExState{Active: true, Input: "w"}
			before.Notify = NotifyState{Text: "n"}
			snapshot := *before

			ReduceConversation(before, event(et, map[string]any{"text": "x", "prompt": "p", "question": "q"}))

			if !reflect.DeepEqual(snapshot.Input, before.Input) {
				// Input 变化属双轨期临时越权（已登记）。
			} else if et != gateway.EventPermissionResolved && et != gateway.EventUserQuestionAnswered && et != gateway.EventRunCancelled {
				t.Fatalf("%q should write Input slot (temp grant)", et)
			}
			// 其余槽零变化（除 Input/Stream/Runtime 的临时授权范围）。
			if !reflect.DeepEqual(snapshot.Search, before.Search) || !reflect.DeepEqual(snapshot.Ex, before.Ex) || !reflect.DeepEqual(snapshot.Notify, before.Notify) {
				t.Fatalf("%q touched Search/Ex/Notify slot", et)
			}
			if !reflect.DeepEqual(snapshot.Overlay, before.Overlay) || snapshot.Mode != before.Mode {
				t.Fatalf("%q touched Overlay/Mode slot", et)
			}
			if !reflect.DeepEqual(snapshot.Gateway, before.Gateway) {
				t.Fatalf("%q touched Gateway slot", et)
			}
		})
	}
}
