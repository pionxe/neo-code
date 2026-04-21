package runtime

import (
	"context"
	"strings"

	"neo-code/internal/subagent"
	"neo-code/internal/tools"
)

// runtimeSubAgentInvoker 复用 runtime.RunSubAgentTask，为工具层提供即时子代理执行能力。
type runtimeSubAgentInvoker struct {
	service    *Service
	runID      string
	sessionID  string
	callerID   string
	defaultDir string
}

// newRuntimeSubAgentInvoker 构造绑定当前运行上下文的子代理调用桥接器。
func newRuntimeSubAgentInvoker(
	service *Service,
	runID string,
	sessionID string,
	callerID string,
	workdir string,
) tools.SubAgentInvoker {
	if service == nil {
		return nil
	}
	return runtimeSubAgentInvoker{
		service:    service,
		runID:      strings.TrimSpace(runID),
		sessionID:  strings.TrimSpace(sessionID),
		callerID:   strings.TrimSpace(callerID),
		defaultDir: strings.TrimSpace(workdir),
	}
}

// Run 调用 runtime 子代理执行链路，并把结果映射为工具层统一结构。
func (i runtimeSubAgentInvoker) Run(ctx context.Context, input tools.SubAgentRunInput) (tools.SubAgentRunResult, error) {
	role := input.Role
	if !role.Valid() {
		role = subagent.RoleCoder
	}

	taskID := strings.TrimSpace(input.TaskID)
	if taskID == "" {
		taskID = "spawn-subagent-inline"
	}
	workdir := strings.TrimSpace(input.Workdir)
	if workdir == "" {
		workdir = i.defaultDir
	}

	runID := strings.TrimSpace(input.RunID)
	if runID == "" {
		runID = i.runID
	}
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" {
		sessionID = i.sessionID
	}
	callerID := strings.TrimSpace(input.CallerAgent)
	if callerID == "" {
		callerID = i.callerID
	}

	result, err := i.service.RunSubAgentTask(ctx, SubAgentTaskInput{
		RunID:     runID,
		SessionID: sessionID,
		AgentID:   callerID,
		Role:      role,
		Task: subagent.Task{
			ID:             taskID,
			Goal:           strings.TrimSpace(input.Goal),
			ExpectedOutput: strings.TrimSpace(input.ExpectedOut),
			Workspace:      workdir,
		},
		Budget: subagent.Budget{
			MaxSteps: input.MaxSteps,
			Timeout:  input.Timeout,
		},
		Capability: subagent.Capability{
			AllowedTools: append([]string(nil), input.AllowedTools...),
			AllowedPaths: append([]string(nil), input.AllowedPaths...),
		},
	})

	return tools.SubAgentRunResult{
		Role:       result.Role,
		TaskID:     result.TaskID,
		State:      result.State,
		StopReason: result.StopReason,
		StepCount:  result.StepCount,
		Output:     result.Output,
		Error:      strings.TrimSpace(result.Error),
	}, err
}
