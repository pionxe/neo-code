package subagent

import (
	"context"
	"testing"

	providertypes "neo-code/internal/provider/types"
)

type noopToolExecutor struct{}

func (noopToolExecutor) ListToolSpecs(ctx context.Context, input ToolSpecListInput) ([]providertypes.ToolSpec, error) {
	_ = ctx
	_ = input
	return nil, nil
}

func (noopToolExecutor) ExecuteTool(ctx context.Context, input ToolExecutionInput) (ToolExecutionResult, error) {
	_ = ctx
	_ = input
	return ToolExecutionResult{}, nil
}

func TestWorkerStepPropagatesExecutionContext(t *testing.T) {
	t.Parallel()

	policy, err := DefaultRolePolicy(RoleCoder)
	if err != nil {
		t.Fatalf("DefaultRolePolicy() error = %v", err)
	}

	var captured StepInput
	worker, err := NewWorker(RoleCoder, policy, EngineFunc(func(ctx context.Context, input StepInput) (StepOutput, error) {
		_ = ctx
		captured = input
		return StepOutput{
			Done: true,
			Output: Output{
				Summary:     "done",
				Findings:    []string{"f1"},
				Patches:     []string{"p1"},
				Risks:       []string{"r1"},
				NextActions: []string{"n1"},
				Artifacts:   []string{"a1"},
			},
		}, nil
	}), withExecutionContext(ExecutionContext{ToolExecutor: noopToolExecutor{}}))
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}

	task := Task{
		ID:        "task-exec",
		Goal:      "goal",
		Workspace: "  /tmp/workspace  ",
		RunID:     "run-exec",
		SessionID: "session-exec",
		AgentID:   "agent-exec",
	}
	if err := worker.Start(task, Budget{MaxSteps: 1}, Capability{}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if _, err := worker.Step(context.Background()); err != nil {
		t.Fatalf("Step() error = %v", err)
	}

	if captured.RunID != "run-exec" {
		t.Fatalf("RunID = %q, want %q", captured.RunID, "run-exec")
	}
	if captured.SessionID != "session-exec" {
		t.Fatalf("SessionID = %q, want %q", captured.SessionID, "session-exec")
	}
	if captured.AgentID != "agent-exec" {
		t.Fatalf("AgentID = %q, want %q", captured.AgentID, "agent-exec")
	}
	if captured.Workdir != "/tmp/workspace" {
		t.Fatalf("Workdir = %q, want %q", captured.Workdir, "/tmp/workspace")
	}
	if captured.Executor == nil {
		t.Fatalf("expected executor to be injected")
	}
}

func TestRolePolicyToolDefaultsAndValidation(t *testing.T) {
	t.Parallel()

	policy, err := DefaultRolePolicy(RoleReviewer)
	if err != nil {
		t.Fatalf("DefaultRolePolicy() error = %v", err)
	}
	if policy.ToolUseMode != ToolUseModeAuto {
		t.Fatalf("ToolUseMode = %q, want %q", policy.ToolUseMode, ToolUseModeAuto)
	}
	if policy.MaxToolCallsPerStep <= 0 {
		t.Fatalf("MaxToolCallsPerStep = %d, want > 0", policy.MaxToolCallsPerStep)
	}

	tests := []struct {
		name    string
		mode    ToolUseMode
		limit   int
		wantErr bool
	}{
		{name: "valid auto", mode: ToolUseModeAuto, limit: 1, wantErr: false},
		{name: "valid required", mode: ToolUseModeRequired, limit: 2, wantErr: false},
		{name: "valid disabled", mode: ToolUseModeDisabled, limit: 3, wantErr: false},
		{name: "invalid mode", mode: ToolUseMode("unknown"), limit: 1, wantErr: true},
		{name: "invalid negative limit", mode: ToolUseModeAuto, limit: -1, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := RolePolicy{
				Role:                RoleReviewer,
				SystemPrompt:        "prompt",
				AllowedTools:        []string{"filesystem_read_file"},
				ToolUseMode:         tt.mode,
				MaxToolCallsPerStep: tt.limit,
				RequiredSections:    []string{"summary"},
			}
			err := p.Validate()
			if tt.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
