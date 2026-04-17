package services

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	agentruntime "neo-code/internal/runtime"
)

const permissionResolveTimeout = 10 * time.Second

// Runner 定义执行 runtime run 所需最小能力。
type Runner interface {
	Run(ctx context.Context, input agentruntime.UserInput) error
}

// PreparedRunner 定义“输入归一化 + run”链路所需最小能力。
// Submitter 定义 runtime 单入口提交所需的最小能力。
type Submitter interface {
	Submit(ctx context.Context, input agentruntime.PrepareInput) error
}

// Compactor 定义执行 runtime compact 所需最小能力。
type Compactor interface {
	Compact(ctx context.Context, input agentruntime.CompactInput) (agentruntime.CompactResult, error)
}

// PermissionResolver 定义权限审批提交所需最小能力。
type PermissionResolver interface {
	ResolvePermission(ctx context.Context, input agentruntime.PermissionResolutionInput) error
}

// ListenForRuntimeEventCmd 监听 runtime 事件通道，并将结果映射为 UI 消息。
func ListenForRuntimeEventCmd(
	sub <-chan agentruntime.RuntimeEvent,
	eventMsg func(agentruntime.RuntimeEvent) tea.Msg,
	closedMsg func() tea.Msg,
) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-sub
		if !ok {
			return closedMsg()
		}
		return eventMsg(event)
	}
}

// RunAgentCmd 执行 runtime run，并将执行结果回传为 UI 消息。
func RunAgentCmd(
	runtime Runner,
	input agentruntime.UserInput,
	doneMsg func(error) tea.Msg,
) tea.Cmd {
	return func() tea.Msg {
		err := runtime.Run(context.Background(), input)
		return doneMsg(err)
	}
}

// RunPreparedAgentCmd 先执行输入归一化，再执行 runtime run，并将结果映射为 UI 消息。
// RunSubmitCmd 执行 runtime 单入口提交，并将结果映射为 UI 消息。
func RunSubmitCmd(runtime Submitter, input agentruntime.PrepareInput, doneMsg func(error) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		err := runtime.Submit(context.Background(), input)
		return doneMsg(err)
	}
}

// RunCompactCmd 执行 runtime compact，并将结果映射为 UI 消息。
func RunCompactCmd(
	runtime Compactor,
	input agentruntime.CompactInput,
	doneMsg func(error) tea.Msg,
) tea.Cmd {
	return func() tea.Msg {
		_, err := runtime.Compact(context.Background(), input)
		return doneMsg(err)
	}
}

// RunResolvePermissionCmd 提交权限审批决定，并将结果映射为 UI 消息。
func RunResolvePermissionCmd(
	runtime PermissionResolver,
	input agentruntime.PermissionResolutionInput,
	doneMsg func(agentruntime.PermissionResolutionInput, error) tea.Msg,
) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), permissionResolveTimeout)
		defer cancel()

		err := runtime.ResolvePermission(ctx, input)
		return doneMsg(input, err)
	}
}
