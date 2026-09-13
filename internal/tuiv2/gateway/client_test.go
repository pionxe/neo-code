package gateway

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestEventTypeValues(t *testing.T) {
	tests := map[EventType]string{
		EventAgentChunk:            "agent_chunk",
		EventAgentMessageStart:     "agent_message_start",
		EventAgentMessageEnd:       "agent_message_end",
		EventToolStart:             "tool_start",
		EventToolResult:            "tool_result",
		EventToolOutput:            "tool_output",
		EventSessionUpdated:        "session_updated",
		EventSessionCreated:        "session_created",
		EventSessionDeleted:        "session_deleted",
		EventRunStarted:            "run_started",
		EventRunFinished:           "run_finished",
		EventRunError:              "run_error",
		EventRunCanceled:           "run_canceled",
		EventTokenUsage:            "token_usage",
		EventPhaseChanged:          "phase_changed",
		EventPermissionRequested:   "permission_requested",
		EventPermissionResolved:    "permission_resolved",
		EventUserQuestionRequested: "user_question_requested",
		EventModelChanged:          "model_changed",
		EventHealthChanged:         "health_changed",
		EventGatewayOffline:        "gateway_offline",
		EventError:                 "error",
	}

	for got, want := range tests {
		if string(got) != want {
			t.Fatalf("event value = %q, want %q", got, want)
		}
	}
}

func TestRealClientSatisfiesClient(t *testing.T) {
	// S4 时代的占位构造已由 S5 的 mock 注入构造取代（见 real_test.go），
	// 此处保留接口满足性断言（契约面）。
	var _ Client = (*RealClient)(nil)
}

// TestClientShapeFreeze 是 Client 接口形状冻结测试（S4 完备性，issue #44）：
// 反射枚举方法全集并与"方法→真实 RPC"映射表比对键集合一致——接口增删
// 方法必须同步更新映射表（权威依据：docs/reference/tui-gateway-contract-matrix.md
// 与 Client 注释映射总表）。映射值用 RPC 方法名字符串而非
// internal/gateway/protocol 常量：tuiv2 禁 import 后端模块（边界 lint 三禁）。
func TestClientShapeFreeze(t *testing.T) {
	want := map[string]string{
		"Health":             "gateway.ping",
		"ListSessions":       "gateway.listSessions",
		"LoadSession":        "gateway.loadSession",
		"CreateSession":      "gateway.createSession",
		"SendMessage":        "gateway.run",
		"CancelRun":          "gateway.cancel",
		"SubscribeEvents":    "gateway.bindStream + gateway.event",
		"ResolvePermission":  "gateway.resolvePermission",
		"AnswerUserQuestion": "gateway.userQuestionAnswer",
		"ListModels":         "gateway.listModels",
		"SetModel":           "gateway.setSessionModel",
		"GetModel":           "gateway.getSessionModel",
		"Close":              "", // 客户端本地资源释放，无 RPC
	}
	typ := reflect.TypeOf((*Client)(nil)).Elem()
	got := make(map[string]bool, typ.NumMethod())
	for i := 0; i < typ.NumMethod(); i++ {
		name := typ.Method(i).Name
		got[name] = true
		if _, ok := want[name]; !ok {
			t.Fatalf("接口出现未登记方法 %q——请同步 Client 映射注释与本测试", name)
		}
	}
	for name := range want {
		if !got[name] {
			t.Fatalf("映射表中的方法 %q 已从接口消失——请同步更新", name)
		}
	}
}

// TestErrUnsupportedSentinel 烟测哨兵契约（S4）：可被 errors.Is 识别
// （实现方必须 %w 包装返回，UI 层据此显式降级——ADR-002）。
func TestErrUnsupportedSentinel(t *testing.T) {
	wrapped := fmt.Errorf("listModels: %w", ErrUnsupported)
	if !errors.Is(wrapped, ErrUnsupported) {
		t.Fatal("wrapped ErrUnsupported must be identifiable via errors.Is")
	}
	if ErrUnsupported.Error() != "gateway: unsupported capability" {
		t.Fatalf("哨兵文案是契约面，不应随手改动: %q", ErrUnsupported.Error())
	}
}
