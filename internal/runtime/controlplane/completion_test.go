package controlplane

import "testing"

func TestEvaluateCompletionBlockedByPendingTodo(t *testing.T) {
	t.Parallel()

	state, completed := EvaluateCompletion(CompletionState{
		HasPendingAgentTodos: true,
	}, false)
	if completed {
		t.Fatalf("expected completion to be blocked")
	}
	if state.CompletionBlockedReason != CompletionBlockedReasonPendingTodo {
		t.Fatalf("blocked reason = %q, want %q", state.CompletionBlockedReason, CompletionBlockedReasonPendingTodo)
	}
}

func TestEvaluateCompletionBlockedByUnverifiedWrite(t *testing.T) {
	t.Parallel()

	state, completed := EvaluateCompletion(CompletionState{
		HasUnverifiedWrites: true,
	}, false)
	if completed {
		t.Fatalf("expected completion to be blocked")
	}
	if state.CompletionBlockedReason != CompletionBlockedReasonUnverifiedWrite {
		t.Fatalf("blocked reason = %q, want %q", state.CompletionBlockedReason, CompletionBlockedReasonUnverifiedWrite)
	}
}

func TestEvaluateCompletionBlockedAfterToolCalls(t *testing.T) {
	t.Parallel()

	state, completed := EvaluateCompletion(CompletionState{}, true)
	if completed {
		t.Fatalf("expected completion to be blocked after tool call turn")
	}
	if state.CompletionBlockedReason != CompletionBlockedReasonPostExecuteClosureRequired {
		t.Fatalf("blocked reason = %q, want %q", state.CompletionBlockedReason, CompletionBlockedReasonPostExecuteClosureRequired)
	}
}

func TestEvaluateCompletionAllowsSatisfiedClosure(t *testing.T) {
	t.Parallel()

	state, completed := EvaluateCompletion(CompletionState{}, false)
	if !completed {
		t.Fatalf("expected completion to succeed")
	}
	if state.CompletionBlockedReason != CompletionBlockedReasonNone {
		t.Fatalf("blocked reason = %q, want empty", state.CompletionBlockedReason)
	}
}
