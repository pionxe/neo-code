// Package gateway 定义 TUI v2 访问 Gateway 的客户端侧契约。
package gateway

import (
	"context"
	"errors"
	"time"
)

// ErrUnsupported 表示真实 Gateway 不提供该能力（ADR-002）：
// UI 层收到后必须显式降级呈现，后端零改动。
// 契约事实（S4 审计核实）：当前 Client 全部 13 方法在真实网关侧均有
// 已注册实现（映射见 Client 注释与 docs/reference/tui-gateway-contract-matrix.md），
// 本哨兵是面向未来扩展的契约兑现——新增方法时若真实网关暂缺对应
// RPC，实现方返回本哨兵而非自造错误。
var ErrUnsupported = errors.New("gateway: unsupported capability")

// Client 是 TUI v2 获取数据、发送动作和订阅事件的唯一入口。
//
// 方法↔真实 RPC 映射（权威依据：docs/reference/tui-gateway-contract-matrix.md
// 与 internal/gateway/registry.go 注册表，S4 逐一核实）：
// Health→gateway.ping；ListSessions→gateway.listSessions；
// LoadSession→gateway.loadSession；CreateSession→gateway.createSession；
// SendMessage→gateway.run；CancelRun→gateway.cancel；
// SubscribeEvents→gateway.bindStream + gateway.event 通知；
// ResolvePermission→gateway.resolvePermission；
// AnswerUserQuestion→gateway.userQuestionAnswer；
// ListModels→gateway.listModels；SetModel→gateway.setSessionModel；
// GetModel→gateway.getSessionModel。Close 为客户端本地资源释放，无 RPC。
type Client interface {
	// Health 对应 gateway.ping：连接健康检查。
	Health(ctx context.Context) (*HealthResult, error)
	// ListSessions 对应 gateway.listSessions：会话列表。
	ListSessions(ctx context.Context) ([]SessionSummary, error)
	// LoadSession 对应 gateway.loadSession：全会话快照。
	LoadSession(ctx context.Context, id string) (*SessionDetail, error)
	// CreateSession 对应 gateway.createSession：创建会话。
	CreateSession(ctx context.Context) (*SessionSummary, error)
	// SendMessage 对应 gateway.run：异步受理用户消息（返回 run 确认）。
	SendMessage(ctx context.Context, sessionID string, text string) (*RunAck, error)
	// CancelRun 对应 gateway.cancel：按 run/session 绑定取消运行。
	CancelRun(ctx context.Context, sessionID string, runID string) error
	// SubscribeEvents 对应 gateway.bindStream（事件经 gateway.event 通知推送）：
	// 订阅指定会话的事件流。
	SubscribeEvents(ctx context.Context, sessionID string) (<-chan GatewayEvent, error)
	// ResolvePermission 对应 gateway.resolvePermission：提交工具权限决策。
	ResolvePermission(ctx context.Context, decision PermissionDecision) error
	// AnswerUserQuestion 对应 gateway.userQuestionAnswer：提交 ask_user 回答。
	AnswerUserQuestion(ctx context.Context, answer UserQuestionAnswer) error
	// ListModels 对应 gateway.listModels：模型目录。
	ListModels(ctx context.Context) ([]ModelInfo, error)
	// SetModel 对应 gateway.setSessionModel：切换会话模型。
	SetModel(ctx context.Context, sessionID string, modelID string) error
	// GetModel 对应 gateway.getSessionModel：查询会话当前模型。
	GetModel(ctx context.Context, sessionID string) (string, error)
	// Close 释放客户端本地资源（无对应 RPC）。
	Close() error
}

// HealthResult 描述 Gateway 客户端可见的健康检查摘要。
type HealthResult struct {
	OK      bool
	Status  string
	Backend string
	Message string
}

// SessionSummary 描述 TUI 会话列表需要展示的最小信息。
type SessionSummary struct {
	ID        string
	Title     string
	Mode      string
	Model     string
	UpdatedAt time.Time
}

// SessionDetail 描述单个会话的可展示历史和用量摘要。
type SessionDetail struct {
	Summary SessionSummary
	Stream  []StreamItem
	Usage   TokenUsage
}

// StreamItem 描述会话流历史中的单条 UI 记录。
type StreamItem struct {
	ID        string
	Kind      string
	Role      string
	Text      string
	Status    string
	CreatedAt time.Time
}

// TokenUsage 描述会话详情中的 token 用量摘要。
type TokenUsage struct {
	Input  int
	Output int
	Total  int
}

// GatewayEvent 描述 Gateway 事件流中的一条通知，payload 只承载 TUI 自有 DTO 数据。
type GatewayEvent struct {
	Type      EventType
	SessionID string
	RunID     string
	Payload   map[string]any
	At        time.Time
}

// RunAck 描述用户消息触发 run 后返回给 UI 的确认信息。
type RunAck struct {
	SessionID string
	RunID     string
	Accepted  bool
	Message   string
}

// PermissionDecision 描述 UI 对工具权限请求的决策。
type PermissionDecision struct {
	RequestID string
	SessionID string
	RunID     string
	Allow     bool
	Reason    string
}

// UserQuestionAnswer 描述 UI 对 ask_user 问题的回答。
type UserQuestionAnswer struct {
	QuestionID string
	SessionID  string
	RunID      string
	Text       string
}

// ModelInfo 描述 TUI 模型选择器需要展示的模型摘要。
type ModelInfo struct {
	ID           string
	Name         string
	Provider     string
	Current      bool
	Capabilities []string
}
