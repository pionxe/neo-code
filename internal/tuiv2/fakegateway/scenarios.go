package fakegateway

const (
	// ScenarioDefault 是 TUI v2 默认启动场景。
	ScenarioDefault = "default"
	// ScenarioToolApproval 是后续阶段用于工具审批流程的 Fake Gateway 场景。
	ScenarioToolApproval = "tool_approval"
	// ScenarioGatewayOffline 是后续阶段用于 Gateway 离线状态的 Fake Gateway 场景。
	ScenarioGatewayOffline = "gateway_offline"
)

var knownScenarios = map[string]struct{}{
	ScenarioDefault:        {},
	ScenarioToolApproval:   {},
	ScenarioGatewayOffline: {},
}

// IsKnownScenario 判断场景名是否属于当前 Fake Gateway 预置场景集合。
func IsKnownScenario(name string) bool {
	_, ok := knownScenarios[name]
	return ok
}
