package runtime

import (
	"context"
	"encoding/json"
	"strings"

	runtimehooks "neo-code/internal/runtime/hooks"
	providertypes "neo-code/internal/provider/types"
)

const (
	// hookErrorClassBlocked 标识由 before_tool_call hook 拦截产生的工具错误分类。
	hookErrorClassBlocked = "hook_blocked"
)

type hookContextKey string

const hookRuntimeEnvelopeKey hookContextKey = "runtime_hook_envelope"

type hookRuntimeEnvelope struct {
	RunID     string
	SessionID string
	Turn      int
	Phase     string
}

// HookExecutor 定义 runtime 调用 hook 的最小执行契约。
type HookExecutor interface {
	Run(ctx context.Context, point runtimehooks.HookPoint, input runtimehooks.HookContext) runtimehooks.RunOutput
}

type hookRuntimeEventEmitter struct {
	service *Service
}

func newHookRuntimeEventEmitter(service *Service) *hookRuntimeEventEmitter {
	return &hookRuntimeEventEmitter{service: service}
}

// EmitHookEvent 将 hooks 包内事件桥接为 runtime 事件，供 TUI 与日志统一消费。
func (e *hookRuntimeEventEmitter) EmitHookEvent(ctx context.Context, event runtimehooks.HookEvent) error {
	if e == nil || e.service == nil {
		return nil
	}
	envelope, _ := runtimeHookEnvelopeFromContext(ctx)
	kind := EventType(strings.TrimSpace(string(event.Type)))
	if kind == "" {
		return nil
	}
	payload := any(HookEventPayload{
		HookID:     event.HookID,
		Point:      string(event.Point),
		Scope:      string(event.Scope),
		Source:     string(event.Source),
		Kind:       string(event.Kind),
		Mode:       string(event.Mode),
		Status:     string(event.Status),
		Message:    strings.TrimSpace(event.Message),
		StartedAt:  event.StartedAt,
		DurationMS: event.DurationMS,
		Error:      event.Error,
	})
	if event.Type == runtimehooks.HookEventNotification {
		payload = HookNotificationPayload{
			HookID:       event.HookID,
			Source:       string(event.Source),
			Point:        string(event.Point),
			Status:       string(event.Status),
			Reason:       strings.TrimSpace(event.RewakeReason),
			Summary:      strings.TrimSpace(event.RewakeSummary),
			Message:      strings.TrimSpace(event.Message),
			DedupeKey:    strings.TrimSpace(event.DedupeKey),
			Notification: strings.TrimSpace(event.RewakeSummary),
		}
	}
	return e.service.emitWithEnvelope(ctx, RuntimeEvent{
		Type:           kind,
		RunID:          envelope.RunID,
		SessionID:      envelope.SessionID,
		Turn:           envelope.Turn,
		Phase:          envelope.Phase,
		PayloadVersion: 0,
		Payload:        payload,
	})
}

// runHookPoint 在指定运行态上下文执行一个 hook 点，并自动注入 run/session 元数据。
func (s *Service) runHookPoint(
	ctx context.Context,
	state *runState,
	point runtimehooks.HookPoint,
	input runtimehooks.HookContext,
) runtimehooks.RunOutput {
	if s == nil || s.hookExecutor == nil {
		return runtimehooks.RunOutput{}
	}
	runID := firstNonBlank(hookRunIDFromState(state), input.RunID)
	sessionID := firstNonBlank(hookSessionIDFromState(state), input.SessionID)
	input.RunID = firstNonBlank(input.RunID, runID)
	input.SessionID = firstNonBlank(input.SessionID, sessionID)
	if input.Metadata == nil {
		input.Metadata = make(map[string]any, 8)
	}
	input.Metadata["run_id"] = input.RunID
	input.Metadata["session_id"] = input.SessionID
	if state != nil {
		input.Metadata["runtime_run_token"] = state.runToken
		if _, exists := input.Metadata["phase"]; !exists {
			input.Metadata["phase"] = hookPhaseFromState(state)
		}
		input.Metadata["turn"] = hookTurnFromState(state)
	}
	scopedCtx := withRuntimeHookEnvelope(ctx, hookRuntimeEnvelope{
		RunID:     runID,
		SessionID: sessionID,
		Turn:      hookTurnFromState(state),
		Phase:     hookPhaseFromState(state),
	})
	output := s.hookExecutor.Run(scopedCtx, point, input)
	s.recordUserHookAnnotations(state, output)
	return output
}

func withRuntimeHookEnvelope(ctx context.Context, envelope hookRuntimeEnvelope) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, hookRuntimeEnvelopeKey, envelope)
}

func runtimeHookEnvelopeFromContext(ctx context.Context) (hookRuntimeEnvelope, bool) {
	if ctx == nil {
		return hookRuntimeEnvelope{}, false
	}
	raw := ctx.Value(hookRuntimeEnvelopeKey)
	envelope, ok := raw.(hookRuntimeEnvelope)
	return envelope, ok
}

func hookRunIDFromState(state *runState) string {
	if state == nil {
		return ""
	}
	return strings.TrimSpace(state.runID)
}

func hookSessionIDFromState(state *runState) string {
	if state == nil {
		return ""
	}
	return strings.TrimSpace(state.session.ID)
}

func hookTurnFromState(state *runState) int {
	if state == nil {
		return turnUnspecified
	}
	return state.turn
}

func hookPhaseFromState(state *runState) string {
	if state == nil {
		return ""
	}
	if state.lifecycle == "" {
		return ""
	}
	return string(state.lifecycle)
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func findHookBlockMessage(output runtimehooks.RunOutput) string {
	if !output.Blocked {
		return ""
	}
	for _, result := range output.Results {
		if !strings.EqualFold(strings.TrimSpace(result.HookID), strings.TrimSpace(output.BlockedBy)) {
			continue
		}
		message := strings.TrimSpace(result.Message)
		if message != "" {
			return message
		}
		errText := strings.TrimSpace(result.Error)
		if errText != "" {
			return errText
		}
		break
	}
	if blockedBy := strings.TrimSpace(output.BlockedBy); blockedBy != "" {
		return "hook blocked by " + blockedBy
	}
	return "hook blocked"
}

// findHookBlockSource 返回本次阻断命中的来源标签，优先从阻断结果回推，其次回退输出字段。
func findHookBlockSource(output runtimehooks.RunOutput) runtimehooks.HookSource {
	if !output.Blocked {
		return ""
	}
	for _, result := range output.Results {
		if !strings.EqualFold(strings.TrimSpace(result.HookID), strings.TrimSpace(output.BlockedBy)) {
			continue
		}
		if result.Source != "" {
			return result.Source
		}
		break
	}
	return output.BlockedSource
}

// recordUserHookAnnotations 将 user/repo hook 产生的消息缓存到运行态注释缓冲区，供后续观测链路消费。
func (s *Service) recordUserHookAnnotations(state *runState, output runtimehooks.RunOutput) {
	if state == nil || len(output.Results) == 0 {
		return
	}
	notes := make([]string, 0, len(output.Results))
	for _, result := range output.Results {
		if result.Scope != runtimehooks.HookScopeUser && result.Scope != runtimehooks.HookScopeRepo {
			continue
		}
		message := strings.TrimSpace(result.Message)
		if message != "" {
			notes = append(notes, message)
		}
		for _, annotation := range result.Metadata.Annotations {
			trimmed := strings.TrimSpace(annotation)
			if trimmed != "" {
				notes = append(notes, trimmed)
			}
		}
	}
	if len(notes) == 0 {
		return
	}
	state.mu.Lock()
	state.hookAnnotations = append(state.hookAnnotations, notes...)
	state.mu.Unlock()
}

// applyCommandHookUpdateInput 检查 hook 输出中的 update_input 并应用到用户输入 parts。
// 当前仅支持 user_prompt_submit 点位；update_input 格式: {"text": "..."}  替换文本内容。
func applyCommandHookUpdateInput(output runtimehooks.RunOutput, parts []providertypes.ContentPart) []providertypes.ContentPart {
	if len(output.Results) == 0 {
		return parts
	}
	for _, result := range output.Results {
		if len(result.Metadata.UpdateInput) == 0 {
			continue
		}
		cap, ok := runtimehooks.HookPointCapabilities(result.Point)
		if !ok || !cap.CanUpdateInput {
			continue
		}
		var update struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(result.Metadata.UpdateInput, &update); err != nil {
			continue
		}
		if update.Text == "" {
			continue
		}
		replaced := false
		newParts := make([]providertypes.ContentPart, 0, len(parts))
		for _, part := range parts {
			if !replaced && part.Kind == providertypes.ContentPartText {
				newParts = append(newParts, providertypes.NewTextPart(update.Text))
				replaced = true
			} else {
				newParts = append(newParts, part)
			}
		}
		return newParts
	}
	return parts
}
