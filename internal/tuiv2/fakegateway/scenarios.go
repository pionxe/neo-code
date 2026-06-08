package fakegateway

import (
	"time"

	"neo-code/internal/tuiv2/gateway"
)

const (
	// ScenarioDefault 是 TUI v2 默认启动场景，提供完整 Ghost Console 演示数据。
	ScenarioDefault = "default"
	// ScenarioEmptySessions 用于验证空会话列表。
	ScenarioEmptySessions = "empty_sessions"
	// ScenarioStreamingChat 用于验证助手逐字流式输出。
	ScenarioStreamingChat = "streaming_chat"
	// ScenarioToolApproval 用于验证工具权限等待流程。
	ScenarioToolApproval = "tool_approval"
	// ScenarioToolFailed 用于验证工具调用失败。
	ScenarioToolFailed = "tool_failed"
	// ScenarioGatewayOffline 用于验证 Gateway 不可达。
	ScenarioGatewayOffline = "gateway_offline"
	// ScenarioRuntimeError 用于验证 Runtime 内部错误。
	ScenarioRuntimeError = "runtime_error"
	// ScenarioLongOutput 用于验证大量消息滚动。
	ScenarioLongOutput = "long_output"
	// ScenarioManySessions 用于验证大量会话选择器。
	ScenarioManySessions = "many_sessions"
	// ScenarioSmallTerminal 用于验证窄终端适配。
	ScenarioSmallTerminal = "small_terminal"
	// ScenarioSlowGateway 用于验证高延迟 Gateway。
	ScenarioSlowGateway = "slow_gateway"
	// ScenarioAskUser 用于验证用户问答流程。
	ScenarioAskUser = "ask_user"
	// ScenarioCancelRunning 用于验证取消运行流程。
	ScenarioCancelRunning = "cancel_running"
)

type fakeScenario struct {
	name         string
	health       gateway.HealthResult
	healthError  error
	rpcDelay     time.Duration
	sessions     []gateway.SessionSummary
	details      map[string]gateway.SessionDetail
	models       []gateway.ModelInfo
	currentModel string
	events       []scheduledEvent
	sendAck      gateway.RunAck
}

type scheduledEvent struct {
	after time.Duration
	event gateway.GatewayEvent
}

var knownScenarios = map[string]struct{}{
	ScenarioDefault:        {},
	ScenarioEmptySessions:  {},
	ScenarioStreamingChat:  {},
	ScenarioToolApproval:   {},
	ScenarioToolFailed:     {},
	ScenarioGatewayOffline: {},
	ScenarioRuntimeError:   {},
	ScenarioLongOutput:     {},
	ScenarioManySessions:   {},
	ScenarioSmallTerminal:  {},
	ScenarioSlowGateway:    {},
	ScenarioAskUser:        {},
	ScenarioCancelRunning:  {},
}

// IsKnownScenario 判断场景名是否属于当前 Fake Gateway 预置场景集合。
func IsKnownScenario(name string) bool {
	_, ok := knownScenarios[name]
	return ok
}

// scenarioByName 返回场景定义副本，调用方可以安全修改返回值中的切片内容。
func scenarioByName(name string) (fakeScenario, bool) {
	base := baseScenario(name)
	switch name {
	case ScenarioDefault:
		base.events = defaultEvents()
	case ScenarioEmptySessions:
		base.sessions = []gateway.SessionSummary{}
		base.details = map[string]gateway.SessionDetail{}
		base.events = []scheduledEvent{}
	case ScenarioStreamingChat:
		base.details[defaultSessionID] = detailWithStream([]gateway.StreamItem{
			userItem("stream-user-1", "What is the Ghost Console design?"),
			assistantItem("stream-agent-1", "Let me walk you through the Ghost Console architecture step by step."),
		})
		base.events = streamingEvents()
	case ScenarioToolApproval:
		base.events = toolApprovalEvents()
	case ScenarioToolFailed:
		base.events = toolFailedEvents()
	case ScenarioGatewayOffline:
		base.health = gateway.HealthResult{OK: false, Status: "offline", Backend: "fake", Message: "gateway offline"}
		base.healthError = errGatewayOffline
		base.sessions = []gateway.SessionSummary{}
		base.details = map[string]gateway.SessionDetail{}
		base.events = []scheduledEvent{{after: tick, event: event(gateway.EventGatewayOffline, "", "", payload("message", "gateway offline"))}}
	case ScenarioRuntimeError:
		base.events = runtimeErrorEvents()
	case ScenarioLongOutput:
		base.details[defaultSessionID] = detailWithStream(longStreamItems())
		base.events = []scheduledEvent{{after: tick, event: event(gateway.EventSessionUpdated, defaultSessionID, "", payload("phase", "loaded long output"))}}
	case ScenarioManySessions:
		base.sessions = manySessionSummaries()
		base.details = detailsForSessions(base.sessions)
		base.events = []scheduledEvent{{after: tick, event: event(gateway.EventSessionUpdated, base.sessions[0].ID, "", payload("count", len(base.sessions)))}}
	case ScenarioSmallTerminal:
		base.details[defaultSessionID] = detailWithStream([]gateway.StreamItem{
			userItem("small-user", "Short"),
			assistantItem("small-agent", "Compact layout data for narrow terminals."),
		})
		base.events = []scheduledEvent{{after: tick, event: event(gateway.EventSessionUpdated, defaultSessionID, "", payload("terminal", "small"))}}
	case ScenarioSlowGateway:
		base.rpcDelay = 80 * time.Millisecond
		base.events = []scheduledEvent{
			{after: 40 * time.Millisecond, event: event(gateway.EventRunStarted, defaultSessionID, defaultRunID, payload("phase", "slow start"))},
			{after: 80 * time.Millisecond, event: event(gateway.EventAgentChunk, defaultSessionID, defaultRunID, payload("text", "Delayed response arrived."))},
			{after: 40 * time.Millisecond, event: event(gateway.EventRunFinished, defaultSessionID, defaultRunID, payload("phase", "done"))},
		}
	case ScenarioAskUser:
		base.events = askUserEvents()
	case ScenarioCancelRunning:
		base.events = cancelRunningEvents()
	default:
		return fakeScenario{}, false
	}
	return cloneScenario(base), true
}

// baseScenario 构造所有场景共享的默认连接、会话和模型状态。
func baseScenario(name string) fakeScenario {
	summary := defaultSessionSummary()
	sessions := []gateway.SessionSummary{
		summary,
		{ID: "session-api-debug", Title: "API Debugging Session", Mode: "input", Model: defaultModelID, UpdatedAt: fixtureTime.Add(-30 * time.Minute)},
		{ID: "session-refactor", Title: "Refactor TUI Components", Mode: "input", Model: defaultModelID, UpdatedAt: fixtureTime.Add(-1 * time.Hour)},
		{ID: "session-deploy", Title: "Deploy Preparation", Mode: "input", Model: "neo-fake-fast", UpdatedAt: fixtureTime.Add(-2 * time.Hour)},
	}
	details := detailsForSessions(sessions)
	// Override the first session's detail with the richer default
	details[summary.ID] = defaultSessionDetail()
	return fakeScenario{
		name:         name,
		health:       gateway.HealthResult{OK: true, Status: "ok", Backend: "fake", Message: name},
		rpcDelay:     tick,
		sessions:     sessions,
		details:      details,
		models:       defaultModels(),
		currentModel: defaultModelID,
		sendAck:      gateway.RunAck{SessionID: summary.ID, RunID: defaultRunID, Accepted: true, Message: "accepted"},
	}
}

// cloneScenario 复制场景中的切片和 map，避免测试或调用方共享底层数据。
func cloneScenario(src fakeScenario) fakeScenario {
	src.sessions = append([]gateway.SessionSummary(nil), src.sessions...)
	src.models = append([]gateway.ModelInfo(nil), src.models...)
	src.events = append([]scheduledEvent(nil), src.events...)
	details := make(map[string]gateway.SessionDetail, len(src.details))
	for key, detail := range src.details {
		detail.Stream = append([]gateway.StreamItem(nil), detail.Stream...)
		details[key] = detail
	}
	src.details = details
	return src
}
