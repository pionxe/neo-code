package gateway

import (
	"context"
	"errors"
)

var errRealClientReserved = errors.New("real gateway client is reserved for Phase 20")

// RealClient 是真实 Gateway 客户端占位，Phase 20 才接入具体传输协议。
type RealClient struct{}

var _ Client = (*RealClient)(nil)

// NewRealClient 创建真实 Gateway 客户端占位实例，当前仅用于冻结接口形状。
func NewRealClient() *RealClient {
	return &RealClient{}
}

// Health 返回真实 Gateway 客户端未实现错误。
func (c *RealClient) Health(ctx context.Context) (*HealthResult, error) {
	return nil, errRealClientReserved
}

// ListSessions 返回真实 Gateway 客户端未实现错误。
func (c *RealClient) ListSessions(ctx context.Context) ([]SessionSummary, error) {
	return nil, errRealClientReserved
}

// LoadSession 返回真实 Gateway 客户端未实现错误。
func (c *RealClient) LoadSession(ctx context.Context, id string) (*SessionDetail, error) {
	return nil, errRealClientReserved
}

// CreateSession 返回真实 Gateway 客户端未实现错误。
func (c *RealClient) CreateSession(ctx context.Context) (*SessionSummary, error) {
	return nil, errRealClientReserved
}

// SendMessage 返回真实 Gateway 客户端未实现错误。
func (c *RealClient) SendMessage(ctx context.Context, sessionID string, text string) (*RunAck, error) {
	return nil, errRealClientReserved
}

// CancelRun 返回真实 Gateway 客户端未实现错误。
func (c *RealClient) CancelRun(ctx context.Context, sessionID string, runID string) error {
	return errRealClientReserved
}

// SubscribeEvents 返回真实 Gateway 客户端未实现错误。
func (c *RealClient) SubscribeEvents(ctx context.Context, sessionID string) (<-chan GatewayEvent, error) {
	return nil, errRealClientReserved
}

// ResolvePermission 返回真实 Gateway 客户端未实现错误。
func (c *RealClient) ResolvePermission(ctx context.Context, decision PermissionDecision) error {
	return errRealClientReserved
}

// AnswerUserQuestion 返回真实 Gateway 客户端未实现错误。
func (c *RealClient) AnswerUserQuestion(ctx context.Context, answer UserQuestionAnswer) error {
	return errRealClientReserved
}

// ListModels 返回真实 Gateway 客户端未实现错误。
func (c *RealClient) ListModels(ctx context.Context) ([]ModelInfo, error) {
	return nil, errRealClientReserved
}

// SetModel 返回真实 Gateway 客户端未实现错误。
func (c *RealClient) SetModel(ctx context.Context, sessionID string, modelID string) error {
	return errRealClientReserved
}

// GetModel 返回真实 Gateway 客户端未实现错误。
func (c *RealClient) GetModel(ctx context.Context, sessionID string) (string, error) {
	return "", errRealClientReserved
}

// Close 关闭真实 Gateway 客户端占位实例。
func (c *RealClient) Close() error {
	return nil
}
