package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	providertypes "neo-code/internal/provider/types"
	"neo-code/internal/security"
	"neo-code/internal/subagent"
	"neo-code/internal/tools"
)

func TestSubAgentRuntimeToolExecutorListToolSpecs(t *testing.T) {
	t.Parallel()

	service := NewWithFactory(
		newRuntimeConfigManager(t),
		&stubToolManager{
			specs: []providertypes.ToolSpec{
				{Name: "filesystem_read_file"},
				{Name: "bash"},
			},
		},
		newMemoryStore(),
		&scriptedProviderFactory{provider: &scriptedProvider{}},
		nil,
	)
	executor := newSubAgentRuntimeToolExecutor(service)

	tests := []struct {
		name     string
		allow    []string
		wantSize int
	}{
		{name: "no allowlist", allow: nil, wantSize: 2},
		{name: "single allowlist", allow: []string{"bash"}, wantSize: 1},
		{name: "case-insensitive allowlist", allow: []string{"FILESYSTEM_READ_FILE"}, wantSize: 1},
		{name: "empty allowlist behaves as full set", allow: []string{""}, wantSize: 2},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			specs, err := executor.ListToolSpecs(context.Background(), subagent.ToolSpecListInput{
				SessionID:    "session-list-tools",
				Role:         subagent.RoleCoder,
				AllowedTools: tt.allow,
			})
			if err != nil {
				t.Fatalf("ListToolSpecs() error = %v", err)
			}
			if len(specs) != tt.wantSize {
				t.Fatalf("len(specs) = %d, want %d", len(specs), tt.wantSize)
			}
		})
	}
}

func TestSubAgentRuntimeToolExecutorExecuteToolEvents(t *testing.T) {
	t.Parallel()

	t.Run("allow should emit started and result", func(t *testing.T) {
		t.Parallel()

		toolManager := &stubToolManager{
			executeFn: func(ctx context.Context, input tools.ToolCallInput) (tools.ToolResult, error) {
				return tools.ToolResult{
					ToolCallID: input.ID,
					Name:       input.Name,
					Content:    "ok",
					Metadata:   map[string]any{"truncated": true},
				}, nil
			},
		}
		service := NewWithFactory(
			newRuntimeConfigManager(t),
			toolManager,
			newMemoryStore(),
			&scriptedProviderFactory{provider: &scriptedProvider{}},
			nil,
		)
		executor := newSubAgentRuntimeToolExecutor(service)

		result, err := executor.ExecuteTool(context.Background(), subagent.ToolExecutionInput{
			RunID:     "run-subagent-tool-allow",
			SessionID: "session-subagent-tool-allow",
			TaskID:    "task-subagent-tool-allow",
			Role:      subagent.RoleCoder,
			AgentID:   "subagent:allow",
			Workdir:   t.TempDir(),
			Timeout:   2 * time.Second,
			Call: providertypes.ToolCall{
				ID:        "call-allow",
				Name:      "filesystem_read_file",
				Arguments: `{"path":"README.md"}`,
			},
		})
		if err != nil {
			t.Fatalf("ExecuteTool() error = %v", err)
		}
		if result.Decision != permissionDecisionAllow {
			t.Fatalf("decision = %q, want %q", result.Decision, permissionDecisionAllow)
		}

		events := collectRuntimeEvents(service.Events())
		assertEventSequence(t, events, []EventType{EventSubAgentToolCallStarted, EventSubAgentToolCallResult})
		assertSubAgentToolEventPayload(t, events, EventSubAgentToolCallResult, "filesystem_read_file", permissionDecisionAllow, true)
	})

	t.Run("permission deny should emit denied", func(t *testing.T) {
		t.Parallel()

		registry := tools.NewRegistry()
		registry.Register(&stubTool{name: "bash", content: "ok"})
		gateway, err := security.NewStaticGateway(security.DecisionDeny, nil)
		if err != nil {
			t.Fatalf("NewStaticGateway() error = %v", err)
		}
		manager, err := tools.NewManager(registry, gateway, nil)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		service := NewWithFactory(
			newRuntimeConfigManager(t),
			manager,
			newMemoryStore(),
			&scriptedProviderFactory{provider: &scriptedProvider{}},
			nil,
		)
		executor := newSubAgentRuntimeToolExecutor(service)

		result, execErr := executor.ExecuteTool(context.Background(), subagent.ToolExecutionInput{
			RunID:     "run-subagent-tool-deny",
			SessionID: "session-subagent-tool-deny",
			TaskID:    "task-subagent-tool-deny",
			Role:      subagent.RoleCoder,
			AgentID:   "subagent:deny",
			Workdir:   t.TempDir(),
			Timeout:   2 * time.Second,
			Call: providertypes.ToolCall{
				ID:        "call-deny",
				Name:      "bash",
				Arguments: `{"command":"echo hi"}`,
			},
		})
		if execErr == nil {
			t.Fatalf("expected permission error")
		}
		if !errors.Is(execErr, tools.ErrPermissionDenied) {
			t.Fatalf("expected ErrPermissionDenied, got %v", execErr)
		}
		if result.Decision != string(security.DecisionDeny) {
			t.Fatalf("decision = %q, want %q", result.Decision, security.DecisionDeny)
		}

		events := collectRuntimeEvents(service.Events())
		assertEventSequence(t, events, []EventType{EventSubAgentToolCallStarted, EventSubAgentToolCallDenied})
		assertSubAgentToolEventPayload(t, events, EventSubAgentToolCallDenied, "bash", string(security.DecisionDeny), false)
	})
}
