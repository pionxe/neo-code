package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	// Debug 开启翻译层的丢弃/异常日志（默认静默丢弃）。
	Debug bool
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

	debug bool

	closeOnce sync.Once
	closed    chan struct{}
	pumpDone  chan struct{}
}

// realSubscription 是一条会话订阅。
// 关闭协议两阶段（PR #47 审计实测暴露的发送竞态修正）：
//   - retire()：关闭 done 信号——必须在注册表 mu 内调用（与泵的
//     send-select 互斥），泵据此从 send 阻塞中经回退分支释放；
//   - closeCh()：关闭下游通道——必须在 mu 外调用（泵可能在 done
//     就绪后仍选中缓冲未满的 send 分支，先关通道会 panic）。
//
// 两条 once 各自幂等：三关闭路径（顶替/ctx cancel/客户端 Close）
// 可任意重叠。
type realSubscription struct {
	ch       chan GatewayEvent
	done     chan struct{} // retire 完成信号：trySend 回退与 watcher 退出
	retired  sync.Once
	chClosed sync.Once
	sendMu   sync.Mutex // 序列化 trySend 与 closeCh（发送竞态守卫）
}

// retire 关闭完成信号（幂等）；调用方须持注册表 mu。
func (s *realSubscription) retire() {
	s.retired.Do(func() { close(s.done) })
}

// closeCh 关闭下游通道（幂等）；持 sendMu 与 trySend 的发送互斥
// （杜绝并发 close/send 竞态），retire 已关 done——trySend 的阻塞
// 发送必经回退唤醒，无互锁；调用方须已 retire 且不在注册表 mu 内。
func (s *realSubscription) closeCh() {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	s.chClosed.Do(func() { close(s.ch) })
}

// trySend 尝试投递事件：订阅已 retire 时返回 false（事件丢弃）。
// sendMu 内先做 done 守卫再发送——done 已关闭⇔通道已关闭（closeCh
// 被 sendMu 挡住），守卫直接返回；done 未关闭⇒closeCh 被挡在 sendMu
// 外⇒发送安全（杜绝"关闭后进入"的 send-on-closed 竞态——PR #47
// 审计 P0-2 实测 500 次 234 次 panic）。
func (s *realSubscription) trySend(event GatewayEvent) bool {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	select {
	case <-s.done:
		return false
	default:
	}
	select {
	case s.ch <- event:
		return true
	case <-s.done:
		return false
	}
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
		options = normalizeRealOptions(options)
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
		debug:      options.Debug,
		closed:     make(chan struct{}),
		pumpDone:   make(chan struct{}),
	}
	go c.runPump()

	authCtx, cancel := context.WithTimeout(context.Background(), defaultRealAuthTimeout)
	defer cancel()
	if err := rpc.Authenticate(authCtx); err != nil {
		_ = rpc.Close()
		return nil, err
	}
	return c, nil
}

// normalizeRealOptions 归一装配选项：强制关闭自动拉起（v1 自动拉起是
// 自我重执行，tuiv2 二进制无 gateway 子命令必然失败——S5 审计 Q5/Q6
// 裁定的落地行，独立纯函数便于测试钉死）。
func normalizeRealOptions(options RealClientOptions) RealClientOptions {
	options.RPC.DisableAutoSpawn = true
	return options
}

// Close 释放客户端资源（幂等）；订阅通道由各订阅自身的关闭路径负责。
func (c *RealClient) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
		// 先 retire 全部订阅（trySend 的唯一回退信号），泵才能从满缓冲
		// 阻塞发送中释放——否则 Close 等 pumpDone 与泵等回退互锁。
		c.closeSubscriptions()
		_ = c.rpc.Close()
		<-c.pumpDone
	})
	return nil
}

// closeSubscriptions 关闭全部订阅（Close 路径）：先在 mu 内 retire 全部
// （含泵 send-select 的回退信号），mu 外再关通道（防发送竞态）。
func (c *RealClient) closeSubscriptions() {
	c.mu.Lock()
	pending := make([]*realSubscription, 0, len(c.subs))
	for _, sub := range c.subs {
		sub.retire()
		pending = append(pending, sub)
	}
	c.subs = make(map[string]*realSubscription)
	c.mu.Unlock()
	for _, sub := range pending {
		sub.closeCh()
	}
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

// SubscribeEvents 对应 gateway.bindStream（事件经 gateway.event 通知推送，
// 由构造期单泵扇出到本订阅）：绑定会话事件流并返回订阅通道。
// 生命周期契约与 fake 一致——调用方 ctx cancel 即订阅关闭；同会话
// 重复订阅为新顶替旧（先注册新后关旧，与 kernel 单流换代语义一致）。
func (c *RealClient) SubscribeEvents(ctx context.Context, sessionID string) (<-chan GatewayEvent, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("gateway: session id is empty")
	}
	if err := c.rpc.Call(ctx, "gateway.bindStream", struct {
		SessionID string `json:"session_id"`
	}{SessionID: sessionID}, &struct{}{}); err != nil {
		return nil, err
	}

	sub := &realSubscription{
		ch:   make(chan GatewayEvent, realSubBuffer),
		done: make(chan struct{}),
	}
	c.mu.Lock()
	old := c.subs[sessionID]
	c.subs[sessionID] = sub // 先注册新
	if old != nil {
		old.retire() // mu 内关 done：泵 send-select 立即获得回退信号
	}
	c.mu.Unlock()
	if old != nil {
		old.closeCh() // mu 外关旧通道（sendMu 序列化防发送竞态）
	}

	// 调用方 ctx cancel 路径：注销并关闭订阅（retire 在 mu 内）。
	go func() {
		select {
		case <-ctx.Done():
			c.mu.Lock()
			if c.subs[sessionID] == sub {
				delete(c.subs, sessionID)
				sub.retire()
			}
			c.mu.Unlock()
			sub.closeCh()
		case <-sub.done:
		}
	}()
	return sub.ch, nil
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
// Decision 映射 Allow→"allow_once"、否则 "reject"（服务端枚举仅接受
// allow_once/allow_session/reject——protocol/jsonrpc.go:1532 与
// validate.go:557 双路径强校验，PR #47 审计 P0 实测）；RequestID 为空时
// 回填追踪槽（S5 审计协调者裁定）。
func (c *RealClient) ResolvePermission(ctx context.Context, decision PermissionDecision) error {
	requestID, err := c.resolvePermRequestID(decision.SessionID, decision.RequestID)
	if err != nil {
		return err
	}
	decisionValue := "reject"
	if decision.Allow {
		decisionValue = "allow_once"
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

// ---------- 事件扁平化（S5 审计裁定的单一翻译点） ----------

// realSubBuffer 是订阅下游通道缓冲：吸收渲染抖动，避免泵阻塞传导为
// v1 通知队列背压（3s 强断共享连接）的连接级风险（S5 审计钉死项）。
const realSubBuffer = 128

// realEventChannel 是网关事件通知的方法名（协议常量的本地字符串，
// tuiv2 不 import 服务端协议包——边界禁 4 放行面仅客户端包）。
const realEventChannel = "gateway.event"

// realFrame 是 gateway.event 通知 params 的本地解码结构
// （wire 契约：{type, action, session_id, run_id, payload}）。
type realFrame struct {
	SessionID string         `json:"session_id"`
	RunID     string         `json:"run_id"`
	Payload   map[string]any `json:"payload"`
}

// runPump 是构造期唯一通知泵：消费共享通知通道，扁平化后按会话扇出。
// 退出两路径：客户端 Close（closed）/ 通知通道关闭（连接断开）。
func (c *RealClient) runPump() {
	defer close(c.pumpDone)
	for {
		select {
		case <-c.closed:
			return
		case notification, ok := <-c.rpc.Notifications():
			if !ok {
				return
			}
			if notification.Method != realEventChannel {
				continue
			}
			c.dispatchNotification(notification)
		}
	}
}

// dispatchNotification 将单条 gateway.event 通知翻译并扇出到订阅。
func (c *RealClient) dispatchNotification(notification gatewayclient.Notification) {
	var params realFrame
	if len(notification.Params) == 0 {
		return
	}
	if err := json.Unmarshal(notification.Params, &params); err != nil {
		c.debugf("gateway.event params decode: %v", err)
		return
	}
	sessionID := strings.TrimSpace(params.SessionID)

	// 无 envelope 的外层错误帧（run_error/ask_error，payload={code,message}）：
	// 必须消费——否则 run 失败后 UI 恒 running（S5 审计 P0-4）。
	event, hasEnvelope, deliverable := flattenGatewayEvent(params)
	if !hasEnvelope {
		event = c.errorFallbackEvent(params)
		if event.Type == "" {
			return
		}
		sessionID = strings.TrimSpace(params.SessionID)
	} else if !deliverable {
		c.debugf("drop runtime event（非 tuiv2 词汇，与白名单外 no-op 语义一致）")
		return
	}

	// request_id 追踪：泵内扇出前写入（无订阅/换代窗口仍可回填）；
	// *_resolved 事件清槽防陈旧回填（S5 审计钉死项）。
	c.trackRequestID(event)

	// 扇出：注册表 mu 内仅做订阅查找（不持 mu 发送——顶替/关闭路径需要
	// mu，否则泵阻塞发送时构成死锁）；发送交由订阅级 trySend（done 回退
	// + sendMu 序列化，无发送竞态、无死锁）。
	c.mu.Lock()
	sub := c.subs[sessionID]
	c.mu.Unlock()
	if sub == nil {
		return // 无订阅：事件自然消亡（通道生命周期由订阅方管理）
	}
	sub.trySend(event)
}

// errorFallbackEvent 将无 envelope 的外层错误帧翻译为 EventError；
// 非 run_error/ask_error 类返回零值（调用方跳过）。
func (c *RealClient) errorFallbackEvent(params realFrame) GatewayEvent {
	eventType, _ := params.Payload["event_type"].(string)
	switch eventType {
	case "run_error", "ask_error":
		message, _ := params.Payload["message"].(string)
		if message == "" {
			if code, ok := params.Payload["code"].(string); ok {
				message = code
			}
		}
		return GatewayEvent{
			Type:      EventError,
			SessionID: params.SessionID,
			RunID:     params.RunID,
			Payload:   map[string]any{"message": message},
			At:        time.Now(),
		}
	default:
		return GatewayEvent{}
	}
}

// trackRequestID 在扇出前登记/清槽 request_id（双槽独立，S5 协调者裁定）。
func (c *RealClient) trackRequestID(event GatewayEvent) {
	switch event.Type {
	case EventPermissionRequested:
		if id, _ := event.Payload["request_id"].(string); id != "" {
			c.mu.Lock()
			c.permReqID[event.SessionID] = id
			c.mu.Unlock()
		}
	case EventPermissionResolved:
		c.mu.Lock()
		delete(c.permReqID, event.SessionID)
		c.mu.Unlock()
	case EventUserQuestionRequested:
		if id, _ := event.Payload["request_id"].(string); id != "" {
			c.mu.Lock()
			c.questReqID[event.SessionID] = id
			c.mu.Unlock()
		}
	case EventUserQuestionAnswered:
		c.mu.Lock()
		delete(c.questReqID, event.SessionID)
		c.mu.Unlock()
	}
}

// flattenGatewayEvent 从通知 payload 提取 runtime envelope 并翻译为
// tuiv2 事件（S4 三类分组的 A 组同名映射 + 值/键归一化）。
// 返回值：hasEnvelope=false 表示无 runtime envelope（调用方走外层错误帧
// 回退）；hasEnvelope=true 且 deliverable=false 表示有 envelope 但事件
// 不在 tuiv2 词汇内（调用方丢弃——不可扇出空事件）。
func flattenGatewayEvent(params realFrame) (event GatewayEvent, hasEnvelope bool, deliverable bool) {
	envelope := extractRuntimeEnvelope(params.Payload)
	if envelope == nil {
		return GatewayEvent{}, false, false
	}
	runtimeType, _ := envelope["runtime_event_type"].(string)
	eventType, ok := translateRuntimeEventType(runtimeType)
	if !ok {
		return GatewayEvent{}, true, false
	}
	// payload_version 硬校验（契约矩阵：mismatch = 硬不兼容，fail fast；
	// 对齐 v1 runtimeEventPayloadVersion=4 语义）。
	if got := payloadIntValue(envelope, "payload_version"); got != 4 {
		return GatewayEvent{
			Type:      EventError,
			SessionID: strings.TrimSpace(params.SessionID),
			RunID:     strings.TrimSpace(params.RunID),
			Payload:   map[string]any{"message": fmt.Sprintf("unsupported runtime payload_version: got %d want 4", got)},
			At:        time.Now(),
		}, true, true
	}
	payload := normalizeEventPayload(eventType, runtimeType, envelope)

	updatedAt := time.Now()
	if ts, _ := envelope["timestamp"].(string); ts != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			updatedAt = parsed
		}
	}
	return GatewayEvent{
		Type:      eventType,
		SessionID: strings.TrimSpace(params.SessionID),
		RunID:     strings.TrimSpace(params.RunID),
		Payload:   payload,
		At:        updatedAt,
	}, true, true
}

// extractRuntimeEnvelope 提取 runtime envelope：支持直接形态
// （{runtime_event_type,...}）与 gateway 包裹形态（{event_type,payload:{...}}）。
func extractRuntimeEnvelope(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	if _, ok := payload["runtime_event_type"]; ok {
		return payload
	}
	if nested, ok := payload["payload"].(map[string]any); ok {
		if _, ok := nested["runtime_event_type"]; ok {
			return nested
		}
	}
	return nil
}

// translateRuntimeEventType 将 runtime 事件名映射为 tuiv2 事件常量
// （S4 已同名对齐的 A 组 + 两个特映射）。返回 ok=false 表示 tuiv2 不消费
// （丢弃 + debug 日志的集中点——与 ReduceWithoutInput 白名单语义一致）。
func translateRuntimeEventType(runtimeType string) (EventType, bool) {
	switch strings.TrimSpace(runtimeType) {
	case "agent_chunk":
		return EventAgentChunk, true
	case "tool_start":
		return EventToolStart, true
	case "tool_result":
		return EventToolResult, true
	case "tool_chunk":
		return EventToolOutput, true // runtime 词汇 tool_chunk ↔ tuiv2 tool_output（S4 登记的特映射）
	case "run_canceled":
		return EventRunCanceled, true
	case "token_usage":
		return EventTokenUsage, true
	case "phase_changed":
		return EventPhaseChanged, true
	case "permission_requested":
		return EventPermissionRequested, true
	case "permission_resolved":
		return EventPermissionResolved, true
	case "user_question_requested":
		return EventUserQuestionRequested, true
	case "user_question_answered":
		return EventUserQuestionAnswered, true
	case "error":
		return EventError, true
	case "agent_done":
		return EventRunFinished, true // envelope 结束边界 → C 组 run_finished（派生来源登记）
	default:
		return "", false
	}
}

// tuiv2 phase 词表值（state.RuntimePhase* 的字符串字面量——gateway 包
// 不能 import state（会成环：state 依赖本包 DTO），以字面量+出处注释对齐）。
const (
	realPhaseIdle        = "idle"
	realPhaseRunning     = "running"
	realPhaseWaitingUser = "waiting_user"
)

// translatePhaseValue 是 phase 值翻译（runtime 词表 → tuiv2 词表）：
// 直传 execute 会使 chat runCancel 静默失效、statusbar 误渲染 idle
// （S5 审计 P1-7 双实例证实）；waiting_permission 与 tuiv2 同名直通。
func translatePhaseValue(value string) string {
	switch value {
	case "plan", "execute", "verify", "compacting":
		return realPhaseRunning
	case "waiting_user_question":
		return realPhaseWaitingUser
	case "stopped":
		return realPhaseIdle
	default:
		return value
	}
}

// normalizeEventPayload 按事件做最小键归一化（产出键 = fake fixtures
// 词表，即 state 消费的 golden 契约——S5 审计裁定的三归一）：
// 纯字符串 payload 包装 / envelope 顶层键提升 / PascalCase→小写。
func normalizeEventPayload(eventType EventType, runtimeType string, envelope map[string]any) map[string]any {
	// 纯字符串 payload（runtime 侧这三类事件 payload 就是字符串）。
	if raw, ok := envelope["payload"].(string); ok {
		switch eventType {
		case EventAgentChunk, EventToolOutput, EventError:
			return map[string]any{"text": raw}
		}
	}

	out := map[string]any{}
	for key, value := range envelope {
		if key == "runtime_event_type" || key == "payload_version" {
			continue // 协议元数据，非业务键
		}
		out[strings.ToLower(key)] = normalizeValue(value)
	}
	if nested, ok := envelope["payload"].(map[string]any); ok {
		for key, value := range nested {
			out[strings.ToLower(key)] = normalizeValue(value)
		}
	}

	switch eventType {
	case EventUserQuestionRequested:
		// runtime 键 title/description → state 消费键 question（缺 question
		// 时 ask_user 问题文本恒空——PR #47 审计 P1-1a）。
		if _, ok := out["question"]; !ok {
			if title, ok := out["title"].(string); ok && title != "" {
				out["question"] = title
			} else if desc, ok := out["description"].(string); ok {
				out["question"] = desc
			}
		}
	case EventToolStart:
		// runtime 键 arguments（JSON 字符串）→ state 消费键 input（缺省时
		// 工具行无命令摘要——PR #47 审计 P1-1b）。
		if _, ok := out["input"]; !ok {
			if args, ok := out["arguments"].(string); ok {
				out["input"] = args
			}
		}
	case EventPhaseChanged:
		// phase 值翻译：to（缺省回退 from/phase）→ tuiv2 词表。
		to, _ := out["to"].(string)
		if to == "" {
			to, _ = out["from"].(string)
		}
		if to == "" {
			to, _ = out["phase"].(string)
		}
		out["phase"] = translatePhaseValue(strings.TrimSpace(to))
	case EventTokenUsage:
		// runtime 无 total 键：由 input/output 合成（state 层 total 优先读）。
		input, output := payloadIntValue(out, "input_tokens", "input"), payloadIntValue(out, "output_tokens", "output")
		out["total"] = input + output
	case EventPermissionRequested:
		// runtime 键 tool_name → state 消费键 tool。
		if name, ok := out["tool_name"]; ok {
			out["tool"] = name
		}
	}
	return out
}

// normalizeValue 递归归一：map 键转小写（PascalCase tools.ToolResult
// 实测序列化为 {"ToolCallID","Name","Content",...}，state 只读小写键）。
func normalizeValue(value any) any {
	typed, ok := value.(map[string]any)
	if !ok {
		return value
	}
	out := make(map[string]any, len(typed))
	for key, item := range typed {
		out[strings.ToLower(key)] = normalizeValue(item)
	}
	return out
}

// payloadIntValue 从归一化后的 map 读取整数值（兼容 float64 JSON 形态）。
func payloadIntValue(m map[string]any, keys ...string) int {
	for _, key := range keys {
		if v, ok := m[key].(float64); ok {
			return int(v)
		}
	}
	return 0
}

// debugf 输出翻译层调试日志（RealClientOptions.Debug 开启）。
func (c *RealClient) debugf(format string, args ...any) {
	if c.debug {
		log.Printf("[tuiv2-gateway] "+format, args...)
	}
}
