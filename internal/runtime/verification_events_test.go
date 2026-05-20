package runtime

import (
	"context"
	"testing"

	"neo-code/internal/runtime/acceptgate"
	"neo-code/internal/runtime/controlplane"
	agentsession "neo-code/internal/session"
)

func TestEmitVerificationLifecycleEvents(t *testing.T) {
	t.Parallel()

	service := &Service{events: make(chan RuntimeEvent, 16)}
	state := newRunState("run-verification-events", agentsession.New("verification-events"))
	report := acceptgate.Report{
		Outcome:    acceptgate.OutcomeFailed,
		StopReason: controlplane.StopReasonVerificationFailed,
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
	if stage.Name != "required_todo_failed" || stage.Status != "fail" {
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
	if finished.StopReason != controlplane.StopReasonVerificationFailed {
		t.Fatalf("StopReason = %q", finished.StopReason)
	}
	if finished.ErrorClass != "unknown" {
		t.Fatalf("ErrorClass = %q, want unknown", finished.ErrorClass)
	}
}

func TestEmitVerificationLifecycleEventsAcceptedAndNilGuards(t *testing.T) {
	t.Parallel()

	service := &Service{events: make(chan RuntimeEvent, 16)}
	state := newRunState("run-verification-accepted", agentsession.New("verification-accepted"))
	service.emitVerificationLifecycleEvents(context.Background(), &state, controlplane.CompletionState{}, acceptgate.Report{
		Outcome: acceptgate.OutcomeAccepted,
		Results: []acceptgate.CheckResult{
			{Name: " permission-check ", Passed: false, Reason: "permission denied"},
			{Name: " timeout-check ", Passed: false, Reason: "command timeout"},
			{Name: " lookup-check ", Passed: false, Reason: "binary not found"},
			{Name: " pass-check ", Passed: true, Reason: "  skipped  "},
		},
	})
	service.emitVerificationLifecycleEvents(context.Background(), nil, controlplane.CompletionState{}, acceptgate.Report{})
	var nilService *Service
	nilService.emitVerificationLifecycleEvents(context.Background(), &state, controlplane.CompletionState{}, acceptgate.Report{})

	events := collectRuntimeEvents(service.Events())
	if len(events) != 6 {
		t.Fatalf("event count = %d, want 6", len(events))
	}

	finished, ok := events[len(events)-1].Payload.(VerificationFinishedPayload)
	if !ok {
		t.Fatalf("finished payload type = %T", events[len(events)-1].Payload)
	}
	if finished.ErrorClass != "" {
		t.Fatalf("ErrorClass = %q, want empty for accepted verification", finished.ErrorClass)
	}

	var gotClasses []string
	for _, event := range events[1 : len(events)-1] {
		payload, ok := event.Payload.(VerificationStageFinishedPayload)
		if !ok {
			t.Fatalf("stage payload type = %T", event.Payload)
		}
		gotClasses = append(gotClasses, payload.ErrorClass)
	}
	wantClasses := []string{"permission_denied", "timeout", "command_not_found", ""}
	for i := range wantClasses {
		if gotClasses[i] != wantClasses[i] {
			t.Fatalf("stage error class[%d] = %q, want %q", i, gotClasses[i], wantClasses[i])
		}
	}
}
