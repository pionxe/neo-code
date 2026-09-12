package state

import (
	"fmt"
	"time"

	"neo-code/internal/tuiv2/gateway"
)

// Reduce 将 GatewayEvent 就地映射到 ViewState：直接修改传入的状态并返回同一指针
// （状态指针全程稳定，见 docs/tui-v2/tui-v2-redesign-2026-09.md ADR-001）。
// nil 输入返回新建的空状态。单线程契约：仅在 Bubble Tea Update 循环内调用。
// 全量行为（含 Input 写入）仅供 app 层旧路径使用；kernel 插件路径走
// ReduceWithoutInput（Input 写权归 prompt，经 ApplyInputForEvent）。
func Reduce(current *ViewState, event gateway.GatewayEvent) *ViewState {
	if current == nil {
		current = NewViewState()
	}
	return reduceCore(current, event, true)
}

// reduceCore 是 Reduce 与 ReduceWithoutInput 的共享主体：
// applyInput=false 时跳过 Input 槽写入（六类事件经 ApplyInputForEvent 的部分），
// 其余槽迁移完全一致（审计 P2-a：逐槽一致性由表驱动测试钉死）。
func reduceCore(current *ViewState, event gateway.GatewayEvent, applyInput bool) *ViewState {
	switch event.Type {
	case gateway.EventAgentChunk, gateway.EventAssistantDelta:
		return reduceAgentChunk(current, event)
	case gateway.EventAgentMessageStart:
		return appendStream(current, streamEntry(event, "message", payloadString(event.Payload, "text", "content", "message")))
	case gateway.EventAgentMessageEnd:
		return reduceAgentMessageEnd(current, event)
	case gateway.EventToolStart, gateway.EventToolStarted:
		return reduceToolStart(current, event)
	case gateway.EventToolEnd, gateway.EventToolFinished:
		return reduceToolEnd(current, event)
	case gateway.EventToolOutput:
		return appendStream(current, streamEntry(event, "tool_output", payloadString(event.Payload, "text", "output", "content")))
	case gateway.EventPermissionRequested:
		return reducePermissionRequested(current, event, applyInput)
	case gateway.EventPermissionResolved:
		if applyInput {
			ApplyInputForEvent(current, event)
		}
		current.Runtime.Phase = RuntimePhaseRunning
		return appendStream(current, streamEntry(event, "status", payloadString(event.Payload, "message", "decision", "status")))
	case gateway.EventAskUserQuestion, gateway.EventUserQuestionRequested:
		return reduceAskUserQuestion(current, event, applyInput)
	case gateway.EventUserQuestionAnswered:
		if applyInput {
			ApplyInputForEvent(current, event)
		}
		current.Runtime.Phase = RuntimePhaseRunning
		return appendStream(current, streamEntry(event, "status", payloadString(event.Payload, "message", "answer", "text")))
	case gateway.EventPhaseChanged:
		current.Runtime.Phase = payloadString(event.Payload, "phase", "status")
	case gateway.EventRunStarted:
		current.Runtime.Phase = RuntimePhaseRunning
		current.Runtime.RunID = event.RunID
	case gateway.EventRunFinished:
		if current.Runtime.Phase != RuntimePhaseError && current.Runtime.Phase != RuntimePhaseCancelled {
			current.Runtime.Phase = RuntimePhaseIdle
		}
		current.Runtime.Tokens = tokenUsageFromPayload(event.Payload, current.Runtime.Tokens)
	case gateway.EventRunError, gateway.EventError:
		current.Runtime.Phase = RuntimePhaseError
		return appendStream(current, streamEntry(event, "error", payloadString(event.Payload, "message", "error", "text")))
	case gateway.EventRunCancelled:
		if applyInput {
			ApplyInputForEvent(current, event)
		}
		current.Runtime.Phase = RuntimePhaseCancelled
		return appendStream(current, streamEntry(event, "status", payloadString(event.Payload, "message", "phase", "status")))
	case gateway.EventTokenUsage:
		current.Runtime.Tokens = tokenUsageFromPayload(event.Payload, current.Runtime.Tokens)
	case gateway.EventSessionCreated:
		current.Gateway.Sessions = append(current.Gateway.Sessions, sessionFromPayload(event.Payload))
	case gateway.EventSessionDeleted:
		current.Gateway.Sessions = deleteSession(current.Gateway.Sessions, payloadString(event.Payload, "id", "session_id"))
	case gateway.EventSessionUpdated:
		current.Gateway.Sessions = upsertSession(current.Gateway.Sessions, sessionFromPayload(event.Payload))
	case gateway.EventModelChanged:
		current.Gateway.ActiveModel = payloadString(event.Payload, "model_id", "model", "id")
	case gateway.EventHealthChanged:
		current.Gateway.Connected = payloadBool(event.Payload, "connected", "ok")
	case gateway.EventGatewayOffline:
		current.Gateway.Connected = false
		current.Runtime.Phase = RuntimePhaseError
		return appendStream(current, streamEntry(event, "error", payloadString(event.Payload, "message", "error")))
	}
	return current
}

// reduceAgentChunk 合并助手流式文本：最后一条未完成消息被就地增量追加。
// 取末条目指针直改，取代旧实现的整段 slice 拷贝（copy-then-replace）；
// Metadata 为 nil 时惰性建表，保持 nil 输入契约与旧实现等价。
func reduceAgentChunk(current *ViewState, event gateway.GatewayEvent) *ViewState {
	text := payloadString(event.Payload, "text", "delta", "content")
	if len(current.Stream) > 0 {
		last := &current.Stream[len(current.Stream)-1]
		if last.Type == "message" && !streamEntryDone(*last) {
			last.Content += text
			if last.Metadata == nil {
				last.Metadata = map[string]any{}
			}
			last.Metadata["done"] = false
			if _, ok := last.Metadata["role"].(string); !ok {
				last.Metadata["role"] = "assistant"
			}
			return current
		}
	}
	entry := streamEntry(event, "message", text)
	entry.Metadata["done"] = false
	entry.Metadata["role"] = "assistant"
	return appendStream(current, entry)
}

// reduceAgentMessageEnd 标记最后一条消息完成（就地置位），并同步 token 用量。
func reduceAgentMessageEnd(current *ViewState, event gateway.GatewayEvent) *ViewState {
	if len(current.Stream) > 0 {
		last := &current.Stream[len(current.Stream)-1]
		if last.Type == "message" {
			if last.Metadata == nil {
				last.Metadata = map[string]any{}
			}
			last.Metadata["done"] = true
		}
	}
	current.Runtime.Tokens = tokenUsageFromPayload(event.Payload, current.Runtime.Tokens)
	return current
}

// reduceToolStart 追加工具开始条目，保留工具名和输入摘要。
func reduceToolStart(current *ViewState, event gateway.GatewayEvent) *ViewState {
	entry := streamEntry(event, "tool_start", payloadString(event.Payload, "command", "input", "text"))
	entry.ToolName = payloadString(event.Payload, "tool", "tool_name", "name")
	entry.ToolInput = payloadString(event.Payload, "input", "command")
	return appendStream(current, entry)
}

// reduceToolEnd 追加工具结束条目，记录输出或状态摘要。
func reduceToolEnd(current *ViewState, event gateway.GatewayEvent) *ViewState {
	entry := streamEntry(event, "tool_end", payloadString(event.Payload, "output", "content", "status", "text"))
	entry.ToolName = payloadString(event.Payload, "tool", "tool_name", "name")
	return appendStream(current, entry)
}

// reducePermissionRequested 进入权限等待态，并追加权限状态条目。
// applyInput=false（chat 路径）跳过 Input 写入；条目 content 从 payload
// 直取（审计 P2-a：不得读 current.Input.Prompt——先写后读依赖在
// ReduceWithoutInput 拆分后失效）。
func reducePermissionRequested(current *ViewState, event gateway.GatewayEvent, applyInput bool) *ViewState {
	current.Runtime.Phase = RuntimePhaseWaitingPermission
	prompt := payloadString(event.Payload, "prompt", "message", "tool")
	if applyInput {
		ApplyInputForEvent(current, event)
	}
	return appendStream(current, streamEntry(event, "permission", prompt))
}

// reduceAskUserQuestion 进入用户问答态，并更新输入区提示和选项。
// applyInput 语义同上；条目 content 从 payload 直取。
func reduceAskUserQuestion(current *ViewState, event gateway.GatewayEvent, applyInput bool) *ViewState {
	current.Runtime.Phase = RuntimePhaseWaitingUser
	question := payloadString(event.Payload, "question", "prompt", "message")
	if applyInput {
		ApplyInputForEvent(current, event)
	}
	return appendStream(current, streamEntry(event, "question", question))
}

// appendStream 在流尾部追加条目：就地修改当前状态（指针语义见 Reduce）。
func appendStream(current *ViewState, entry StreamEntry) *ViewState {
	current.Stream = append(current.Stream, entry)
	return current
}

// streamEntry 将 Gateway 事件转换为 StreamEntry 的通用构造。
func streamEntry(event gateway.GatewayEvent, entryType string, content string) StreamEntry {
	metadata := clonePayload(event.Payload)
	if entryType == "message" {
		if _, ok := metadata["role"].(string); !ok {
			metadata["role"] = "assistant"
		}
	}
	return StreamEntry{
		ID:        payloadString(event.Payload, "id", "entry_id", "message_id"),
		Type:      entryType,
		Timestamp: eventTime(event.At),
		Content:   content,
		Metadata:  metadata,
	}
}

// streamEntryDone 判断消息条目是否已完成，完成后不再合并 chunk。
func streamEntryDone(entry StreamEntry) bool {
	done, ok := entry.Metadata["done"].(bool)
	return ok && done
}

// tokenUsageFromPayload 从事件 payload 中提取 token 用量，缺省字段沿用旧值。
func tokenUsageFromPayload(payload map[string]any, fallback TokenUsage) TokenUsage {
	return TokenUsage{
		Input:  payloadInt(payload, fallback.Input, "input", "input_tokens"),
		Output: payloadInt(payload, fallback.Output, "output", "output_tokens"),
		Total:  payloadInt(payload, fallback.Total, "total", "total_tokens", "tokens"),
	}
}

// sessionFromPayload 从事件 payload 中提取会话摘要 DTO。
func sessionFromPayload(payload map[string]any) gateway.SessionSummary {
	return gateway.SessionSummary{
		ID:    payloadString(payload, "id", "session_id"),
		Title: payloadString(payload, "title", "name"),
		Mode:  payloadString(payload, "mode"),
		Model: payloadString(payload, "model", "model_id"),
	}
}

// deleteSession 返回移除指定会话后的新会话列表。
func deleteSession(sessions []gateway.SessionSummary, id string) []gateway.SessionSummary {
	next := make([]gateway.SessionSummary, 0, len(sessions))
	for _, session := range sessions {
		if session.ID != id {
			next = append(next, session)
		}
	}
	return next
}

// upsertSession 返回更新或追加会话摘要后的新会话列表。
func upsertSession(sessions []gateway.SessionSummary, updated gateway.SessionSummary) []gateway.SessionSummary {
	if updated.ID == "" {
		return append([]gateway.SessionSummary(nil), sessions...)
	}
	next := append([]gateway.SessionSummary(nil), sessions...)
	for index, session := range next {
		if session.ID == updated.ID {
			next[index] = updated
			return next
		}
	}
	return append(next, updated)
}

// payloadString 按候选键从 payload 中读取字符串化值。
func payloadString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			return typed
		case fmt.Stringer:
			return typed.String()
		case nil:
			continue
		default:
			return fmt.Sprint(typed)
		}
	}
	return ""
}

// payloadInt 按候选键从 payload 中读取整数值。
func payloadInt(payload map[string]any, fallback int, keys ...string) int {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case int:
			return typed
		case int64:
			return int(typed)
		case float64:
			return int(typed)
		}
	}
	return fallback
}

// payloadBool 按候选键从 payload 中读取布尔值。
func payloadBool(payload map[string]any, keys ...string) bool {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		if typed, ok := value.(bool); ok {
			return typed
		}
	}
	return false
}

// payloadStringSlice 从 payload 中读取字符串切片。
func payloadStringSlice(payload map[string]any, key string) []string {
	value, ok := payload[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, fmt.Sprint(item))
		}
		return out
	default:
		return nil
	}
}

// clonePayload 复制事件 payload，避免 StreamEntry 持有共享 map。
func clonePayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return map[string]any{}
	}
	clone := make(map[string]any, len(payload)+1)
	for key, value := range payload {
		clone[key] = value
	}
	return clone
}

// eventTime 为缺少时间戳的事件补充当前时间。
func eventTime(at time.Time) time.Time {
	if at.IsZero() {
		return time.Now()
	}
	return at
}
