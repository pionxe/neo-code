package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	gatewayclient "neo-code/internal/gateway/client"
)

// errRealNotImplemented 标记 RealClient 尚未落地的方法（S5 分阶段实装：
// 每个 commit 落地一个方法域，未落地方法保持显式错误而非静默占位）。
var errRealNotImplemented = errors.New("gateway: real client method not wired yet")

// defaultRealAuthTimeout 是构造期 fail-fast 认证的超时预算。
const defaultRealAuthTimeout = 10 * time.Second

// realRPCClient 是 RealClient 对 v1 RPC 客户端的最小依赖面：
// 生产注入 *gatewayclient.GatewayRPCClient，测试注入 mock 实现。
// 四方法与 v1 remoteGatewayRPCClient 接口同形（认证/调用/通知/关闭）。
type realRPCClient interface {
	Authenticate(ctx context.Context) error
	Call(ctx context.Context, method string, params any, result any) error
	Notifications() <-chan gatewayclient.Notification
	Close() error
}

// RealClientOptions 是 RealClient 的装配参数：复用 v1 RPC 客户端选项
// （地址解析/心跳/重试由其内建），零值即 v1 默认行为。
type RealClientOptions struct {
	// RPC 是 v1 RPC 客户端选项（ListenAddress/TokenFile/超时/重试等）。
	RPC gatewayclient.GatewayRPCClientOptions
	// RPCClient 直接注入已构造的 RPC 客户端（测试用，优先于 RPC 选项）。
	RPCClient realRPCClient
}

// RealClient 是 gateway.Client 的真实网关实现（ADR-004 薄翻译层）：
// 只读复用 v1 RPC 客户端（认证/心跳/重试/通知分发/自动地址解析内建），
// 自身仅做方法映射与事件扁平化；联调问题应落在翻译层而非插件层
// （ADR-004 证伪信号）。
type RealClient struct {
	rpc realRPCClient

	// subs 是会话订阅注册表：sessionID → 订阅条目（新顶替旧并关旧）。
	// 构造期单通知泵按此表扇出（S5 审计 P0-1：v1 Notifications 是单接收者
	// 共享通道，per-subscription 泵会在换代重叠窗口竞争偷事件）。
	mu   sync.Mutex
	subs map[string]*realSubscription

	// request_id 双槽追踪（S5 审计协调者裁定）：服务端强校验
	// resolvePermission/userQuestionAnswer 的 request_id 非空，而事件流
	// 权限/问答请求携带该 ID——泵在扇出前登记（见 flattenGatewayEvent），
	// 提交决策时若调用方未填则回填（显式值优先）。语义="最近 pending"，
	// 对齐 runtime 单会话串行挂起模型；*_resolved 事件清槽防陈旧回填。
	permReqID  map[string]string // sessionID → 最近 pending 权限请求 ID
	questReqID map[string]string // sessionID → 最近 pending 问答请求 ID

	closeOnce sync.Once
	closed    chan struct{}
}

// realSubscription 是一条会话订阅：下游通道 + close-once。
// 三条关闭路径可重叠（被顶替 / 调用方 ctx cancel / 客户端 Close），
// 必须 sync.Once 防双关 panic（S5 审计钉死项）。
type realSubscription struct {
	ch   chan GatewayEvent
	once sync.Once
}

// close 关闭下游通道（幂等）。
func (s *realSubscription) close() {
	s.once.Do(func() { close(s.ch) })
}

// NewRealClient 创建真实网关客户端并执行 fail-fast 认证连通性检查
// （对齐 v1 RemoteRuntimeAdapter 装配先例：认证失败即返回错误，不留半可用实例）。
//
// 纪律（S5 审计裁定）：强制 DisableAutoSpawn——v1 的自动拉起是自我重执行
// （exec 自身可执行文件 + "gateway" 子命令），tuiv2 二进制无该子命令，
// 自动拉起必然失败；真实网关由外部（脚本/运维）先行启动，客户端显式连接。
func NewRealClient(options RealClientOptions) (*RealClient, error) {
	var rpc realRPCClient
	if options.RPCClient != nil {
		rpc = options.RPCClient
	} else {
		options.RPC.DisableAutoSpawn = true
		client, err := gatewayclient.NewGatewayRPCClient(options.RPC)
		if err != nil {
			return nil, err
		}
		rpc = client
	}

	c := &RealClient{
		rpc:        rpc,
		subs:       make(map[string]*realSubscription),
		permReqID:  make(map[string]string),
		questReqID: make(map[string]string),
		closed:     make(chan struct{}),
	}

	authCtx, cancel := context.WithTimeout(context.Background(), defaultRealAuthTimeout)
	defer cancel()
	if err := rpc.Authenticate(authCtx); err != nil {
		_ = rpc.Close()
		return nil, err
	}
	return c, nil
}

// Close 释放客户端资源（幂等）；订阅通道由各订阅自身的关闭路径负责。
func (c *RealClient) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
		_ = c.rpc.Close()
	})
	return nil
}

// Health 对应 gateway.ping：连接健康检查。
func (c *RealClient) Health(ctx context.Context) (*HealthResult, error) {
	var frame map[string]any
	if err := c.rpc.Call(ctx, "gateway.ping", nil, &frame); err != nil {
		return nil, err
	}
	return &HealthResult{OK: true, Status: "ok", Backend: "gateway"}, nil
}

// realSessionSummary 是 gateway.listSessions 结果条目的本地解码结构
// （wire 契约 json tag 对齐服务端 DTO；tuiv2 不 import 服务端包，
// 只 import 客户端包——边界禁 4 放行面）。
type realSessionSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	AgentMode string    `json:"agent_mode"`
	Model     string    `json:"model"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListSessions 对应 gateway.listSessions：会话列表。
// 结果是整帧 MessageFrame，数据在 payload.sessions 下（v1 callFrame 先例）。
func (c *RealClient) ListSessions(ctx context.Context) ([]SessionSummary, error) {
	var frame struct {
		Payload struct {
			Sessions []realSessionSummary `json:"sessions"`
		} `json:"payload"`
	}
	if err := c.rpc.Call(ctx, "gateway.listSessions", nil, &frame); err != nil {
		return nil, err
	}
	out := make([]SessionSummary, 0, len(frame.Payload.Sessions))
	for _, item := range frame.Payload.Sessions {
		out = append(out, SessionSummary{
			ID:        strings.TrimSpace(item.ID),
			Title:     strings.TrimSpace(item.Title),
			Mode:      strings.TrimSpace(item.AgentMode),
			Model:     strings.TrimSpace(item.Model),
			UpdatedAt: item.UpdatedAt,
		})
	}
	return out, nil
}

// realSessionMessage 是会话消息快照条目的本地解码结构（wire 契约）。
// 服务端消息无 ID/CreatedAt 字段（v1 DTO 实测，S5 审计 Q7 裁定）。
type realSessionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	IsError bool   `json:"is_error"`
}

// realSession 是 gateway.loadSession 结果的本地解码结构（payload 直出）。
type realSession struct {
	ID        string               `json:"id"`
	Title     string               `json:"title"`
	AgentMode string               `json:"agent_mode"`
	Model     string               `json:"model"`
	UpdatedAt time.Time            `json:"updated_at"`
	Messages  []realSessionMessage `json:"messages"`
}

// translateSessionMessages 将 v1 会话消息快照翻译为 tuiv2 流条目
// （S5 审计 Q7 裁定：role user/assistant→message、tool→tool_end——
// 服务端消息无工具名来源，降级空；IsError→Status；ID 按序号合成，
// 服务端消息无 ID 字段）。
func translateSessionMessages(messages []realSessionMessage) []StreamItem {
	out := make([]StreamItem, 0, len(messages))
	for i, m := range messages {
		kind := "message"
		if m.Role == "tool" {
			kind = "tool_end"
		}
		status := ""
		if m.IsError {
			status = "error"
		}
		out = append(out, StreamItem{
			ID:        fmt.Sprintf("real-msg-%d", i+1),
			Kind:      kind,
			Role:      m.Role,
			Text:      m.Content,
			Status:    status,
			CreatedAt: time.Now(),
		})
	}
	return out
}

// LoadSession 对应 gateway.loadSession：全会话快照。
// 用量恒零：真实网关 Session 快照无用量字段（token 经 token_usage
// 事件流维护，S5 审计裁定）。
func (c *RealClient) LoadSession(ctx context.Context, id string) (*SessionDetail, error) {
	sessionID := strings.TrimSpace(id)
	if sessionID == "" {
		return nil, errors.New("gateway: session id is empty")
	}
	params := struct {
		SessionID string `json:"session_id"`
	}{SessionID: sessionID}
	var frame struct {
		Payload realSession `json:"payload"`
	}
	if err := c.rpc.Call(ctx, "gateway.loadSession", params, &frame); err != nil {
		return nil, err
	}
	loaded := frame.Payload
	return &SessionDetail{
		Summary: SessionSummary{
			ID:        strings.TrimSpace(loaded.ID),
			Title:     strings.TrimSpace(loaded.Title),
			Mode:      strings.TrimSpace(loaded.AgentMode),
			Model:     strings.TrimSpace(loaded.Model),
			UpdatedAt: loaded.UpdatedAt,
		},
		Stream: translateSessionMessages(loaded.Messages),
	}, nil
}

// CreateSession 对应 gateway.createSession：服务端返回创建后的会话 ID
// （frame 级 session_id），摘要其余字段由后续 listSessions 刷新补全。
func (c *RealClient) CreateSession(ctx context.Context) (*SessionSummary, error) {
	var frame struct {
		SessionID string `json:"session_id"`
	}
	if err := c.rpc.Call(ctx, "gateway.createSession", nil, &frame); err != nil {
		return nil, err
	}
	sessionID := strings.TrimSpace(frame.SessionID)
	if sessionID == "" {
		return nil, errors.New("gateway: createSession returned empty session id")
	}
	return &SessionSummary{ID: sessionID, Title: "New Session"}, nil
}

// SendMessage 对应 gateway.run：异步受理用户消息（返回 run 确认）。
// ack 的 session_id/run_id 在 frame 级；服务端省略时回退为请求值
// （v1 Submit 先例）。
func (c *RealClient) SendMessage(ctx context.Context, sessionID string, text string) (*RunAck, error) {
	params := struct {
		SessionID string `json:"session_id,omitempty"`
		InputText string `json:"input_text,omitempty"`
	}{
		SessionID: strings.TrimSpace(sessionID),
		InputText: text,
	}
	var frame struct {
		SessionID string `json:"session_id"`
		RunID     string `json:"run_id"`
	}
	if err := c.rpc.Call(ctx, "gateway.run", params, &frame); err != nil {
		return nil, err
	}
	ack := &RunAck{
		SessionID: strings.TrimSpace(frame.SessionID),
		RunID:     strings.TrimSpace(frame.RunID),
		Accepted:  true,
	}
	if ack.SessionID == "" {
		ack.SessionID = params.SessionID
	}
	return ack, nil
}

// CancelRun 对应 gateway.cancel：按 run/session 绑定取消运行。
func (c *RealClient) CancelRun(ctx context.Context, sessionID string, runID string) error {
	params := struct {
		SessionID string `json:"session_id,omitempty"`
		RunID     string `json:"run_id,omitempty"`
	}{
		SessionID: strings.TrimSpace(sessionID),
		RunID:     strings.TrimSpace(runID),
	}
	var frame struct{}
	return c.rpc.Call(ctx, "gateway.cancel", params, &frame)
}

// SubscribeEvents 对应 gateway.bindStream + gateway.event（S5 分阶段实装）。
func (c *RealClient) SubscribeEvents(ctx context.Context, sessionID string) (<-chan GatewayEvent, error) {
	return nil, errRealNotImplemented
}

// resolvePermRequestID 解析权限请求 ID：显式值优先，空则回填
// 追踪槽登记的最近 pending ID；两者皆空返回错误（服务端强校验
// request_id 非空，本地先行失败给出可读原因）。
func (c *RealClient) resolvePermRequestID(sessionID, explicit string) (string, error) {
	if id := strings.TrimSpace(explicit); id != "" {
		return id, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if id := c.permReqID[sessionID]; id != "" {
		return id, nil
	}
	return "", errors.New("gateway: permission request id unavailable（未捕获到 permission_requested 事件）")
}

// ResolvePermission 对应 gateway.resolvePermission：提交工具权限决策。
// Decision 映射 Allow→"allow"、否则 "deny"（服务端按小写枚举消费，
// v1 先例）；RequestID 为空时回填追踪槽（S5 审计协调者裁定）。
func (c *RealClient) ResolvePermission(ctx context.Context, decision PermissionDecision) error {
	requestID, err := c.resolvePermRequestID(decision.SessionID, decision.RequestID)
	if err != nil {
		return err
	}
	decisionValue := "deny"
	if decision.Allow {
		decisionValue = "allow"
	}
	params := struct {
		RequestID string `json:"request_id"`
		Decision  string `json:"decision"`
	}{
		RequestID: requestID,
		Decision:  decisionValue,
	}
	var frame struct{}
	return c.rpc.Call(ctx, "gateway.resolvePermission", params, &frame)
}

// resolveQuestRequestID 解析问答请求 ID（语义同 resolvePermRequestID，
// 双槽独立——权限/问答事件交错时共用一桶会错填，S5 审计钉死项）。
func (c *RealClient) resolveQuestRequestID(sessionID, explicit string) (string, error) {
	if id := strings.TrimSpace(explicit); id != "" {
		return id, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if id := c.questReqID[sessionID]; id != "" {
		return id, nil
	}
	return "", errors.New("gateway: question request id unavailable（未捕获到 user_question_requested 事件）")
}

// AnswerUserQuestion 对应 gateway.userQuestionAnswer：提交 ask_user 回答。
// 自由文本回答映射 Status="answered" + Message=Text（服务端 Status 可选，
// 语义标注 answered）；QuestionID 为空时回填追踪槽（协调者裁定）。
func (c *RealClient) AnswerUserQuestion(ctx context.Context, answer UserQuestionAnswer) error {
	requestID, err := c.resolveQuestRequestID(answer.SessionID, answer.QuestionID)
	if err != nil {
		return err
	}
	params := struct {
		RequestID string   `json:"request_id"`
		Status    string   `json:"status,omitempty"`
		Values    []string `json:"values,omitempty"`
		Message   string   `json:"message,omitempty"`
	}{
		RequestID: requestID,
		Status:    "answered",
		Message:   answer.Text,
	}
	var frame struct{}
	return c.rpc.Call(ctx, "gateway.userQuestionAnswer", params, &frame)
}

// realModelEntry 是 gateway.listModels 结果条目的本地解码结构（wire 契约）。
// CapabilityHints 是结构化对象而 tuiv2 ModelInfo.Capabilities 是字符串切片，
// 词汇不一致且模型选择器未消费——S5 不映射（如实留空，S7/S10 按需扩展）。
type realModelEntry struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

// ListModels 对应 gateway.listModels：模型目录 + 当前选中模型。
// Current 由 selected_model_id 派生（S5 审计裁定）；空 ID 条目跳过、
// 空名回退 ID（v1 先例）。
func (c *RealClient) ListModels(ctx context.Context) ([]ModelInfo, error) {
	var frame struct {
		Payload struct {
			Models          []realModelEntry `json:"models"`
			SelectedModelID string           `json:"selected_model_id"`
		} `json:"payload"`
	}
	if err := c.rpc.Call(ctx, "gateway.listModels", nil, &frame); err != nil {
		return nil, err
	}
	selected := strings.TrimSpace(frame.Payload.SelectedModelID)
	out := make([]ModelInfo, 0, len(frame.Payload.Models))
	for _, item := range frame.Payload.Models {
		modelID := strings.TrimSpace(item.ID)
		if modelID == "" {
			continue
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = modelID
		}
		out = append(out, ModelInfo{
			ID:       modelID,
			Name:     name,
			Provider: strings.TrimSpace(item.Provider),
			Current:  selected != "" && modelID == selected,
		})
	}
	return out, nil
}

// SetModel 对应 gateway.setSessionModel：切换会话模型。
func (c *RealClient) SetModel(ctx context.Context, sessionID string, modelID string) error {
	params := struct {
		SessionID string `json:"session_id"`
		ModelID   string `json:"model_id"`
	}{
		SessionID: strings.TrimSpace(sessionID),
		ModelID:   strings.TrimSpace(modelID),
	}
	var frame struct{}
	return c.rpc.Call(ctx, "gateway.setSessionModel", params, &frame)
}

// GetModel 对应 gateway.getSessionModel：查询会话当前模型
// （payload 为 SessionModelResult，model_id 为权威真值）。
func (c *RealClient) GetModel(ctx context.Context, sessionID string) (string, error) {
	params := struct {
		SessionID string `json:"session_id"`
	}{SessionID: strings.TrimSpace(sessionID)}
	var frame struct {
		Payload struct {
			ModelID string `json:"model_id"`
		} `json:"payload"`
	}
	if err := c.rpc.Call(ctx, "gateway.getSessionModel", params, &frame); err != nil {
		return "", err
	}
	return strings.TrimSpace(frame.Payload.ModelID), nil
}
