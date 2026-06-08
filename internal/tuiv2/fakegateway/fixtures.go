package fakegateway

import (
	"fmt"
	"time"

	"neo-code/internal/tuiv2/gateway"
)

const (
	defaultSessionID = "session-ghost-console"
	defaultRunID     = "run-ghost-001"
	defaultModelID   = "neo-fake-pro"
	tick             = 10 * time.Millisecond
)

var fixtureTime = time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)

// defaultSessionSummary 返回 Ghost Console 演示场景的默认会话摘要。
func defaultSessionSummary() gateway.SessionSummary {
	return gateway.SessionSummary{
		ID:        defaultSessionID,
		Title:     "Ghost Console demo",
		Mode:      "input",
		Model:     defaultModelID,
		UpdatedAt: fixtureTime,
	}
}

// defaultSessionDetail 返回包含基础对话历史的默认会话详情。
func defaultSessionDetail() gateway.SessionDetail {
	return detailWithStream([]gateway.StreamItem{
		userItem("msg-user-1", "Build a Ghost Console TUI that does not touch v1."),
		assistantItem("msg-agent-1", "Plan accepted. I will use the Gateway client contract and pure-function reducers for all state."),
		userItem("msg-user-2", "What components make up the Focus-Only layout?"),
		assistantItem("msg-agent-2", "The layout has three tiers: Ambient Status (top), Agent Stream (center), and Command Prompt (bottom). A Soft Inspector appears on the right when the terminal is wide enough."),
	})
}

// detailWithStream 使用给定流历史构造默认会话详情。
func detailWithStream(stream []gateway.StreamItem) gateway.SessionDetail {
	return gateway.SessionDetail{
		Summary: defaultSessionSummary(),
		Stream:  append([]gateway.StreamItem(nil), stream...),
		Usage:   gateway.TokenUsage{Input: 128, Output: 256, Total: 384},
	}
}

// defaultModels 返回 Fake Gateway 暴露给 TUI 的模型列表。
func defaultModels() []gateway.ModelInfo {
	return []gateway.ModelInfo{
		{ID: defaultModelID, Name: "Neo Fake Pro", Provider: "fake", Current: true, Capabilities: []string{"tools", "streaming"}},
		{ID: "neo-fake-fast", Name: "Neo Fake Fast", Provider: "fake", Current: false, Capabilities: []string{"streaming"}},
	}
}

// defaultEvents 返回默认场景的完整演示事件序列。
func defaultEvents() []scheduledEvent {
	return []scheduledEvent{
		{after: tick / 2, event: event(gateway.EventRunStarted, defaultSessionID, defaultRunID, payload("phase", "running"))},
		{after: tick, event: event(gateway.EventAgentMessageStart, defaultSessionID, defaultRunID, payload("message", "msg-1", "text", "Ghost Console ready."))},
		{after: tick, event: event(gateway.EventAgentChunk, defaultSessionID, defaultRunID, payload("text", "I have loaded the Ghost Console demo."))},
		{after: tick, event: event(gateway.EventAgentChunk, defaultSessionID, defaultRunID, payload("text", " The TUI v2 architecture uses pure-function reducers and unidirectional data flow."))},
		{after: tick, event: event(gateway.EventAgentMessageEnd, defaultSessionID, defaultRunID, payload("message_id", "msg-1"))},
		{after: tick, event: event(gateway.EventToolStarted, defaultSessionID, defaultRunID, payload("tool", "filesystem.read", "input", "internal/tuiv2/state/reducer.go"))},
		{after: tick, event: event(gateway.EventToolFinished, defaultSessionID, defaultRunID, payload("tool", "filesystem.read", "output", "Reducer handles 22 event types with pure-function mapping", "status", "ok"))},
		{after: tick, event: event(gateway.EventToolStarted, defaultSessionID, defaultRunID, payload("tool", "filesystem.grep", "input", "tea.Model impl"))},
		{after: tick, event: event(gateway.EventAgentChunk, defaultSessionID, defaultRunID, payload("text", "The layout follows Focus-Only design with 3-tier responsive breakpoints."))},
		{after: tick, event: event(gateway.EventAgentMessageEnd, defaultSessionID, defaultRunID, payload("message_id", "msg-2"))},
		{after: tick, event: event(gateway.EventToolFinished, defaultSessionID, defaultRunID, payload("tool", "filesystem.grep", "output", "Found AmbientStatus, AgentStream, CommandPrompt, SoftInspector", "status", "ok"))},
		{after: tick, event: event(gateway.EventTokenUsage, defaultSessionID, defaultRunID, payload("total", 384, "input", 100, "output", 284))},
		{after: tick, event: event(gateway.EventRunFinished, defaultSessionID, defaultRunID, payload("phase", "done"))},
	}
}

// streamingEvents 返回逐块输出的助手流式事件。
func streamingEvents() []scheduledEvent {
	return []scheduledEvent{
		{after: tick, event: event(gateway.EventRunStarted, defaultSessionID, defaultRunID, payload("phase", "streaming"))},
		{after: tick, event: event(gateway.EventAgentMessageStart, defaultSessionID, defaultRunID, payload("message", "stream-msg-1", "text", "Let me explain the Ghost Console architecture."))},
		{after: tick, event: event(gateway.EventAgentChunk, defaultSessionID, defaultRunID, payload("text", "The Ghost Console"))},
		{after: tick, event: event(gateway.EventAgentChunk, defaultSessionID, defaultRunID, payload("text", " is a terminal-native "))},
		{after: tick, event: event(gateway.EventAgentChunk, defaultSessionID, defaultRunID, payload("text", "design language for NeoCode. "))},
		{after: tick, event: event(gateway.EventAgentChunk, defaultSessionID, defaultRunID, payload("text", "It emphasizes whitespace, indentation, "))},
		{after: tick, event: event(gateway.EventAgentChunk, defaultSessionID, defaultRunID, payload("text", "and semantic symbols over heavy borders."))},
		{after: tick, event: event(gateway.EventAgentMessageEnd, defaultSessionID, defaultRunID, payload("message_id", "stream-msg-1"))},
		{after: tick, event: event(gateway.EventToolStarted, defaultSessionID, defaultRunID, payload("tool", "filesystem.grep", "input", "search NeoCode patterns"))},
		{after: tick, event: event(gateway.EventToolFinished, defaultSessionID, defaultRunID, payload("tool", "filesystem.grep", "output", "found 15 matches in 8 files", "status", "ok"))},
		{after: tick, event: event(gateway.EventToolStarted, defaultSessionID, defaultRunID, payload("tool", "filesystem.read", "input", "internal/tuiv2/app.go"))},
		{after: tick, event: event(gateway.EventAgentChunk, defaultSessionID, defaultRunID, payload("text", "Looking at the app structure, "))},
		{after: tick, event: event(gateway.EventAgentChunk, defaultSessionID, defaultRunID, payload("text", "I can see the Focus-Only layout is already implemented."))},
		{after: tick, event: event(gateway.EventAgentMessageEnd, defaultSessionID, defaultRunID, payload("message_id", "stream-msg-2"))},
		{after: tick, event: event(gateway.EventToolFinished, defaultSessionID, defaultRunID, payload("tool", "filesystem.read", "output", "530 lines, layout uses 3-tier responsive design", "status", "ok"))},
		{after: tick, event: event(gateway.EventTokenUsage, defaultSessionID, defaultRunID, payload("total", 580, "input", 120, "output", 460))},
		{after: tick, event: event(gateway.EventRunFinished, defaultSessionID, defaultRunID, payload("phase", "done"))},
	}
}

// toolApprovalEvents 返回工具权限等待流程的事件序列。
func toolApprovalEvents() []scheduledEvent {
	return []scheduledEvent{
		{after: tick, event: event(gateway.EventRunStarted, defaultSessionID, defaultRunID, payload("phase", "running"))},
		{after: tick, event: event(gateway.EventToolStarted, defaultSessionID, defaultRunID, payload("tool", "bash", "command", "go test ./..."))},
		{after: tick, event: event(gateway.EventPermissionRequested, defaultSessionID, defaultRunID, payload("request_id", "perm-001", "tool", "bash"))},
	}
}

// toolFailedEvents 返回工具执行失败流程的事件序列。
func toolFailedEvents() []scheduledEvent {
	return []scheduledEvent{
		{after: tick, event: event(gateway.EventToolStarted, defaultSessionID, defaultRunID, payload("tool", "webfetch"))},
		{after: tick, event: event(gateway.EventToolFinished, defaultSessionID, defaultRunID, payload("tool", "webfetch", "status", "failed"))},
		{after: tick, event: event(gateway.EventError, defaultSessionID, defaultRunID, payload("message", "tool failed: timeout"))},
	}
}

// runtimeErrorEvents 返回 Runtime 错误流程的事件序列。
func runtimeErrorEvents() []scheduledEvent {
	return []scheduledEvent{
		{after: tick, event: event(gateway.EventRunStarted, defaultSessionID, defaultRunID, payload("phase", "running"))},
		{after: tick, event: event(gateway.EventError, defaultSessionID, defaultRunID, payload("message", "runtime error: provider stream interrupted"))},
		{after: tick, event: event(gateway.EventRunFinished, defaultSessionID, defaultRunID, payload("phase", "failed"))},
	}
}

// askUserEvents 返回 ask_user 问答等待流程的事件序列。
func askUserEvents() []scheduledEvent {
	return []scheduledEvent{
		{after: tick, event: event(gateway.EventRunStarted, defaultSessionID, defaultRunID, payload("phase", "running"))},
		{after: tick, event: event(gateway.EventUserQuestionRequested, defaultSessionID, defaultRunID, payload("question_id", "ask-001", "question", "Which branch should I use?"))},
	}
}

// cancelRunningEvents 返回运行中取消流程的事件序列。
func cancelRunningEvents() []scheduledEvent {
	return []scheduledEvent{
		{after: tick, event: event(gateway.EventRunStarted, defaultSessionID, defaultRunID, payload("phase", "running"))},
		{after: 2 * tick, event: event(gateway.EventAgentChunk, defaultSessionID, defaultRunID, payload("text", "Working..."))},
		{after: tick, event: event(gateway.EventRunCancelled, defaultSessionID, defaultRunID, payload("phase", "cancelled"))},
	}
}

// longStreamItems 返回大量历史消息，用于滚动区域验证。
func longStreamItems() []gateway.StreamItem {
	items := make([]gateway.StreamItem, 0, 32)
	for i := 1; i <= 32; i++ {
		items = append(items, assistantItem(fmt.Sprintf("long-%02d", i), fmt.Sprintf("Long output line %02d for scroll testing.", i)))
	}
	return items
}

// manySessionSummaries 返回大量会话摘要，用于 picker 场景验证。
func manySessionSummaries() []gateway.SessionSummary {
	sessions := make([]gateway.SessionSummary, 0, 24)
	for i := 1; i <= 24; i++ {
		sessions = append(sessions, gateway.SessionSummary{
			ID:        fmt.Sprintf("session-%02d", i),
			Title:     fmt.Sprintf("Session %02d", i),
			Mode:      "input",
			Model:     defaultModelID,
			UpdatedAt: fixtureTime.Add(time.Duration(i) * time.Minute),
		})
	}
	return sessions
}

// detailsForSessions 为会话摘要批量生成对应详情。
func detailsForSessions(sessions []gateway.SessionSummary) map[string]gateway.SessionDetail {
	details := make(map[string]gateway.SessionDetail, len(sessions))
	for _, summary := range sessions {
		details[summary.ID] = gateway.SessionDetail{
			Summary: summary,
			Stream:  []gateway.StreamItem{assistantItem("item-"+summary.ID, "Session fixture loaded.")},
			Usage:   gateway.TokenUsage{Input: 10, Output: 20, Total: 30},
		}
	}
	return details
}

// userItem 构造用户角色的历史消息。
func userItem(id string, text string) gateway.StreamItem {
	return streamItem(id, "message", "user", text, "done")
}

// assistantItem 构造助手角色的历史消息。
func assistantItem(id string, text string) gateway.StreamItem {
	return streamItem(id, "message", "assistant", text, "done")
}

// streamItem 构造通用历史流记录。
func streamItem(id string, kind string, role string, text string, status string) gateway.StreamItem {
	return gateway.StreamItem{ID: id, Kind: kind, Role: role, Text: text, Status: status, CreatedAt: fixtureTime}
}

// event 构造带固定时间戳的 Gateway 事件。
func event(kind gateway.EventType, sessionID string, runID string, data map[string]any) gateway.GatewayEvent {
	return gateway.GatewayEvent{Type: kind, SessionID: sessionID, RunID: runID, Payload: data, At: fixtureTime}
}

// payload 将键值参数转换为事件 payload。
func payload(values ...any) map[string]any {
	data := make(map[string]any, len(values)/2)
	for i := 0; i+1 < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			continue
		}
		data[key] = values[i+1]
	}
	return data
}
