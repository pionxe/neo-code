// Package fakegateway 提供面向 TUI v2 Gateway 客户端接口的 Fake 实现。
package fakegateway

import (
	"context"
	"fmt"
	"time"

	"neo-code/internal/tuiv2/gateway"
)

// Config 描述 Fake Gateway 客户端的启动参数。
type Config struct {
	Scenario string
}

type client struct {
	scenario string
}

var _ gateway.Client = (*client)(nil)

// New 创建 Fake Gateway 客户端，占位实现只校验场景，不向 UI 暴露硬编码数据。
func New(cfg Config) (gateway.Client, error) {
	if !IsKnownScenario(cfg.Scenario) {
		return nil, fmt.Errorf("unknown fake gateway scenario %q", cfg.Scenario)
	}
	return &client{scenario: cfg.Scenario}, nil
}

// Health 返回 Fake Gateway 的最小健康检查结果。
func (c *client) Health(ctx context.Context) (*gateway.HealthResult, error) {
	return &gateway.HealthResult{
		OK:      c.scenario != ScenarioGatewayOffline,
		Status:  c.healthStatus(),
		Backend: "fake",
		Message: c.scenario,
	}, nil
}

// ListSessions 返回 Fake Gateway 的会话摘要列表，Phase 2 保持为空集合。
func (c *client) ListSessions(ctx context.Context) ([]gateway.SessionSummary, error) {
	return []gateway.SessionSummary{}, nil
}

// LoadSession 返回 Fake Gateway 的最小会话详情。
func (c *client) LoadSession(ctx context.Context, id string) (*gateway.SessionDetail, error) {
	return &gateway.SessionDetail{
		Summary: gateway.SessionSummary{
			ID:        id,
			Title:     "Untitled",
			Mode:      "input",
			Model:     "fake",
			UpdatedAt: time.Now(),
		},
		Stream: []gateway.StreamItem{},
	}, nil
}

// CreateSession 返回 Fake Gateway 的新会话摘要。
func (c *client) CreateSession(ctx context.Context) (*gateway.SessionSummary, error) {
	return &gateway.SessionSummary{
		ID:        "fake-session",
		Title:     "Untitled",
		Mode:      "input",
		Model:     "fake",
		UpdatedAt: time.Now(),
	}, nil
}

// SendMessage 返回 Fake Gateway 对用户消息的 run 确认。
func (c *client) SendMessage(ctx context.Context, sessionID string, text string) (*gateway.RunAck, error) {
	return &gateway.RunAck{
		SessionID: sessionID,
		RunID:     "fake-run",
		Accepted:  true,
		Message:   text,
	}, nil
}

// CancelRun 接收 Fake Gateway 的取消请求。
func (c *client) CancelRun(ctx context.Context, sessionID string, runID string) error {
	return nil
}

// SubscribeEvents 返回 Fake Gateway 事件流，Phase 2 中默认立即关闭。
func (c *client) SubscribeEvents(ctx context.Context, sessionID string) (<-chan gateway.GatewayEvent, error) {
	events := make(chan gateway.GatewayEvent)
	close(events)
	return events, nil
}

// ResolvePermission 接收 Fake Gateway 的权限决策结果。
func (c *client) ResolvePermission(ctx context.Context, decision gateway.PermissionDecision) error {
	return nil
}

// AnswerUserQuestion 接收 Fake Gateway 的 ask_user 回答。
func (c *client) AnswerUserQuestion(ctx context.Context, answer gateway.UserQuestionAnswer) error {
	return nil
}

// ListModels 返回 Fake Gateway 的最小模型列表。
func (c *client) ListModels(ctx context.Context) ([]gateway.ModelInfo, error) {
	return []gateway.ModelInfo{
		{
			ID:       "fake",
			Name:     "Fake Model",
			Provider: "fake",
			Current:  true,
		},
	}, nil
}

// SetModel 接收 Fake Gateway 的模型切换请求。
func (c *client) SetModel(ctx context.Context, sessionID string, modelID string) error {
	return nil
}

// GetModel 返回 Fake Gateway 的当前模型 ID。
func (c *client) GetModel(ctx context.Context, sessionID string) (string, error) {
	return "fake", nil
}

// Close 关闭 Fake Gateway 客户端。
func (c *client) Close() error {
	return nil
}

// healthStatus 根据当前场景返回 Fake Gateway 健康状态文本。
func (c *client) healthStatus() string {
	if c.scenario == ScenarioGatewayOffline {
		return "offline"
	}
	return "ok"
}
