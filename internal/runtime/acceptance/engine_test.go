package acceptance

import (
	"context"
	"testing"

	"neo-code/internal/runtime/controlplane"
	"neo-code/internal/runtime/verify"
	agentsession "neo-code/internal/session"
)

type staticPolicy struct {
	verifiers []verify.FinalVerifier
	err       error
}

func (p staticPolicy) ResolveVerifiers(input verify.FinalVerifyInput) ([]verify.FinalVerifier, error) {
	_ = input
	return p.verifiers, p.err
}

type staticVerifier struct {
	name   string
	result verify.VerificationResult
}

func (v staticVerifier) Name() string { return v.name }
func (v staticVerifier) VerifyFinal(ctx context.Context, input verify.FinalVerifyInput) (verify.VerificationResult, error) {
	_ = ctx
	_ = input
	return v.result, nil
}

func TestEngineEvaluateFinal(t *testing.T) {
	t.Parallel()

	makeInput := func() FinalAcceptanceInput {
		return FinalAcceptanceInput{
			CompletionGate: CompletionGateDecision{Passed: true},
			VerificationInput: verify.FinalVerifyInput{
				TaskState: verify.TaskStateSnapshot{VerificationProfile: string(agentsession.VerificationProfileTaskOnly)},
			},
		}
	}

	t.Run("completion gate false returns continue", func(t *testing.T) {
		t.Parallel()
		decision, err := NewEngine(staticPolicy{}).EvaluateFinal(context.Background(), FinalAcceptanceInput{
			CompletionGate: CompletionGateDecision{Passed: false},
		})
		if err != nil {
			t.Fatalf("EvaluateFinal() error = %v", err)
		}
		if decision.Status != AcceptanceContinue {
			t.Fatalf("status = %q, want continue", decision.Status)
		}
	})

	t.Run("invalid profile becomes structured failed decision", func(t *testing.T) {
		t.Parallel()
		decision, err := NewEngine(staticPolicy{err: context.DeadlineExceeded}).EvaluateFinal(context.Background(), makeInput())
		if err != nil {
			t.Fatalf("EvaluateFinal() error = %v", err)
		}
		if decision.Status != AcceptanceFailed || decision.StopReason != controlplane.StopReasonVerificationConfigMissing {
			t.Fatalf("unexpected decision: %+v", decision)
		}
	})

	t.Run("soft block returns continue", func(t *testing.T) {
		t.Parallel()
		decision, err := NewEngine(staticPolicy{
			verifiers: []verify.FinalVerifier{
				staticVerifier{name: "todo", result: verify.VerificationResult{Name: "todo", Status: verify.VerificationSoftBlock}},
			},
		}).EvaluateFinal(context.Background(), makeInput())
		if err != nil {
			t.Fatalf("EvaluateFinal() error = %v", err)
		}
		if decision.Status != AcceptanceContinue {
			t.Fatalf("status = %q, want continue", decision.Status)
		}
	})

	t.Run("hard block returns incomplete", func(t *testing.T) {
		t.Parallel()
		decision, err := NewEngine(staticPolicy{
			verifiers: []verify.FinalVerifier{
				staticVerifier{name: "todo", result: verify.VerificationResult{Name: "todo", Status: verify.VerificationHardBlock, WaitingExternal: true}},
			},
		}).EvaluateFinal(context.Background(), makeInput())
		if err != nil {
			t.Fatalf("EvaluateFinal() error = %v", err)
		}
		if decision.Status != AcceptanceIncomplete || decision.StopReason != controlplane.StopReasonTodoWaitingExternal {
			t.Fatalf("unexpected decision: %+v", decision)
		}
	})

	t.Run("fail returns failed", func(t *testing.T) {
		t.Parallel()
		decision, err := NewEngine(staticPolicy{
			verifiers: []verify.FinalVerifier{
				staticVerifier{name: "build", result: verify.VerificationResult{Name: "build", Status: verify.VerificationFail, ErrorClass: verify.ErrorClassEnvMissing}},
			},
		}).EvaluateFinal(context.Background(), makeInput())
		if err != nil {
			t.Fatalf("EvaluateFinal() error = %v", err)
		}
		if decision.Status != AcceptanceFailed || decision.StopReason != controlplane.StopReasonVerificationConfigMissing {
			t.Fatalf("unexpected decision: %+v", decision)
		}
	})

	t.Run("all pass returns accepted", func(t *testing.T) {
		t.Parallel()
		decision, err := NewEngine(staticPolicy{
			verifiers: []verify.FinalVerifier{
				staticVerifier{name: "todo", result: verify.VerificationResult{Name: "todo", Status: verify.VerificationPass}},
			},
		}).EvaluateFinal(context.Background(), makeInput())
		if err != nil {
			t.Fatalf("EvaluateFinal() error = %v", err)
		}
		if decision.Status != AcceptanceAccepted {
			t.Fatalf("status = %q, want accepted", decision.Status)
		}
	})

	t.Run("retry exhausted no longer overrides final decision", func(t *testing.T) {
		t.Parallel()
		input := makeInput()
		input.VerificationInput.Todos = []verify.TodoSnapshot{{ID: "todo-1", Required: true, RetryCount: 1, RetryLimit: 1}}
		decision, err := NewEngine(staticPolicy{
			verifiers: []verify.FinalVerifier{
				staticVerifier{name: "todo", result: verify.VerificationResult{Name: "todo", Status: verify.VerificationPass}},
			},
		}).EvaluateFinal(context.Background(), input)
		if err != nil {
			t.Fatalf("EvaluateFinal() error = %v", err)
		}
		if decision.Status != AcceptanceAccepted || decision.StopReason != controlplane.StopReasonAccepted {
			t.Fatalf("unexpected decision: %+v", decision)
		}
	})
}
