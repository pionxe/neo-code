package runtime

import (
	"strings"

	providertypes "neo-code/internal/provider/types"
	"neo-code/internal/tools"
)

// triggerMemoExtraction 在 Run 结束后异步触发记忆提取，避免阻塞主闭环。
func (s *Service) triggerMemoExtraction(sessionID string, messages []providertypes.Message, skip bool) {
	if s == nil || s.memoExtractor == nil || len(messages) == 0 {
		return
	}
	if skip {
		return
	}

	s.memoExtractor.Schedule(sessionID, cloneMessages(messages))
}

// isSuccessfulRememberToolCall 判断工具调用是否成功完成显式记忆写入。
func isSuccessfulRememberToolCall(callName string, result tools.ToolResult, execErr error) bool {
	if execErr != nil || result.IsError {
		return false
	}
	return strings.TrimSpace(callName) == tools.ToolNameMemoRemember
}
