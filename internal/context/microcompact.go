package context

import (
	"strings"

	"neo-code/internal/context/internalcompact"
	"neo-code/internal/provider"
)

const (
	// microCompactClearedMessage 是旧工具结果被读时微压缩后的占位符文本。
	microCompactClearedMessage = "[Old tool result content cleared]"
	// microCompactRetainedToolSpans 定义默认保留原始内容的最近可压缩工具块数量。
	microCompactRetainedToolSpans = 2
)

var microCompactableTools = map[string]struct{}{
	"bash":                  {},
	"webfetch":              {},
	"filesystem_read_file":  {},
	"filesystem_grep":       {},
	"filesystem_glob":       {},
	"filesystem_edit":       {},
	"filesystem_write_file": {},
}

// microCompactMessages 对裁剪后的消息做只读投影式微压缩，仅清理旧工具结果内容。
func microCompactMessages(messages []provider.Message) []provider.Message {
	cloned := cloneContextMessages(messages)
	if len(cloned) == 0 {
		return cloned
	}

	spans := internalcompact.BuildMessageSpans(cloned)
	protectedStart, hasProtectedTail := internalcompact.ProtectedTailStart(spans)
	retainedCompactableSpans := 0

	for spanIndex := len(spans) - 1; spanIndex >= 0; spanIndex-- {
		span := spans[spanIndex]
		if hasProtectedTail && span.Start >= protectedStart {
			continue
		}
		if !isToolCallSpan(cloned, span) {
			continue
		}

		compactableIDs := compactableToolCallIDs(cloned[span.Start].ToolCalls)
		if len(compactableIDs) == 0 {
			continue
		}
		if !hasCompactableToolContent(cloned, span, compactableIDs) {
			continue
		}
		if retainedCompactableSpans < microCompactRetainedToolSpans {
			retainedCompactableSpans++
			continue
		}

		for messageIndex := span.Start + 1; messageIndex < span.End; messageIndex++ {
			if shouldClearToolMessage(cloned[messageIndex], compactableIDs) {
				cloned[messageIndex].Content = microCompactClearedMessage
			}
		}
	}

	return cloned
}

// cloneContextMessages 深拷贝消息切片，避免读时投影污染 runtime 持有的原始会话消息。
func cloneContextMessages(messages []provider.Message) []provider.Message {
	if len(messages) == 0 {
		return nil
	}

	cloned := make([]provider.Message, 0, len(messages))
	for _, message := range messages {
		next := message
		next.ToolCalls = append([]provider.ToolCall(nil), message.ToolCalls...)
		cloned = append(cloned, next)
	}
	return cloned
}

// isToolCallSpan 判断当前 span 是否是由 assistant tool call 起始的原子工具块。
func isToolCallSpan(messages []provider.Message, span internalcompact.MessageSpan) bool {
	if span.Start < 0 || span.Start >= len(messages) {
		return false
	}
	message := messages[span.Start]
	return message.Role == provider.RoleAssistant && len(message.ToolCalls) > 0
}

// compactableToolCallIDs 返回 assistant tool call 中可参与微压缩的调用 ID 集合。
func compactableToolCallIDs(calls []provider.ToolCall) map[string]struct{} {
	if len(calls) == 0 {
		return nil
	}

	ids := make(map[string]struct{}, len(calls))
	for _, call := range calls {
		toolName := strings.TrimSpace(call.Name)
		if _, ok := microCompactableTools[toolName]; !ok {
			continue
		}
		callID := strings.TrimSpace(call.ID)
		if callID == "" {
			continue
		}
		ids[callID] = struct{}{}
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// hasCompactableToolContent 判断工具块中是否存在会影响保留预算的有效工具结果内容。
func hasCompactableToolContent(messages []provider.Message, span internalcompact.MessageSpan, compactableIDs map[string]struct{}) bool {
	for messageIndex := span.Start + 1; messageIndex < span.End; messageIndex++ {
		if shouldClearToolMessage(messages[messageIndex], compactableIDs) {
			return true
		}
	}
	return false
}

// shouldClearToolMessage 判断一条 tool 消息是否满足旧结果清理条件。
func shouldClearToolMessage(message provider.Message, compactableIDs map[string]struct{}) bool {
	if message.Role != provider.RoleTool || message.IsError {
		return false
	}
	if compactableIDs == nil {
		return false
	}
	if _, ok := compactableIDs[strings.TrimSpace(message.ToolCallID)]; !ok {
		return false
	}

	content := strings.TrimSpace(message.Content)
	return content != "" && content != microCompactClearedMessage
}
