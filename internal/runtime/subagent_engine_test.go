package runtime

import (
	"context"
	"strings"
	"testing"

	providertypes "neo-code/internal/provider/types"
	"neo-code/internal/subagent"
	"neo-code/internal/tools"
)

func TestRuntimeSubAgentEngineRunStepToolLoopSuccess(t *testing.T) {
	t.Parallel()

	manager := newRuntimeConfigManager(t)
	providerImpl := &scriptedProvider{
		responses: []scriptedResponse{
			{
				Message: providertypes.Message{
					Role: providertypes.RoleAssistant,
					ToolCalls: []providertypes.ToolCall{
						{ID: "call-1", Name: "filesystem_read_file", Arguments: `{"path":"README.md"}`},
					},
				},
				FinishReason: "tool_calls",
			},
			{
				Message: providertypes.Message{
					Role: providertypes.RoleAssistant,
					Parts: []providertypes.ContentPart{
						providertypes.NewTextPart(`{"summary":"done","findings":["f1"],"patches":["p1"],"risks":["r1"],"next_actions":["n1"],"artifacts":["a1"]}`),
					},
				},
				FinishReason: "stop",
			},
		},
	}
	toolManager := &stubToolManager{
		specs: []providertypes.ToolSpec{{Name: "filesystem_read_file", Schema: map[string]any{"type": "object"}}},
		executeFn: func(ctx context.Context, input tools.ToolCallInput) (tools.ToolResult, error) {
			return tools.ToolResult{
				ToolCallID: input.ID,
				Name:       input.Name,
				Content:    "file content",
				IsError:    false,
				Metadata:   map[string]any{"truncated": true},
			}, nil
		},
	}
	service := NewWithFactory(manager, toolManager, newMemoryStore(), &scriptedProviderFactory{provider: providerImpl}, nil)
	policy, err := subagent.DefaultRolePolicy(subagent.RoleCoder)
	if err != nil {
		t.Fatalf("DefaultRolePolicy() error = %v", err)
	}
	policy.ToolUseMode = subagent.ToolUseModeAuto
	policy.MaxToolCallsPerStep = 2
	engine := runtimeSubAgentEngine{service: service, role: subagent.RoleCoder, policy: policy}

	output, err := engine.RunStep(context.Background(), subagent.StepInput{
		Role:   subagent.RoleCoder,
		Policy: policy,
		Task:   subagent.Task{ID: "task-1", Goal: "read file and summarize"},
		Budget: subagent.Budget{MaxSteps: 4},
		Capability: subagent.Capability{
			AllowedTools: []string{"filesystem_read_file"},
		},
		RunID:     "run-subagent-step-success",
		SessionID: "session-subagent-step-success",
		AgentID:   "subagent:task-1",
		Workdir:   t.TempDir(),
		Executor:  newSubAgentRuntimeToolExecutor(service),
	})
	if err != nil {
		t.Fatalf("RunStep() error = %v", err)
	}
	if !output.Done {
		t.Fatalf("expected step done")
	}
	if output.Output.Summary != "done" {
		t.Fatalf("summary = %q, want %q", output.Output.Summary, "done")
	}
	if toolManager.executeCalls != 1 {
		t.Fatalf("tool execute calls = %d, want 1", toolManager.executeCalls)
	}
	if providerImpl.callCount != 2 {
		t.Fatalf("provider calls = %d, want 2", providerImpl.callCount)
	}

	events := collectRuntimeEvents(service.Events())
	assertEventSequence(t, events, []EventType{EventSubAgentToolCallStarted, EventSubAgentToolCallResult})
	assertSubAgentToolEventPayload(t, events, EventSubAgentToolCallResult, "filesystem_read_file", permissionDecisionAllow, true)
}

func TestRuntimeSubAgentEngineRunStepCapabilityDenied(t *testing.T) {
	t.Parallel()

	manager := newRuntimeConfigManager(t)
	providerImpl := &scriptedProvider{
		responses: []scriptedResponse{
			{
				Message: providertypes.Message{
					Role: providertypes.RoleAssistant,
					ToolCalls: []providertypes.ToolCall{
						{ID: "call-bash", Name: "bash", Arguments: `{"command":"echo hi"}`},
					},
				},
				FinishReason: "tool_calls",
			},
			{
				Message: providertypes.Message{
					Role: providertypes.RoleAssistant,
					Parts: []providertypes.ContentPart{
						providertypes.NewTextPart(`{"summary":"fallback","findings":["f1"],"patches":["p1"],"risks":["r1"],"next_actions":["n1"],"artifacts":["a1"]}`),
					},
				},
				FinishReason: "stop",
			},
		},
	}
	toolManager := &stubToolManager{
		specs: []providertypes.ToolSpec{{Name: "bash", Schema: map[string]any{"type": "object"}}},
	}
	service := NewWithFactory(manager, toolManager, newMemoryStore(), &scriptedProviderFactory{provider: providerImpl}, nil)
	policy, err := subagent.DefaultRolePolicy(subagent.RoleCoder)
	if err != nil {
		t.Fatalf("DefaultRolePolicy() error = %v", err)
	}
	policy.MaxToolCallsPerStep = 2
	engine := runtimeSubAgentEngine{service: service, role: subagent.RoleCoder, policy: policy}

	stepOutput, err := engine.RunStep(context.Background(), subagent.StepInput{
		Role:   subagent.RoleCoder,
		Policy: policy,
		Task:   subagent.Task{ID: "task-cap-deny", Goal: "execute bash"},
		Budget: subagent.Budget{MaxSteps: 4},
		Capability: subagent.Capability{
			AllowedTools: []string{"filesystem_read_file"},
		},
		RunID:     "run-subagent-cap-deny",
		SessionID: "session-subagent-cap-deny",
		AgentID:   "subagent:task-cap-deny",
		Workdir:   t.TempDir(),
		Executor:  newSubAgentRuntimeToolExecutor(service),
	})
	if err != nil {
		t.Fatalf("RunStep() error = %v", err)
	}
	if !stepOutput.Done {
		t.Fatalf("expected step done")
	}
	if toolManager.executeCalls != 0 {
		t.Fatalf("tool execute calls = %d, want 0", toolManager.executeCalls)
	}

	events := collectRuntimeEvents(service.Events())
	assertEventSequence(t, events, []EventType{EventSubAgentToolCallDenied})
	assertSubAgentToolEventPayload(t, events, EventSubAgentToolCallDenied, "bash", permissionDecisionDeny, false)
}

func TestRuntimeSubAgentEngineRunStepRequiredModeWithoutToolFails(t *testing.T) {
	t.Parallel()

	manager := newRuntimeConfigManager(t)
	providerImpl := &scriptedProvider{
		responses: []scriptedResponse{
			{
				Message: providertypes.Message{
					Role: providertypes.RoleAssistant,
					Parts: []providertypes.ContentPart{
						providertypes.NewTextPart(`{"summary":"done","findings":["f1"],"patches":["p1"],"risks":["r1"],"next_actions":["n1"],"artifacts":["a1"]}`),
					},
				},
				FinishReason: "stop",
			},
		},
	}
	toolManager := &stubToolManager{
		specs: []providertypes.ToolSpec{{Name: "filesystem_read_file", Schema: map[string]any{"type": "object"}}},
	}
	service := NewWithFactory(manager, toolManager, newMemoryStore(), &scriptedProviderFactory{provider: providerImpl}, nil)
	policy, err := subagent.DefaultRolePolicy(subagent.RoleCoder)
	if err != nil {
		t.Fatalf("DefaultRolePolicy() error = %v", err)
	}
	policy.ToolUseMode = subagent.ToolUseModeRequired
	policy.MaxToolCallsPerStep = 1
	engine := runtimeSubAgentEngine{service: service, role: subagent.RoleCoder, policy: policy}

	_, err = engine.RunStep(context.Background(), subagent.StepInput{
		Role:       subagent.RoleCoder,
		Policy:     policy,
		Task:       subagent.Task{ID: "task-required", Goal: "must call tool"},
		Budget:     subagent.Budget{MaxSteps: 2},
		Capability: subagent.Capability{AllowedTools: []string{"filesystem_read_file"}},
		RunID:      "run-subagent-required",
		SessionID:  "session-subagent-required",
		AgentID:    "subagent:task-required",
		Workdir:    t.TempDir(),
		Executor:   newSubAgentRuntimeToolExecutor(service),
	})
	if err == nil || !strings.Contains(err.Error(), "requires at least one tool call") {
		t.Fatalf("expected required-mode error, got %v", err)
	}
}

func TestRuntimeSubAgentEngineFallbackWhenRuntimeUnavailable(t *testing.T) {
	t.Parallel()

	policy, err := subagent.DefaultRolePolicy(subagent.RoleReviewer)
	if err != nil {
		t.Fatalf("DefaultRolePolicy() error = %v", err)
	}
	engine := runtimeSubAgentEngine{
		service: nil,
		role:    subagent.RoleReviewer,
		policy:  policy,
	}
	step, err := engine.RunStep(context.Background(), subagent.StepInput{
		Role:   subagent.RoleReviewer,
		Policy: policy,
		Task:   subagent.Task{ID: "task-fallback", Goal: "review"},
	})
	if err != nil {
		t.Fatalf("RunStep() error = %v", err)
	}
	if !step.Done {
		t.Fatalf("expected fallback step done")
	}
	if step.Output.Summary != "review" {
		t.Fatalf("summary = %q, want %q", step.Output.Summary, "review")
	}
}

func TestParseSubAgentOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:  "plain json",
			input: `{"summary":"s","findings":["f"],"patches":["p"],"risks":["r"],"next_actions":["n"],"artifacts":["a"]}`,
		},
		{
			name:  "json with prefix and suffix",
			input: "result:\n```json\n{\"summary\":\"s\",\"findings\":[\"f\"],\"patches\":[\"p\"],\"risks\":[\"r\"],\"next_actions\":[\"n\"],\"artifacts\":[\"a\"]}\n```\nend",
		},
		{
			name:    "invalid json",
			input:   `summary only`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseSubAgentOutput(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSubAgentOutput() error = %v", err)
			}
			if got.Summary != "s" {
				t.Fatalf("summary = %q, want %q", got.Summary, "s")
			}
		})
	}
}

func assertSubAgentToolEventPayload(
	t *testing.T,
	events []RuntimeEvent,
	eventType EventType,
	toolName string,
	decision string,
	truncated bool,
) {
	t.Helper()
	for _, event := range events {
		if event.Type != eventType {
			continue
		}
		payload, ok := event.Payload.(SubAgentToolCallEventPayload)
		if !ok {
			t.Fatalf("payload type = %T, want SubAgentToolCallEventPayload", event.Payload)
		}
		if payload.ToolName != toolName {
			t.Fatalf("tool_name = %q, want %q", payload.ToolName, toolName)
		}
		if payload.Decision != decision {
			t.Fatalf("decision = %q, want %q", payload.Decision, decision)
		}
		if payload.Truncated != truncated {
			t.Fatalf("truncated = %v, want %v", payload.Truncated, truncated)
		}
		return
	}
	t.Fatalf("event %q not found", eventType)
}
