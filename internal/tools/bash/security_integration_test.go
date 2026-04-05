package bash

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"neo-code/internal/security"
	"neo-code/internal/tools"
)

type stubSecurityExecutor struct {
	calls int
	err   error
}

func (e *stubSecurityExecutor) Execute(
	ctx context.Context,
	call tools.ToolCallInput,
	command string,
	requestedWorkdir string,
) (tools.ToolResult, error) {
	e.calls++
	if e.err != nil {
		return tools.NewErrorResult("bash", tools.NormalizeErrorReason("bash", e.err), "", nil), e.err
	}
	return tools.ToolResult{
		Name:    "bash",
		Content: "executed",
		Metadata: map[string]any{
			"command": command,
		},
	}, nil
}

func TestBashToolManagerPermissionDecisions(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	args, err := json.Marshal(map[string]string{
		"command": "echo hi",
	})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	tests := []struct {
		name        string
		rules       []security.Rule
		executorErr error
		expectErr   string
		expectCalls int
		expectBody  string
	}{
		{
			name:        "allow runs executor",
			expectCalls: 1,
			expectBody:  "executed",
		},
		{
			name: "deny blocks before executor",
			rules: []security.Rule{
				{ID: "deny-bash", Type: security.ActionTypeBash, Resource: "bash", Decision: security.DecisionDeny, Reason: "bash denied"},
			},
			expectErr:   "bash denied",
			expectCalls: 0,
			expectBody:  "reason: bash denied",
		},
		{
			name: "ask blocks before executor",
			rules: []security.Rule{
				{ID: "ask-bash", Type: security.ActionTypeBash, Resource: "bash", Decision: security.DecisionAsk, Reason: "need approval"},
			},
			expectErr:   "need approval",
			expectCalls: 0,
			expectBody:  "reason: need approval",
		},
		{
			name:        "executor error is returned",
			executorErr: errors.New("bash: failed"),
			expectErr:   "failed",
			expectCalls: 1,
			expectBody:  "reason: failed",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			executor := &stubSecurityExecutor{err: tt.executorErr}
			tool := NewWithExecutor(workspace, defaultShell(), 2*time.Second, executor)
			registry := tools.NewRegistry()
			registry.Register(tool)

			engine, err := security.NewStaticGateway(security.DecisionAllow, tt.rules)
			if err != nil {
				t.Fatalf("new static gateway: %v", err)
			}
			manager, err := tools.NewManager(registry, engine, security.NewWorkspaceSandbox())
			if err != nil {
				t.Fatalf("new manager: %v", err)
			}

			result, execErr := manager.Execute(context.Background(), tools.ToolCallInput{
				Name:      "bash",
				Arguments: args,
				Workdir:   workspace,
			})

			if tt.expectErr != "" {
				if execErr == nil || !strings.Contains(execErr.Error(), tt.expectErr) {
					t.Fatalf("expected error containing %q, got %v", tt.expectErr, execErr)
				}
			} else if execErr != nil {
				t.Fatalf("unexpected error: %v", execErr)
			}

			if executor.calls != tt.expectCalls {
				t.Fatalf("expected executor calls %d, got %d", tt.expectCalls, executor.calls)
			}
			if tt.expectBody != "" && !strings.Contains(result.Content, tt.expectBody) {
				t.Fatalf("expected result content containing %q, got %q", tt.expectBody, result.Content)
			}
		})
	}
}
