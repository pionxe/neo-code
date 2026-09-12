package state

import (
	"reflect"
	"testing"
	"time"

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

// TestReduceWithoutInputExhaustive 穷举全部事件常量：
// 对话类必须真正迁移状态（Stream 或 Runtime 或 Input 有变化），
// 非对话类必须指针恒等且全槽快照零变化。
func TestReduceWithoutInputExhaustive(t *testing.T) {
	for _, et := range allEventTypes() {
		t.Run(string(et), func(t *testing.T) {
			before := NewViewState()
			before.Gateway.Sessions = []gateway.SessionSummary{{ID: "s1"}}
			before.Gateway.Models = []gateway.ModelInfo{{ID: "m1"}}
			snapshot := *before

			after := ReduceWithoutInput(before, event(et, map[string]any{"text": "x", "phase": "running", "id": "s2", "connected": true, "total_tokens": 9}))

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

// TestReduceWithoutInputScope 锁定六类含 Input 写入事件的变化范围
// （该范围现在由 ApplyInputForEvent 承接，chat 路径跳过——见 96 行下函数名对应）：
// 仅 Input/Stream/Runtime 变化，其余槽（Gateway/Overlay/Search/Ex/Layout/Notify/Confirm/Mode）零变化。
func TestReduceWithoutInputScope(t *testing.T) {
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
			before.Layout = LayoutState{Width: 100, ScrollOffset: 3, AutoScroll: false}
			before.Confirm = ConfirmState{Title: "t"}
			snapshot := *before

			ReduceWithoutInput(before, event(et, map[string]any{"text": "x", "prompt": "p", "question": "q", "decision": "allow", "answer": "a", "phase": "cancelled"}))

			// 允许变化：Input（prompt 经 ApplyInputForEvent）、Stream（状态条目）、Runtime（Phase）。
			// Confirm 槽：本组事件不涉及确认框，必须零变化（补快照，审计 P2）。
			if !reflect.DeepEqual(snapshot.Confirm, before.Confirm) {
				t.Fatalf("%q touched Confirm slot", et)
			}
			// 禁止变化：Search/Ex/Notify/Overlay/Layout/Mode/Gateway。
			if !reflect.DeepEqual(snapshot.Search, before.Search) {
				t.Fatalf("%q touched Search slot", et)
			}
			if !reflect.DeepEqual(snapshot.Ex, before.Ex) {
				t.Fatalf("%q touched Ex slot", et)
			}
			if !reflect.DeepEqual(snapshot.Notify, before.Notify) {
				t.Fatalf("%q touched Notify slot", et)
			}
			if !reflect.DeepEqual(snapshot.Overlay, before.Overlay) {
				t.Fatalf("%q touched Overlay slot", et)
			}
			if !reflect.DeepEqual(snapshot.Layout, before.Layout) {
				t.Fatalf("%q touched Layout slot", et)
			}
			if snapshot.Mode != before.Mode {
				t.Fatalf("%q touched Mode slot", et)
			}
			if !reflect.DeepEqual(snapshot.Gateway, before.Gateway) {
				t.Fatalf("%q touched Gateway slot", et)
			}
		})
	}
}

// TestReduceConversationDecisionAccounting 是白名单的机械守卫（审计 P2）：
// 转发数 + 显式排除数 == 事件常量总数。gateway 新增第 28 个事件时
// allEventTypes 变长而两表未更新，本测试即失败——强制补决定。
func TestReduceWithoutInputDecisionAccounting(t *testing.T) {
	excluded := map[gateway.EventType]bool{
		gateway.EventSessionCreated: true,
		gateway.EventSessionDeleted: true,
		gateway.EventSessionUpdated: true,
		gateway.EventModelChanged:   true,
		gateway.EventHealthChanged:  true,
		gateway.EventGatewayOffline: true, // chat bespoke 分支（见插件 React）
	}
	forwarded, excludedCount := 0, 0
	for _, et := range allEventTypes() {
		if conversationForwarded(et) {
			forwarded++
			if excluded[et] {
				t.Fatalf("%q 同时出现在转发与排除集合", et)
			}
			continue
		}
		excludedCount++
		if !excluded[et] {
			t.Fatalf("%q 未登记决定（转发或排除二选一）", et)
		}
	}
	if forwarded != 21 {
		t.Fatalf("forwarded = %d, want 21", forwarded)
	}
	if excludedCount != len(excluded) {
		t.Fatalf("excluded = %d, want %d（gateway 新增事件需同步三张表）", excludedCount, len(excluded))
	}
}

// TestApplyInputForEventNoopForNonInputEvents 锁定 ApplyInputForEvent 的
// no-op 契约：非六类事件不写 Input（对账守卫并入，审计 P2-b）。
func TestApplyInputForEventNoopForNonInputEvents(t *testing.T) {
	for _, et := range allEventTypes() {
		if isInputWritingEvent(et) {
			continue
		}
		before := NewViewState()
		before.Input.Text = "keep"
		ApplyInputForEvent(before, event(et, map[string]any{"prompt": "p", "question": "q", "options": []any{"o"}}))
		if before.Input.Text != "keep" || before.Input.Mode != InputStateModeMessage {
			t.Fatalf("%q touched Input via ApplyInputForEvent", et)
		}
	}
}

// TestReduceWithoutInputMatchesReduce 是审计 P2-a 的一致性断言：
// 对六类含 Input 写入的事件，Reduce 与 ReduceWithoutInput(+ApplyInputForEvent)
// 在 Input 之外的槽（Stream/Runtime/…）必须逐槽一致；Input 部分由
// ApplyInputForEvent 补齐后整体等价于 Reduce。
func TestReduceWithoutInputMatchesReduce(t *testing.T) {
	for _, et := range allEventTypes() {
		if !isInputWritingEvent(et) {
			continue
		}
		t.Run(string(et), func(t *testing.T) {
			payload := map[string]any{
				"text": "x", "prompt": "P", "question": "Q", "decision": "allow",
				"answer": "a", "phase": "cancelled", "message": "m",
				"options": []any{"o1", "o2"}, "total": 5,
			}
			// 两条路径使用同一固定 At（eventTime 零值回退 Now，两次调用
			// 时间戳微秒差会污染 DeepEqual）。
			fixedAt := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
			evA := event(et, payload)
			evA.At = fixedAt
			evB := event(et, payload)
			evB.At = fixedAt
			// 路径 A：全量 Reduce。
			a := NewViewState()
			a = Reduce(a, evA)
			// 路径 B：ReduceWithoutInput + prompt 侧 ApplyInputForEvent。
			b := NewViewState()
			b = ReduceWithoutInput(b, evB)
			// b.Input 未变（Input 原值不变断言——chat 路径语义）。
			wantInput := NewViewState().Input
			if !reflect.DeepEqual(b.Input, wantInput) {
				t.Fatalf("chat path Input mutated: %+v", b.Input)
			}
			// prompt 侧补 Input。
			ApplyInputForEvent(b, evB)
			// 逐槽一致性：Input 之外全部相等。
			if !reflect.DeepEqual(a.Stream, b.Stream) {
				t.Fatalf("Stream mismatch:\nA=%+v\nB=%+v", a.Stream, b.Stream)
			}
			if a.Runtime != b.Runtime {
				t.Fatalf("Runtime mismatch: A=%+v B=%+v", a.Runtime, b.Runtime)
			}
			if !reflect.DeepEqual(a.Input, b.Input) {
				t.Fatalf("Input mismatch after prompt apply:\nA=%+v\nB=%+v", a.Input, b.Input)
			}
			// P2-a 专项：permission/question 条目 content 从 payload 直取
			//（chat 路径无 Input.Prompt 中间态，content 仍完整）。
			if et == gateway.EventPermissionRequested || et == gateway.EventAskUserQuestion {
				last := b.Stream[len(b.Stream)-1].Content
				if last == "" {
					t.Fatalf("%q content lost after split (P2-a read dependency)", et)
				}
			}
		})
	}
}
