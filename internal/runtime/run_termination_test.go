package runtime

import (
	"context"
	"errors"
	"testing"

	"neo-code/internal/runtime/controlplane"
	agentsession "neo-code/internal/session"
)

func TestEmitRunTerminationEmitsStopReasonOnce(t *testing.T) {
	t.Parallel()

	s := NewWithFactory(newRuntimeConfigManager(t), &stubToolManager{}, newMemoryStore(), &scriptedProviderFactory{
		provider: &scriptedProvider{},
	}, nil)

	state := newRunState("run-t", agentsessionFixture(t))
	errSample := errors.New("boom")

	s.emitRunTermination(context.Background(), UserInput{RunID: "run-t", SessionID: state.session.ID}, &state, errSample)
	s.emitRunTermination(context.Background(), UserInput{RunID: "run-t", SessionID: state.session.ID}, &state, errSample)

	events := collectRuntimeEvents(s.Events())
	var stops int
	for _, e := range events {
		if e.Type == EventStopReasonDecided {
			stops++
			p, ok := e.Payload.(StopReasonDecidedPayload)
			if !ok {
				t.Fatalf("expected StopReasonDecidedPayload, got %#v", e.Payload)
			}
			if p.Reason != controlplane.StopReasonError {
				t.Fatalf("reason = %q, want error", p.Reason)
			}
		}
	}
	if stops != 1 {
		t.Fatalf("expected exactly one stop_reason_decided, got %d", stops)
	}
}

func agentsessionFixture(t *testing.T) agentsession.Session {
	t.Helper()
	s := agentsession.New("t")
	s.ID = "sess-t"
	return s
}
