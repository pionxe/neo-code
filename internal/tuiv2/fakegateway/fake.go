// Package fakegateway 提供面向 TUI v2 Gateway 客户端接口的 Fake 实现。
package fakegateway

import (
	"fmt"

	"neo-code/internal/tuiv2/gateway"
)

// Config 描述 Fake Gateway 客户端的启动参数。
type Config struct {
	Scenario string
}

type client struct {
	scenario string
}

// New 创建 Fake Gateway 客户端，占位实现只校验场景，不向 UI 暴露硬编码数据。
func New(cfg Config) (gateway.Client, error) {
	if !IsKnownScenario(cfg.Scenario) {
		return nil, fmt.Errorf("unknown fake gateway scenario %q", cfg.Scenario)
	}
	return client{scenario: cfg.Scenario}, nil
}
