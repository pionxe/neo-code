package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// CommandHookPayloadVersion 定义 command hook stdin 协议版本号，变更 stdin 结构时递增。
const CommandHookPayloadVersion = "1"

// CommandHookPayload 是通过 stdin 传给外部命令的单行 JSON。
type CommandHookPayload struct {
	PayloadVersion string         `json:"payload_version"`
	HookID         string         `json:"hook_id"`
	Point          string         `json:"point"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// CommandHookResponse 是外部命令通过 stdout 返回的单行 JSON。
type CommandHookResponse struct {
	Status      string          `json:"status"`
	Message     string          `json:"message,omitempty"`
	UpdateInput json.RawMessage `json:"update_input,omitempty"`
	Annotations []string        `json:"annotations,omitempty"`
}

// CommandHookSpec 描述一个 command hook 的执行参数。
type CommandHookSpec struct {
	HookID  string
	Point   HookPoint
	Command []string // argv 模式: [binary, arg1, arg2, ...]
	Shell   bool     // true = 通过 sh -c / powershell -Command 执行
	Workdir string
}

// BuildCommandPayload 构造传给外部命令的 stdin JSON payload。
func BuildCommandPayload(hookID string, point HookPoint, metadata map[string]any) CommandHookPayload {
	payload := CommandHookPayload{
		PayloadVersion: CommandHookPayloadVersion,
		HookID:         strings.TrimSpace(hookID),
		Point:          string(point),
	}
	if len(metadata) > 0 {
		payload.Metadata = metadata
	}
	return payload
}

// ParseCommandResponse 解析外部命令 stdout 输出的单行 JSON。
// 非 JSON 输入返回 error，调用方可退化为 exit code 兼容模式。
func ParseCommandResponse(raw []byte) (CommandHookResponse, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return CommandHookResponse{}, fmt.Errorf("empty stdout")
	}
	var resp CommandHookResponse
	if err := json.Unmarshal(trimmed, &resp); err != nil {
		return CommandHookResponse{}, fmt.Errorf("invalid JSON: %w", err)
	}
	normalized := strings.ToLower(strings.TrimSpace(resp.Status))
	switch normalized {
	case "pass", "block", "failed":
		resp.Status = normalized
	default:
		return CommandHookResponse{}, fmt.Errorf("invalid status %q", resp.Status)
	}
	return resp, nil
}

// RunCommandHook 执行外部命令并返回结构化的 HookResult。
func RunCommandHook(ctx context.Context, spec CommandHookSpec, input HookContext) HookResult {
	payload := BuildCommandPayload(spec.HookID, spec.Point, input.Metadata)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return HookResult{
			HookID:  spec.HookID,
			Point:   spec.Point,
			Status:  HookResultFailed,
			Message: fmt.Sprintf("command hook marshal payload failed: %v", err),
			Error:   err.Error(),
		}
	}
	payloadBytes = append(payloadBytes, '\n')

	cmd := buildExecCmd(ctx, spec)
	cmd.Dir = spec.Workdir
	cmd.Env = buildCommandEnv(spec)
	cmd.Stdin = bytes.NewReader(payloadBytes)

	stdout, err := cmd.Output()
	message := strings.TrimSpace(string(stdout))

	// 尝试解析 stdout JSON 协议
	resp, parseErr := ParseCommandResponse(stdout)
	if parseErr == nil {
		return buildResultFromResponse(spec, resp)
	}

	// 退化模式: stdout 非 JSON，按 exit code 推断状态
	return buildResultFromExitCode(ctx, spec, err, message)
}

func buildExecCmd(ctx context.Context, spec CommandHookSpec) *exec.Cmd {
	if spec.Shell && len(spec.Command) > 0 {
		shell := spec.Command[0]
		if runtime.GOOS == "windows" {
			return exec.CommandContext(ctx, "powershell", "-Command", shell)
		}
		return exec.CommandContext(ctx, "sh", "-c", shell)
	}
	if len(spec.Command) == 1 {
		return exec.CommandContext(ctx, spec.Command[0])
	}
	return exec.CommandContext(ctx, spec.Command[0], spec.Command[1:]...)
}

func buildCommandEnv(spec CommandHookSpec) []string {
	env := []string{
		"NEOCODE_HOOK_HOOK_ID=" + spec.HookID,
		"NEOCODE_HOOK_POINT=" + string(spec.Point),
		"NEOCODE_HOOK_PAYLOAD_VERSION=" + CommandHookPayloadVersion,
	}
	if runtime.GOOS == "windows" {
		if sd := os.Getenv("SystemDrive"); sd != "" {
			env = append(env, "SystemDrive="+sd)
		}
	}
	return env
}

func buildResultFromResponse(spec CommandHookSpec, resp CommandHookResponse) HookResult {
	result := HookResult{
		HookID:  spec.HookID,
		Point:   spec.Point,
		Message: strings.TrimSpace(resp.Message),
	}
	switch resp.Status {
	case "pass":
		result.Status = HookResultPass
	case "block":
		result.Status = HookResultBlock
	case "failed":
		result.Status = HookResultFailed
		if result.Message == "" {
			result.Message = "hook returned failed status"
		}
		result.Error = result.Message
	}
	if len(resp.Annotations) > 0 {
		result.Metadata.Annotations = resp.Annotations
	}
	if len(resp.UpdateInput) > 0 {
		result.Metadata.UpdateInput = resp.UpdateInput
	}
	return result
}

func buildResultFromExitCode(ctx context.Context, spec CommandHookSpec, err error, message string) HookResult {
	result := HookResult{
		HookID:  spec.HookID,
		Point:   spec.Point,
		Message: message,
	}
	if err == nil {
		result.Status = HookResultPass
		return result
	}
	// 上下文取消/超时优先判定为 failed
	if ctx.Err() != nil {
		result.Status = HookResultFailed
		if result.Message == "" {
			result.Message = fmt.Sprintf("command %v", ctx.Err())
		}
		result.Error = ctx.Err().Error()
		return result
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		switch code {
		case 1, 2:
			result.Status = HookResultBlock
		default:
			result.Status = HookResultFailed
			if result.Message == "" {
				result.Message = fmt.Sprintf("command exited with code %d", code)
			}
			result.Error = err.Error()
		}
		return result
	}
	result.Status = HookResultFailed
	if result.Message == "" {
		result.Message = err.Error()
	}
	result.Error = err.Error()
	return result
}
