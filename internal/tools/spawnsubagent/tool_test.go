package spawnsubagent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"neo-code/internal/security"
	"neo-code/internal/subagent"
	"neo-code/internal/tools"
)

type stubSubAgentInvoker struct {
	result tools.SubAgentRunResult
	err    error
	last   tools.SubAgentRunInput
}

func (i *stubSubAgentInvoker) Run(ctx context.Context, input tools.SubAgentRunInput) (tools.SubAgentRunResult, error) {
	if err := ctx.Err(); err != nil {
		return tools.SubAgentRunResult{}, err
	}
	i.last = input
	return i.result, i.err
}

func TestToolMetadata(t *testing.T) {
	t.Parallel()

	tool := New()
	if tool.Name() != tools.ToolNameSpawnSubAgent {
		t.Fatalf("Name() = %q, want %q", tool.Name(), tools.ToolNameSpawnSubAgent)
	}
	if strings.TrimSpace(tool.Description()) == "" {
		t.Fatalf("Description() should not be empty")
	}
	if tool.MicroCompactPolicy() != tools.MicroCompactPolicyCompact {
		t.Fatalf("MicroCompactPolicy() = %q, want compact", tool.MicroCompactPolicy())
	}
	schema := tool.Schema()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("Schema().properties type = %T, want map[string]any", schema["properties"])
	}
	if _, ok := properties["items"]; ok {
		t.Fatalf("Schema() should not include items")
	}
	modeProp, ok := properties["mode"].(map[string]any)
	if !ok {
		t.Fatalf("Schema().mode type = %T", properties["mode"])
	}
	enums, ok := modeProp["enum"].([]string)
	if !ok || len(enums) != 1 || enums[0] != spawnModeInline {
		t.Fatalf("mode enum = %#v, want [inline]", modeProp["enum"])
	}
}

func TestToolExecuteInlineMode(t *testing.T) {
	t.Parallel()

	tool := New()
	parentToken := &security.CapabilityToken{AllowedTools: []string{"spawn_subagent", "filesystem_read_file"}}
	invoker := &stubSubAgentInvoker{
		result: tools.SubAgentRunResult{
			Role:       subagent.RoleCoder,
			TaskID:     "inline-1",
			State:      subagent.StateSucceeded,
			StopReason: subagent.StopReasonCompleted,
			StepCount:  2,
			Output: subagent.Output{
				Summary:   "done",
				Findings:  []string{"f1"},
				Artifacts: []string{"a.txt"},
			},
		},
	}

	result, err := tool.Execute(context.Background(), tools.ToolCallInput{
		Name:            tools.ToolNameSpawnSubAgent,
		AgentID:         "agent-main",
		Workdir:         "/tmp/workdir",
		CapabilityToken: parentToken,
		SubAgentInvoker: invoker,
		Arguments: []byte(`{
			"prompt":"review code quality",
			"id":"inline-1",
			"role":"coder",
			"max_steps":3,
			"timeout_sec":90,
			"allowed_tools":["bash"],
			"allowed_paths":["/workspace"]
		}`),
	})
	if err != nil {
		t.Fatalf("Execute() inline error = %v", err)
	}
	if !strings.Contains(result.Content, "mode: inline") || !strings.Contains(result.Content, "state: succeeded") {
		t.Fatalf("unexpected inline content: %q", result.Content)
	}
	if invoker.last.TaskID != "inline-1" || invoker.last.Goal != "review code quality" {
		t.Fatalf("unexpected invoker input: %+v", invoker.last)
	}
	if invoker.last.Timeout != 90*time.Second {
		t.Fatalf("timeout = %v, want 90s", invoker.last.Timeout)
	}
	if invoker.last.ParentCapabilityToken == nil || len(invoker.last.ParentCapabilityToken.AllowedTools) == 0 {
		t.Fatalf("parent capability token should be forwarded: %+v", invoker.last.ParentCapabilityToken)
	}
}

func TestToolExecuteInlineModeErrors(t *testing.T) {
	t.Parallel()

	tool := New()
	_, err := tool.Execute(context.Background(), tools.ToolCallInput{
		Name:      tools.ToolNameSpawnSubAgent,
		Arguments: []byte(`{"prompt":"do something"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "subagent invoker is unavailable") {
		t.Fatalf("missing invoker error = %v", err)
	}

	invoker := &stubSubAgentInvoker{err: errors.New("subagent failed")}
	result, err := tool.Execute(context.Background(), tools.ToolCallInput{
		Name:            tools.ToolNameSpawnSubAgent,
		SubAgentInvoker: invoker,
		Arguments:       []byte(`{"prompt":"do something"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "subagent failed") {
		t.Fatalf("expected inline run error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected result.IsError=true")
	}
}

func TestToolExecuteErrorBranches(t *testing.T) {
	t.Parallel()

	tool := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := tool.Execute(ctx, tools.ToolCallInput{
		Name:      tools.ToolNameSpawnSubAgent,
		Arguments: []byte(`{"prompt":"x"}`),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() canceled err = %v, want context canceled", err)
	}
}

func TestParseSpawnInputRejectsItemsAndTodoMode(t *testing.T) {
	t.Parallel()

	_, err := parseSpawnInput([]byte(`{"items":[{"id":"t1","content":"x"}]}`))
	if err == nil || !strings.Contains(err.Error(), "items is not supported") {
		t.Fatalf("items rejection err = %v", err)
	}

	_, err = parseSpawnInput([]byte(`{"mode":"todo","prompt":"x"}`))
	if err == nil || !strings.Contains(err.Error(), `unsupported mode "todo"`) {
		t.Fatalf("todo mode rejection err = %v", err)
	}
}

func TestParseSpawnInputValidationBranches(t *testing.T) {
	t.Parallel()

	tooLong := strings.Repeat("x", maxSpawnTextLen+1)
	tooMany := make([]string, 0, maxSpawnListItems+1)
	for i := 0; i < maxSpawnListItems+1; i++ {
		tooMany = append(tooMany, fmt.Sprintf("item-%d", i))
	}
	hugeJSON := []byte(`{"prompt":"` + strings.Repeat("z", maxSpawnArgumentsBytes) + `"}`)

	tests := []struct {
		name    string
		raw     []byte
		wantErr string
	}{
		{name: "empty arguments", raw: nil, wantErr: "arguments is empty"},
		{name: "too large payload", raw: hugeJSON, wantErr: "payload exceeds"},
		{name: "invalid json", raw: []byte(`{`), wantErr: "parse arguments"},
		{name: "mode unsupported", raw: []byte(`{"mode":"dag","prompt":"x"}`), wantErr: "unsupported mode"},
		{name: "role invalid", raw: []byte(`{"prompt":"do it","role":"manager"}`), wantErr: `unsupported role "manager"`},
		{name: "prompt missing", raw: []byte(`{"id":"x"}`), wantErr: "prompt is empty"},
		{name: "prompt too long", raw: []byte(`{"prompt":"` + tooLong + `"}`), wantErr: "prompt exceeds max length"},
		{name: "id too long", raw: []byte(`{"prompt":"ok","id":"` + tooLong + `"}`), wantErr: "id exceeds max length"},
		{name: "expected output too long", raw: []byte(`{"prompt":"ok","expected_output":"` + tooLong + `"}`), wantErr: "expected_output exceeds max length"},
		{name: "allowed tools too many", raw: []byte(`{"prompt":"ok","allowed_tools":["` + strings.Join(tooMany, `","`) + `"]}`), wantErr: "allowed_tools exceeds max items"},
		{name: "allowed paths too many", raw: []byte(`{"prompt":"ok","allowed_paths":["` + strings.Join(tooMany, `","`) + `"]}`), wantErr: "allowed_paths exceeds max items"},
		{name: "negative max steps", raw: []byte(`{"prompt":"ok","max_steps":-1}`), wantErr: "max_steps must be >= 0"},
		{name: "negative timeout", raw: []byte(`{"prompt":"ok","timeout_sec":-1}`), wantErr: "timeout_sec must be >= 0"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseSpawnInput(tt.raw)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("parseSpawnInput() err = %v, want contains %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseSpawnInputContentFallback(t *testing.T) {
	t.Parallel()

	input, err := parseSpawnInput([]byte(`{"content":"  summarize  "}`))
	if err != nil {
		t.Fatalf("parseSpawnInput() error = %v", err)
	}
	if input.Prompt != "summarize" {
		t.Fatalf("prompt = %q, want summarize", input.Prompt)
	}
}

func TestDefaultInlineTaskID(t *testing.T) {
	t.Parallel()

	if got := defaultInlineTaskID("   "); got != "spawn-subagent-inline" {
		t.Fatalf("defaultInlineTaskID(blank) = %q", got)
	}
	if got := defaultInlineTaskID("review tests"); !strings.HasPrefix(got, "spawn-inline-") {
		t.Fatalf("defaultInlineTaskID(nonblank) = %q", got)
	}
}
