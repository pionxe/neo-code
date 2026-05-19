package runtime

import (
	"context"
	"testing"

	"neo-code/internal/runtime/acceptgate"
	"neo-code/internal/runtime/controlplane"
	runtimeverify "neo-code/internal/runtime/verify"
	agentsession "neo-code/internal/session"
)

func TestEmitVerificationLifecycleEvents(t *testing.T) {
	t.Parallel()

	service := &Service{events: make(chan RuntimeEvent, 16)}
	state := newRunState("run-verification-events", agentsession.New("verification-events"))
	report := acceptgate.Report{
		Outcome:    acceptgate.OutcomeFailed,
		StopReason: controlplane.StopReasonAcceptCheckFailed,
		Results: []acceptgate.CheckResult{
			{Name: "required_todo_failed", Passed: false, Reason: "required todo failed: t1"},
			{Name: "output_only", Passed: true},
		},
	}

	service.emitVerificationLifecycleEvents(context.Background(), &state, controlplane.CompletionState{
		CompletionBlockedReason: controlplane.CompletionBlockedReasonPendingTodo,
	}, report)

	events := collectRuntimeEvents(service.Events())
	assertEventSequence(t, events, []EventType{
		EventVerificationStarted,
		EventVerificationStageFinished,
		EventVerificationStageFinished,
		EventVerificationFinished,
	})

	started, ok := events[0].Payload.(VerificationStartedPayload)
	if !ok {
		t.Fatalf("started payload type = %T", events[0].Payload)
	}
	if started.CompletionPassed {
		t.Fatalf("CompletionPassed = true, want false")
	}
	if started.CompletionBlockedReason != string(controlplane.CompletionBlockedReasonPendingTodo) {
		t.Fatalf("CompletionBlockedReason = %q", started.CompletionBlockedReason)
	}

	stage, ok := events[1].Payload.(VerificationStageFinishedPayload)
	if !ok {
		t.Fatalf("stage payload type = %T", events[1].Payload)
	}
	if stage.Name != "required_todo_failed" || stage.Status != runtimeverify.VerificationFail {
		t.Fatalf("unexpected stage payload: %+v", stage)
	}
	if stage.ErrorClass == "" {
		t.Fatalf("expected failed stage to carry error class")
	}

	finished, ok := events[len(events)-1].Payload.(VerificationFinishedPayload)
	if !ok {
		t.Fatalf("finished payload type = %T", events[len(events)-1].Payload)
	}
	if finished.AcceptanceStatus != string(acceptgate.OutcomeFailed) {
		t.Fatalf("AcceptanceStatus = %q", finished.AcceptanceStatus)
	}
	if finished.StopReason != controlplane.StopReasonAcceptCheckFailed {
		t.Fatalf("StopReason = %q", finished.StopReason)
	}
	if finished.ErrorClass != runtimeverify.ErrorClassUnknown {
		t.Fatalf("ErrorClass = %q, want unknown", finished.ErrorClass)
	}
}
