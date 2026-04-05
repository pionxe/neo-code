package bash

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"neo-code/internal/tools"
)

type Tool struct {
	root     string
	shell    string
	timeout  time.Duration
	executor SecurityExecutor
}

type input struct {
	Command string `json:"command"`
	Workdir string `json:"workdir,omitempty"`
}

func New(root string, shell string, timeout time.Duration) *Tool {
	executor := NewDefaultSecurityExecutor(root, shell, timeout)
	return &Tool{
		root:     root,
		shell:    shell,
		timeout:  timeout,
		executor: executor,
	}
}

// NewWithExecutor creates a bash tool using an injected security executor.
func NewWithExecutor(root string, shell string, timeout time.Duration, executor SecurityExecutor) *Tool {
	if executor == nil {
		executor = NewDefaultSecurityExecutor(root, shell, timeout)
	}
	return &Tool{
		root:     root,
		shell:    shell,
		timeout:  timeout,
		executor: executor,
	}
}

func (t *Tool) Name() string {
	return "bash"
}

func (t *Tool) Description() string {
	return "Execute a shell command inside the workspace with timeout and bounded output."
}

func (t *Tool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Shell command to execute.",
			},
			"workdir": map[string]any{
				"type":        "string",
				"description": "Optional working directory relative to the workspace root.",
			},
		},
		"required": []string{"command"},
	}
}

func (t *Tool) Execute(ctx context.Context, call tools.ToolCallInput) (tools.ToolResult, error) {
	var in input
	if err := json.Unmarshal(call.Arguments, &in); err != nil {
		return tools.NewErrorResult(t.Name(), "invalid arguments", err.Error(), nil), err
	}
	if t.executor == nil {
		err := errors.New("bash: security executor is nil")
		return tools.NewErrorResult(t.Name(), tools.NormalizeErrorReason(t.Name(), err), "", nil), err
	}

	return t.executor.Execute(ctx, call, in.Command, in.Workdir)
}
