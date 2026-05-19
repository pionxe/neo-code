package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	providertypes "neo-code/internal/provider/types"
	"neo-code/internal/runtime/controlplane"
	"neo-code/internal/security"
	agentsession "neo-code/internal/session"
	"neo-code/internal/tools"
)

func TestAskUserLifecycleTransitionsBackToExecute(t *testing.T) {
	t.Parallel()

	registry := tools.NewRegistry()
	registry.Register(&stubTool{
		name: tools.ToolNameAskUser,
		executeFn: func(_ context.Context, input tools.ToolCallInput) (tools.ToolResult, error) {
			input.AskUserEventEmitter("user_question_requested", map[string]any{
				"request_id":  "ask-lifecycle-1",
				"question_id": "q1",
				"title":       "Need choice",
				"kind":        "single_choice",
			})
			input.AskUserEventEmitter("user_question_answered", map[string]any{
				"request_id":  "ask-lifecycle-1",
				"question_id": "q1",
				"status":      "answered",
				"values":      []any{"yes"},
			})
			return tools.ToolResult{Name: tools.ToolNameAskUser, Content: "answered"}, nil
		},
	})
	manager, err := tools.NewManager(registry, newAllowPermissionEngine(t), nil)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	service := NewWithFactory(
		newRuntimeConfigManager(t),
		manager,
		newMemoryStore(),
		&scriptedProviderFactory{provider: &scriptedProvider{}},
		nil,
	)
	service.events = make(chan RuntimeEvent, 64)

	state := newRunState("run-ask-lifecycle", agentsession.New("ask-lifecycle"))
	state.planningEnabled = true
	state.session.AgentMode = agentsession.AgentModePlan
	if err := service.setBaseRunState(context.Background(), &state, controlplane.RunStatePlan); err != nil {
		t.Fatalf("set base state: %v", err)
	}
	if err := service.setBaseRunState(context.Background(), &state, controlplane.RunStateExecute); err != nil {
		t.Fatalf("set base state: %v", err)
	}

	_, execErr := service.executeToolCallWithPermission(context.Background(), permissionExecutionInput{
		RunID:       "run-ask-lifecycle",
		SessionID:   state.session.ID,
		State:       &state,
		ToolTimeout: time.Second,
		Call: providertypes.ToolCall{
			ID:        "call-ask-lifecycle",
			Name:      tools.ToolNameAskUser,
			Arguments: `{"question_id":"q1","title":"Need choice","kind":"single_choice"}`,
		},
	})
	if execErr != nil {
		t.Fatalf("executeToolCallWithPermission() error = %v", execErr)
	}
	if state.lifecycle != controlplane.RunStateExecute {
		t.Fatalf("lifecycle = %q, want execute", state.lifecycle)
	}
	if state.pendingUserQuestion != nil {
		t.Fatalf("pending user question should be cleared, got %#v", state.pendingUserQuestion)
	}

	events := collectRuntimeEvents(service.Events())
	if !hasPhaseTransition(events, "execute", "waiting_user_question") {
		t.Fatalf("missing execute -> waiting_user_question transition, events=%+v", events)
	}
	if !hasPhaseTransition(events, "waiting_user_question", "execute") {
		t.Fatalf("missing waiting_user_question -> execute transition, events=%+v", events)
	}
}

func TestAskUserLifecycleCleanupOnInterruptedQuestion(t *testing.T) {
	t.Parallel()

	registry := tools.NewRegistry()
	registry.Register(&stubTool{
		name: tools.ToolNameAskUser,
		executeFn: func(_ context.Context, input tools.ToolCallInput) (tools.ToolResult, error) {
			input.AskUserEventEmitter("user_question_requested", map[string]any{
				"request_id":  "ask-lifecycle-interrupted",
				"question_id": "q2",
				"title":       "Need text",
				"kind":        "text",
			})
			return tools.NewErrorResult(tools.ToolNameAskUser, "interrupted", "", nil), errors.New("ask interrupted")
		},
	})
	manager, err := tools.NewManager(registry, newAllowPermissionEngine(t), nil)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	service := NewWithFactory(
		newRuntimeConfigManager(t),
		manager,
		newMemoryStore(),
		&scriptedProviderFactory{provider: &scriptedProvider{}},
		nil,
	)
	service.events = make(chan RuntimeEvent, 64)

	state := newRunState("run-ask-cleanup", agentsession.New("ask-cleanup"))
	state.planningEnabled = true
	state.session.AgentMode = agentsession.AgentModePlan
	if err := service.setBaseRunState(context.Background(), &state, controlplane.RunStatePlan); err != nil {
		t.Fatalf("set base state: %v", err)
	}
	if err := service.setBaseRunState(context.Background(), &state, controlplane.RunStateExecute); err != nil {
		t.Fatalf("set base state: %v", err)
	}

	_, execErr := service.executeToolCallWithPermission(context.Background(), permissionExecutionInput{
		RunID:       "run-ask-cleanup",
		SessionID:   state.session.ID,
		State:       &state,
		ToolTimeout: time.Second,
		Call: providertypes.ToolCall{
			ID:        "call-ask-cleanup",
			Name:      tools.ToolNameAskUser,
			Arguments: `{"question_id":"q2","title":"Need text","kind":"text"}`,
		},
	})
	if execErr == nil {
		t.Fatalf("expected ask_user interrupted error")
	}
	if state.lifecycle != controlplane.RunStateExecute {
		t.Fatalf("lifecycle = %q, want execute", state.lifecycle)
	}
	if state.pendingUserQuestion != nil {
		t.Fatalf("pending user question should be cleared after cleanup, got %#v", state.pendingUserQuestion)
	}
}

func hasPhaseTransition(events []RuntimeEvent, from string, to string) bool {
	for _, event := range events {
		if event.Type != EventPhaseChanged {
			continue
		}
		payload, ok := event.Payload.(PhaseChangedPayload)
		if !ok {
			continue
		}
		if payload.From == from && payload.To == to {
			return true
		}
	}
	return false
}

func newAllowPermissionEngine(t *testing.T) security.PermissionEngine {
	t.Helper()
	engine, err := security.NewStaticGateway(security.DecisionAllow, nil)
	if err != nil {
		t.Fatalf("new static gateway: %v", err)
	}
	return engine
}
