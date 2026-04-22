package spawnsubagent

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"neo-code/internal/subagent"
	"neo-code/internal/tools"
)

const (
	maxSpawnArgumentsBytes = 64 * 1024
	maxSpawnTextLen        = 1024
	maxSpawnListItems      = 64

	spawnModeInline = "inline"
)

type spawnInput struct {
	Mode           string   `json:"mode"`
	Role           string   `json:"role"`
	ID             string   `json:"id"`
	Prompt         string   `json:"prompt"`
	Content        string   `json:"content"`
	ExpectedOutput string   `json:"expected_output"`
	MaxSteps       int      `json:"max_steps"`
	TimeoutSec     int      `json:"timeout_sec"`
	AllowedTools   []string `json:"allowed_tools"`
	AllowedPaths   []string `json:"allowed_paths"`
}

// Tool 定义 spawn_subagent 工具：仅支持 inline 即时执行模式。
type Tool struct{}

// New 返回 spawn_subagent 工具实例。
func New() *Tool {
	return &Tool{}
}

// Name 返回工具唯一名称。
func (t *Tool) Name() string {
	return tools.ToolNameSpawnSubAgent
}

// Description 返回工具描述。
func (t *Tool) Description() string {
	return "Run subagent immediately in inline mode."
}

// Schema 返回 spawn_subagent 的参数定义，仅保留 inline 模式参数。
func (t *Tool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"mode": map[string]any{
				"type": "string",
				"enum": []string{spawnModeInline},
			},
			"role": map[string]any{
				"type": "string",
				"enum": []string{"researcher", "coder", "reviewer"},
			},
			"id": map[string]any{
				"type": "string",
			},
			"prompt": map[string]any{
				"type": "string",
			},
			"expected_output": map[string]any{
				"type": "string",
			},
			"max_steps": map[string]any{
				"type": "integer",
			},
			"timeout_sec": map[string]any{
				"type": "integer",
			},
			"allowed_tools": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
			},
			"allowed_paths": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
			},
		},
	}
}

// MicroCompactPolicy 声明 spawn_subagent 结果默认参与 micro compact。
func (t *Tool) MicroCompactPolicy() tools.MicroCompactPolicy {
	return tools.MicroCompactPolicyCompact
}

// Execute 解析入参后执行 inline 模式。
func (t *Tool) Execute(ctx context.Context, call tools.ToolCallInput) (tools.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return tools.NewErrorResult(t.Name(), tools.NormalizeErrorReason(t.Name(), err), "", nil), err
	}

	input, err := parseSpawnInput(call.Arguments)
	if err != nil {
		result := tools.NewErrorResult(t.Name(), tools.NormalizeErrorReason(t.Name(), err), err.Error(), nil)
		result = tools.ApplyOutputLimit(result, tools.DefaultOutputLimitBytes)
		return result, err
	}

	return t.executeInlineMode(ctx, call, input)
}

// executeInlineMode 调用 runtime 注入的 SubAgentInvoker，在主循环内即时执行子代理并回灌结果。
func (t *Tool) executeInlineMode(
	ctx context.Context,
	call tools.ToolCallInput,
	input spawnInput,
) (tools.ToolResult, error) {
	if call.SubAgentInvoker == nil {
		err := errors.New("spawn_subagent: subagent invoker is unavailable")
		result := tools.NewErrorResult(t.Name(), tools.NormalizeErrorReason(t.Name(), err), "", nil)
		result = tools.ApplyOutputLimit(result, tools.DefaultOutputLimitBytes)
		return result, err
	}

	role := subagent.Role(input.Role)
	if !role.Valid() {
		role = subagent.RoleCoder
	}
	taskID := strings.TrimSpace(input.ID)
	if taskID == "" {
		taskID = defaultInlineTaskID(input.Prompt)
	}

	runResult, runErr := call.SubAgentInvoker.Run(ctx, tools.SubAgentRunInput{
		CallerAgent:           strings.TrimSpace(call.AgentID),
		ParentCapabilityToken: call.CapabilityToken,
		Role:                  role,
		TaskID:                taskID,
		Goal:                  strings.TrimSpace(input.Prompt),
		ExpectedOut:           strings.TrimSpace(input.ExpectedOutput),
		Workdir:               strings.TrimSpace(call.Workdir),
		MaxSteps:              input.MaxSteps,
		Timeout:               time.Duration(input.TimeoutSec) * time.Second,
		AllowedTools:          append([]string(nil), input.AllowedTools...),
		AllowedPaths:          append([]string(nil), input.AllowedPaths...),
	})

	isError := runErr != nil || runResult.State == subagent.StateFailed || runResult.State == subagent.StateCanceled
	result := tools.ToolResult{
		Name:    t.Name(),
		Content: renderInlineSpawnResult(runResult, runErr),
		IsError: isError,
		Metadata: map[string]any{
			"mode":         spawnModeInline,
			"task_id":      runResult.TaskID,
			"role":         string(runResult.Role),
			"state":        string(runResult.State),
			"stop_reason":  string(runResult.StopReason),
			"step_count":   runResult.StepCount,
			"error":        strings.TrimSpace(runResult.Error),
			"artifact_cnt": len(runResult.Output.Artifacts),
		},
	}
	result = tools.ApplyOutputLimit(result, tools.DefaultOutputLimitBytes)
	return result, runErr
}

// parseSpawnInput 负责解析并校验 spawn_subagent 输入。
func parseSpawnInput(raw []byte) (spawnInput, error) {
	if len(raw) == 0 {
		return spawnInput{}, errors.New("spawn_subagent: arguments is empty")
	}
	if len(raw) > maxSpawnArgumentsBytes {
		return spawnInput{}, fmt.Errorf(
			"spawn_subagent: arguments payload exceeds %d bytes",
			maxSpawnArgumentsBytes,
		)
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return spawnInput{}, fmt.Errorf("spawn_subagent: parse arguments: %w", err)
	}
	if _, ok := root["items"]; ok {
		return spawnInput{}, errors.New("spawn_subagent: items is not supported; only inline mode is available")
	}

	var input spawnInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return spawnInput{}, fmt.Errorf("spawn_subagent: parse arguments: %w", err)
	}
	input.Mode = strings.ToLower(strings.TrimSpace(input.Mode))
	if input.Mode == "" {
		input.Mode = spawnModeInline
	}
	if input.Mode != spawnModeInline {
		return spawnInput{}, fmt.Errorf("spawn_subagent: unsupported mode %q", input.Mode)
	}

	input.ID = strings.TrimSpace(input.ID)
	input.Prompt = strings.TrimSpace(input.Prompt)
	input.Content = strings.TrimSpace(input.Content)
	if input.Prompt == "" {
		input.Prompt = input.Content
	}
	input.ExpectedOutput = strings.TrimSpace(input.ExpectedOutput)
	input.AllowedTools = normalizeStringList(input.AllowedTools)
	input.AllowedPaths = normalizeStringList(input.AllowedPaths)
	input.Role = strings.ToLower(strings.TrimSpace(input.Role))
	if input.Role != "" {
		role := subagent.Role(input.Role)
		if !role.Valid() {
			return spawnInput{}, fmt.Errorf("spawn_subagent: unsupported role %q", input.Role)
		}
	}

	return validateInlineInput(input)
}

// validateInlineInput 校验即时执行模式入参。
func validateInlineInput(input spawnInput) (spawnInput, error) {
	if strings.TrimSpace(input.Prompt) == "" {
		return spawnInput{}, errors.New("spawn_subagent: prompt is empty")
	}
	if len(input.Prompt) > maxSpawnTextLen {
		return spawnInput{}, fmt.Errorf("spawn_subagent: prompt exceeds max length %d", maxSpawnTextLen)
	}
	if len(input.ID) > maxSpawnTextLen {
		return spawnInput{}, fmt.Errorf("spawn_subagent: id exceeds max length %d", maxSpawnTextLen)
	}
	if len(input.ExpectedOutput) > maxSpawnTextLen {
		return spawnInput{}, fmt.Errorf("spawn_subagent: expected_output exceeds max length %d", maxSpawnTextLen)
	}
	if len(input.AllowedTools) > maxSpawnListItems {
		return spawnInput{}, fmt.Errorf("spawn_subagent: allowed_tools exceeds max items %d", maxSpawnListItems)
	}
	if len(input.AllowedPaths) > maxSpawnListItems {
		return spawnInput{}, fmt.Errorf("spawn_subagent: allowed_paths exceeds max items %d", maxSpawnListItems)
	}
	if input.MaxSteps < 0 {
		return spawnInput{}, errors.New("spawn_subagent: max_steps must be >= 0")
	}
	if input.TimeoutSec < 0 {
		return spawnInput{}, errors.New("spawn_subagent: timeout_sec must be >= 0")
	}
	return input, nil
}

// normalizeStringList 统一清理字符串列表并去重，保持输入顺序稳定。
func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// defaultInlineTaskID 为 inline 模式生成稳定 task id，避免空 id 导致审计不可读。
func defaultInlineTaskID(prompt string) string {
	trimmed := strings.TrimSpace(prompt)
	if trimmed == "" {
		return "spawn-subagent-inline"
	}
	sum := sha1.Sum([]byte(trimmed))
	return "spawn-inline-" + hex.EncodeToString(sum[:4])
}

// renderInlineSpawnResult 输出 inline 模式的即时执行结果。
func renderInlineSpawnResult(result tools.SubAgentRunResult, runErr error) string {
	lines := []string{
		"spawn_subagent result",
		fmt.Sprintf("mode: %s", spawnModeInline),
		"task_id: " + strings.TrimSpace(result.TaskID),
		"role: " + strings.TrimSpace(string(result.Role)),
		"state: " + strings.TrimSpace(string(result.State)),
		"stop_reason: " + strings.TrimSpace(string(result.StopReason)),
		fmt.Sprintf("step_count: %d", result.StepCount),
	}
	if text := strings.TrimSpace(result.Output.Summary); text != "" {
		lines = append(lines, "summary: "+text)
	}
	if len(result.Output.Findings) > 0 {
		lines = append(lines, "findings:")
		for _, finding := range result.Output.Findings {
			lines = append(lines, "- "+finding)
		}
	}
	if len(result.Output.Artifacts) > 0 {
		lines = append(lines, "artifacts:")
		for _, artifact := range result.Output.Artifacts {
			lines = append(lines, "- "+artifact)
		}
	}
	errText := strings.TrimSpace(result.Error)
	if errText == "" && runErr != nil {
		errText = strings.TrimSpace(runErr.Error())
	}
	if errText != "" {
		lines = append(lines, "error: "+errText)
	}
	return strings.Join(lines, "\n")
}
