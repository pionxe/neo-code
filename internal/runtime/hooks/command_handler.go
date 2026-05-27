package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// CommandHookPayloadVersion 定义 command hook stdin 协议版本号，变更 stdin 结构时递增。
const CommandHookPayloadVersion = "1"

// maxCommandStdoutBytes 限制外部命令 stdout 最大读取字节数，防止 OOM。
const maxCommandStdoutBytes = 1 << 20 // 1 MiB

// CommandHookPayload 是通过 stdin 传给外部命令的单行 JSON。
type CommandHookPayload struct {
	PayloadVersion string         `json:"payload_version"`
	HookID         string         `json:"hook_id"`
	Point          string         `json:"point"`
	RunID          string         `json:"run_id,omitempty"`
	SessionID      string         `json:"session_id,omitempty"`
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

// ValidateCommandParams 校验 params.command 格式。
// 支持 []string / []any (argv 模式) 和 string + shell=true (shell 模式)。
// 此函数是 command hook params 校验的唯一真源，config / runtime 包均应调用此函数。
func ValidateCommandParams(params map[string]any) error {
	_, _, err := ParseCommandParams(params)
	return err
}

// ParseCommandParams 解析 params.command 为 argv 数组，支持 []string / []any / string+shell 三种格式。
// 返回解析后的 argv、是否为 shell 模式、以及校验错误。
func ParseCommandParams(params map[string]any) (argv []string, shell bool, err error) {
	if len(params) == 0 {
		return nil, false, fmt.Errorf("kind command requires params.command")
	}
	raw, ok := params["command"]
	if !ok || raw == nil {
		return nil, false, fmt.Errorf("kind command requires params.command")
	}
	switch v := raw.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil, false, fmt.Errorf("kind command requires params.command")
		}
		shellVal, _ := params["shell"].(bool)
		if !shellVal {
			return nil, false, fmt.Errorf("string params.command requires params.shell=true; use array format for argv mode")
		}
		return []string{trimmed}, true, nil
	case []string:
		if len(v) == 0 {
			return nil, false, fmt.Errorf("kind command requires non-empty params.command")
		}
		out := make([]string, 0, len(v))
		for _, s := range v {
			trimmed := strings.TrimSpace(s)
			if trimmed == "" {
				return nil, false, fmt.Errorf("params.command contains empty element")
			}
			out = append(out, trimmed)
		}
		return out, false, nil
	case []any:
		if len(v) == 0 {
			return nil, false, fmt.Errorf("kind command requires non-empty params.command")
		}
		out := make([]string, 0, len(v))
		for _, item := range v {
			s := strings.TrimSpace(fmt.Sprintf("%v", item))
			if s == "" {
				return nil, false, fmt.Errorf("params.command contains empty element")
			}
			out = append(out, s)
		}
		return out, false, nil
	default:
		return nil, false, fmt.Errorf("params.command must be a string (with shell=true) or an array of strings")
	}
}

// BuildCommandPayload 构造传给外部命令的 stdin JSON payload。
func BuildCommandPayload(hookID string, point HookPoint, input HookContext) CommandHookPayload {
	payload := CommandHookPayload{
		PayloadVersion: CommandHookPayloadVersion,
		HookID:         strings.TrimSpace(hookID),
		Point:          string(point),
		RunID:          strings.TrimSpace(input.RunID),
		SessionID:      strings.TrimSpace(input.SessionID),
	}
	if len(input.Metadata) > 0 {
		payload.Metadata = input.Metadata
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
// stdout 通过管道捕获并限制为 maxCommandStdoutBytes；stderr 捕获后在失败时附加到结果。
func RunCommandHook(ctx context.Context, spec CommandHookSpec, input HookContext) HookResult {
	payload := BuildCommandPayload(spec.HookID, spec.Point, input)
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

	stdout, stderrBytes, runErr := runAndCapture(cmd)

	// stdout 过大视为执行失败
	if int64(len(stdout)) > maxCommandStdoutBytes {
		msg := fmt.Sprintf("command hook stdout exceeded %d byte limit", maxCommandStdoutBytes)
		return HookResult{
			HookID:  spec.HookID,
			Point:   spec.Point,
			Status:  HookResultFailed,
			Message: msg,
			Error:   msg,
		}
	}

	message := strings.TrimSpace(string(stdout))

	// 非零 exit code 优先于 JSON status（防止恶意脚本声称 pass 但实际失败）
	if runErr != nil {
		return buildResultFromExitCode(ctx, spec, runErr, message, stdout, stderrBytes)
	}

	// exit code 0: 尝试解析 stdout JSON 协议
	resp, parseErr := ParseCommandResponse(stdout)
	if parseErr == nil {
		return buildResultFromResponse(spec, resp)
	}

	// 退化模式: exit 0 但 stdout 非 JSON，按 pass 处理
	return HookResult{
		HookID:  spec.HookID,
		Point:   spec.Point,
		Status:  HookResultPass,
		Message: message,
	}
}

// runAndCapture 执行命令，通过管道捕获 stdout（限制 maxCommandStdoutBytes），同时捕获 stderr。
func runAndCapture(cmd *exec.Cmd) (stdout, stderr []byte, runErr error) {
	cmd.Stderr = &bytes.Buffer{}

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	// 限制读取量，防止恶意脚本 OOM
	limitedReader := io.LimitReader(pipe, maxCommandStdoutBytes+1)
	var stdoutBuf bytes.Buffer
	_, copyErr := io.Copy(&stdoutBuf, limitedReader)
	stdout = stdoutBuf.Bytes()

	waitErr := cmd.Wait()

	if stderrBuf, ok := cmd.Stderr.(*bytes.Buffer); ok {
		stderr = stderrBuf.Bytes()
	}

	// pipe 读取错误优先
	if copyErr != nil {
		return stdout, stderr, fmt.Errorf("reading command stdout: %w", copyErr)
	}

	return stdout, stderr, waitErr
}

func buildExecCmd(ctx context.Context, spec CommandHookSpec) *exec.Cmd {
	if spec.Shell {
		if len(spec.Command) == 0 {
			// 不应到达此处（ParseCommandParams 已校验），防御性 panic
			panic("buildExecCmd: shell mode requires at least one command element")
		}
		shell := spec.Command[0]
		if runtime.GOOS == "windows" {
			return exec.CommandContext(ctx, "powershell", "-Command", shell)
		}
		return exec.CommandContext(ctx, "sh", "-c", shell)
	}
	if len(spec.Command) == 0 {
		panic("buildExecCmd: command requires at least one element")
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
		for _, key := range []string{"SystemRoot", "SystemDrive", "USERPROFILE"} {
			if v := os.Getenv(key); v != "" {
				env = append(env, key+"="+v)
			}
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

func buildResultFromExitCode(ctx context.Context, spec CommandHookSpec, err error, message string, stdout, stderr []byte) HookResult {
	result := HookResult{
		HookID:  spec.HookID,
		Point:   spec.Point,
		Message: message,
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
			result.Error = err.Error()
		default:
			result.Status = HookResultFailed
			if result.Message == "" {
				result.Message = fmt.Sprintf("command exited with code %d", code)
			}
			result.Error = err.Error()
		}
	} else {
		result.Status = HookResultFailed
		if result.Message == "" {
			result.Message = err.Error()
		}
		result.Error = err.Error()
	}
	// 尝试从 stdout JSON 提取 message/annotations（status 仍由 exit code 决定）
	if resp, parseErr := ParseCommandResponse(stdout); parseErr == nil {
		if trimmed := strings.TrimSpace(resp.Message); trimmed != "" {
			result.Message = trimmed
		}
		if len(resp.Annotations) > 0 {
			result.Metadata.Annotations = resp.Annotations
		}
	}
	// 失败时附带 stderr 便于调试
	if stderrText := strings.TrimSpace(string(stderr)); stderrText != "" && result.Message == "" {
		result.Message = stderrText
	}
	return result
}
