package gateway

import (
	"context"
	"errors"
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
		rpc:    rpc,
		subs:   make(map[string]*realSubscription),
		closed: make(chan struct{}),
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

// ListSessions 对应 gateway.listSessions（S5 分阶段实装）。
func (c *RealClient) ListSessions(ctx context.Context) ([]SessionSummary, error) {
	return nil, errRealNotImplemented
}

// LoadSession 对应 gateway.loadSession（S5 分阶段实装）。
func (c *RealClient) LoadSession(ctx context.Context, id string) (*SessionDetail, error) {
	return nil, errRealNotImplemented
}

// CreateSession 对应 gateway.createSession（S5 分阶段实装）。
func (c *RealClient) CreateSession(ctx context.Context) (*SessionSummary, error) {
	return nil, errRealNotImplemented
}

// SendMessage 对应 gateway.run（S5 分阶段实装）。
func (c *RealClient) SendMessage(ctx context.Context, sessionID string, text string) (*RunAck, error) {
	return nil, errRealNotImplemented
}

// CancelRun 对应 gateway.cancel（S5 分阶段实装）。
func (c *RealClient) CancelRun(ctx context.Context, sessionID string, runID string) error {
	return errRealNotImplemented
}

// SubscribeEvents 对应 gateway.bindStream + gateway.event（S5 分阶段实装）。
func (c *RealClient) SubscribeEvents(ctx context.Context, sessionID string) (<-chan GatewayEvent, error) {
	return nil, errRealNotImplemented
}

// ResolvePermission 对应 gateway.resolvePermission（S5 分阶段实装）。
func (c *RealClient) ResolvePermission(ctx context.Context, decision PermissionDecision) error {
	return errRealNotImplemented
}

// AnswerUserQuestion 对应 gateway.userQuestionAnswer（S5 分阶段实装）。
func (c *RealClient) AnswerUserQuestion(ctx context.Context, answer UserQuestionAnswer) error {
	return errRealNotImplemented
}

// ListModels 对应 gateway.listModels（S5 分阶段实装）。
func (c *RealClient) ListModels(ctx context.Context) ([]ModelInfo, error) {
	return nil, errRealNotImplemented
}

// SetModel 对应 gateway.setSessionModel（S5 分阶段实装）。
func (c *RealClient) SetModel(ctx context.Context, sessionID string, modelID string) error {
	return errRealNotImplemented
}

// GetModel 对应 gateway.getSessionModel（S5 分阶段实装）。
func (c *RealClient) GetModel(ctx context.Context, sessionID string) (string, error) {
	return "", errRealNotImplemented
}
