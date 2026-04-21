package runtime

import (
	"context"
	"testing"
	"time"

	"neo-code/internal/subagent"
	"neo-code/internal/tools"
)

func newInvokerSuccessSubAgentFactory() subagent.Factory {
	return subagent.NewWorkerFactory(func(role subagent.Role, policy subagent.RolePolicy) subagent.Engine {
		_ = role
		_ = policy
		return subagent.EngineFunc(func(ctx context.Context, input subagent.StepInput) (subagent.StepOutput, error) {
			_ = ctx
			return subagent.StepOutput{
				Done:  true,
				Delta: "completed",
				Output: subagent.Output{
					Summary:     "completed " + input.Task.ID,
					Findings:    []string{"ok"},
					Patches:     []string{"none"},
					Risks:       []string{"low"},
					NextActions: []string{"continue"},
					Artifacts:   []string{input.Task.ID + ".artifact"},
				},
			}, nil
		})
	})
}

func TestNewRuntimeSubAgentInvokerNilService(t *testing.T) {
	t.Parallel()

	if got := newRuntimeSubAgentInvoker(nil, "run", "session", "agent", ""); got != nil {
		t.Fatalf("expected nil invoker when service is nil")
	}
}

func TestRuntimeSubAgentInvokerRun(t *testing.T) {
	t.Parallel()

	service := NewWithFactory(
		newRuntimeConfigManager(t),
		&stubToolManager{},
		newMemoryStore(),
		&scriptedProviderFactory{provider: &scriptedProvider{}},
		nil,
	)
	service.SetSubAgentFactory(newInvokerSuccessSubAgentFactory())

	invoker := newRuntimeSubAgentInvoker(service, "run-inline", "session-inline", "agent-main", t.TempDir())
	if invoker == nil {
		t.Fatalf("expected non-nil invoker")
	}

	result, err := invoker.Run(context.Background(), tools.SubAgentRunInput{
		Role:        subagent.RoleCoder,
		TaskID:      "task-inline",
		Goal:        "inspect and summarize",
		ExpectedOut: "json summary",
		Timeout:     10 * time.Second,
		MaxSteps:    2,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.TaskID != "task-inline" {
		t.Fatalf("task id = %q, want task-inline", result.TaskID)
	}
	if result.State != subagent.StateSucceeded {
		t.Fatalf("state = %q, want %q", result.State, subagent.StateSucceeded)
	}
}
