package runtime

import (
	"context"
	"testing"

	"neo-code/internal/runtime/controlplane"
)

func TestTemporaryRunStateCountersKeepEffectiveStateStable(t *testing.T) {
	t.Parallel()

	service := &Service{events: make(chan RuntimeEvent, 16)}
	state := newRunState("run-temp-counter", newRuntimeSession("session-temp-counter"))
	if err := service.setBaseRunState(context.Background(), &state, controlplane.RunStatePlan); err != nil {
		t.Fatalf("set base run state: %v", err)
	}
	if err := service.setBaseRunState(context.Background(), &state, controlplane.RunStateExecute); err != nil {
		t.Fatalf("set base run state: %v", err)
	}

	if err := service.enterTemporaryRunState(context.Background(), &state, controlplane.RunStateWaitingPermission); err != nil {
		t.Fatalf("enter waiting #1: %v", err)
	}
	if err := service.enterTemporaryRunState(context.Background(), &state, controlplane.RunStateWaitingPermission); err != nil {
		t.Fatalf("enter waiting #2: %v", err)
	}
	if state.lifecycle != controlplane.RunStateWaitingPermission {
		t.Fatalf("lifecycle = %q, want waiting_permission", state.lifecycle)
	}

	if err := service.leaveTemporaryRunState(context.Background(), &state, controlplane.RunStateWaitingPermission); err != nil {
		t.Fatalf("leave waiting #1: %v", err)
	}
	if state.lifecycle != controlplane.RunStateWaitingPermission {
		t.Fatalf("lifecycle after first leave = %q, want waiting_permission", state.lifecycle)
	}

	if err := service.leaveTemporaryRunState(context.Background(), &state, controlplane.RunStateWaitingPermission); err != nil {
		t.Fatalf("leave waiting #2: %v", err)
	}
	if state.lifecycle != controlplane.RunStateExecute {
		t.Fatalf("lifecycle after second leave = %q, want execute", state.lifecycle)
	}

	events := collectRuntimeEvents(service.Events())
	assertPhaseTransitions(t, events, [][2]string{
		{"", "plan"},
		{"plan", "execute"},
		{"execute", "waiting_permission"},
		{"waiting_permission", "execute"},
	})
}

func TestTemporaryRunStatePriorityWaitingOverCompacting(t *testing.T) {
	t.Parallel()

	service := &Service{events: make(chan RuntimeEvent, 16)}
	state := newRunState("run-temp-priority", newRuntimeSession("session-temp-priority"))
	if err := service.setBaseRunState(context.Background(), &state, controlplane.RunStatePlan); err != nil {
		t.Fatalf("set base run state: %v", err)
	}

	if err := service.enterTemporaryRunState(context.Background(), &state, controlplane.RunStateCompacting); err != nil {
		t.Fatalf("enter compacting: %v", err)
	}
	if state.lifecycle != controlplane.RunStateCompacting {
		t.Fatalf("lifecycle = %q, want compacting", state.lifecycle)
	}
	if err := service.enterTemporaryRunState(context.Background(), &state, controlplane.RunStateWaitingPermission); err != nil {
		t.Fatalf("enter waiting: %v", err)
	}
	if state.lifecycle != controlplane.RunStateWaitingPermission {
		t.Fatalf("lifecycle = %q, want waiting_permission", state.lifecycle)
	}

	if err := service.leaveTemporaryRunState(context.Background(), &state, controlplane.RunStateWaitingPermission); err != nil {
		t.Fatalf("leave waiting: %v", err)
	}
	if state.lifecycle != controlplane.RunStateCompacting {
		t.Fatalf("lifecycle = %q, want compacting after waiting leaves", state.lifecycle)
	}
	if err := service.leaveTemporaryRunState(context.Background(), &state, controlplane.RunStateCompacting); err != nil {
		t.Fatalf("leave compacting: %v", err)
	}
	if state.lifecycle != controlplane.RunStatePlan {
		t.Fatalf("lifecycle = %q, want plan", state.lifecycle)
	}
}

func assertPhaseTransitions(t *testing.T, events []RuntimeEvent, expected [][2]string) {
	t.Helper()

	var phases [][2]string
	for _, event := range events {
		if event.Type != EventPhaseChanged {
			continue
		}
		payload, ok := event.Payload.(PhaseChangedPayload)
		if !ok {
			t.Fatalf("expected phase payload, got %#v", event.Payload)
		}
		phases = append(phases, [2]string{payload.From, payload.To})
	}
	if len(phases) != len(expected) {
		t.Fatalf("phase transition count = %d, want %d, got %+v", len(phases), len(expected), phases)
	}
	for i := range expected {
		if phases[i] != expected[i] {
			t.Fatalf("phase transition[%d] = %+v, want %+v", i, phases[i], expected[i])
		}
	}
}
