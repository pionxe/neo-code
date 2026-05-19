// Package gateway 定义 TUI v2 访问 Gateway 的客户端侧契约。
package gateway

import (
	"context"
	"time"
)

// Client 是 TUI v2 获取数据、发送动作和订阅事件的唯一入口。
type Client interface {
	Health(ctx context.Context) (*HealthResult, error)
	ListSessions(ctx context.Context) ([]SessionSummary, error)
	LoadSession(ctx context.Context, id string) (*SessionDetail, error)
	CreateSession(ctx context.Context) (*SessionSummary, error)
	SendMessage(ctx context.Context, sessionID string, text string) (*RunAck, error)
	CancelRun(ctx context.Context, sessionID string, runID string) error
	SubscribeEvents(ctx context.Context, sessionID string) (<-chan GatewayEvent, error)
	ResolvePermission(ctx context.Context, decision PermissionDecision) error
	AnswerUserQuestion(ctx context.Context, answer UserQuestionAnswer) error
	ListModels(ctx context.Context) ([]ModelInfo, error)
	SetModel(ctx context.Context, sessionID string, modelID string) error
	GetModel(ctx context.Context, sessionID string) (string, error)
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
