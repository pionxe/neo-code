package runtime

import (
	"context"
	"errors"
	"testing"

	"neo-code/internal/config"
	providertypes "neo-code/internal/provider/types"
	"neo-code/internal/runtime/controlplane"
	"neo-code/internal/tools"
)

func TestProgressStreakStopsRun(t *testing.T) {
	t.Setenv("TEST_KEY", "dummy")

	cfg := config.Config{
		Providers:        []config.ProviderConfig{{Name: "test-progress", Driver: "test", BaseURL: "http://localhost", Model: "test", APIKeyEnv: "TEST_KEY"}},
		SelectedProvider: "test-progress",
		MaxLoops:         10, // High enough to not trigger max loops
		Workdir:          t.TempDir(),
	}

	toolManager := &stubToolManager{
		specs: []providertypes.ToolSpec{
			{Name: "tool_error"},
		},
		executeFn: func(ctx context.Context, input tools.ToolCallInput) (tools.ToolResult, error) {
			// Always return error to avoid generating progress
			return tools.ToolResult{Content: "error occurred", IsError: true}, nil
		},
	}

	providerFactory := &scriptedProviderFactory{
		provider: &scriptedProvider{
			chatFn: func(ctx context.Context, req providertypes.GenerateRequest, events chan<- providertypes.StreamEvent) error {
				// the model always decides to call the tool
				events <- providertypes.NewToolCallStartStreamEvent(0, "call_err", "tool_error")
				events <- providertypes.NewToolCallDeltaStreamEvent(0, "call_err", "{}")
				events <- providertypes.NewMessageDoneStreamEvent("tool_calls", nil)
				return nil
			},
		},
	}

	manager := config.NewManager(config.NewLoader(t.TempDir(), &cfg))

	service := NewWithFactory(
		manager,
		toolManager,
		newMemoryStore(),
		providerFactory,
		nil,
	)

	input := UserInput{
		RunID:   "run-progress",
		Content: "trigger error loop",
	}

	err := service.Run(context.Background(), input)
	if err == nil {
		t.Fatal("expected error from streak limit, got nil")
	}

	if !errors.Is(err, ErrNoProgressStreakLimit) {
		t.Fatalf("expected ErrNoProgressStreakLimit, got %v", err)
	}

	events := collectRuntimeEvents(service.Events())

	// Verify StopReason is error and specifies the streak limit
	assertEventContains(t, events, EventStopReasonDecided)

	for _, e := range events {
		if e.Type == EventStopReasonDecided {
			payload := e.Payload.(StopReasonDecidedPayload)
			if payload.Reason != controlplane.StopReasonError {
				t.Errorf("expected StopReasonError, got %s", payload.Reason)
			}
			if payload.Detail != ErrNoProgressStreakLimit.Error() {
				t.Errorf("expected detail to be %q, got %q", ErrNoProgressStreakLimit.Error(), payload.Detail)
			}
		}
	}
}
