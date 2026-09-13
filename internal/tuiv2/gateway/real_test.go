package gateway

import (
	"context"
	"errors"
	"sync"
	"testing"

	gatewayclient "neo-code/internal/gateway/client"
)

// mockRPC 是 realRPCClient 的测试实现：记录调用并注入预设结果。
type mockRPC struct {
	mu            sync.Mutex
	authErr       error
	calls         []mockRPCCall
	results       map[string]func() // 方法名 → 注入副作用（写 result）
	notifications chan gatewayclient.Notification
	authCalls     int
	closeCalls    int
}

type mockRPCCall struct {
	method string
	params any
	result any
}

func (m *mockRPC) Authenticate(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authCalls++
	return m.authErr
}

func (m *mockRPC) Call(ctx context.Context, method string, params any, result any) error {
	m.mu.Lock()
	m.calls = append(m.calls, mockRPCCall{method: method, params: params, result: result})
	fn := m.results[method]
	m.mu.Unlock()
	if fn != nil {
		fn()
	}
	return nil
}

func (m *mockRPC) Notifications() <-chan gatewayclient.Notification {
	return m.notifications
}

func (m *mockRPC) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalls++
	return nil
}

func newMockRPC() *mockRPC {
	return &mockRPC{notifications: make(chan gatewayclient.Notification, 8)}
}

// TestNewRealClientAuthFailsFast 验证构造期 fail-fast：认证失败即返回
// 错误并释放底层客户端（不留半可用实例，对齐 v1 装配先例）。
func TestNewRealClientAuthFailsFast(t *testing.T) {
	mock := newMockRPC()
	mock.authErr = errors.New("connection refused")
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err == nil || c != nil {
		t.Fatal("auth failure should abort construction")
	}
	if mock.closeCalls != 1 {
		t.Fatalf("close calls = %d, want 1 (fail-fast release)", mock.closeCalls)
	}
}

// TestNewRealClientAuthSucceeds 验证认证通过后实例可用。
func TestNewRealClientAuthSucceeds(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	if c == nil || c.rpc == nil {
		t.Fatal("client should wrap injected rpc")
	}
	if mock.authCalls != 1 {
		t.Fatalf("auth calls = %d, want 1", mock.authCalls)
	}
}

// TestRealClientCloseIdempotent 验证 Close 幂等且透传底层释放。
func TestRealClientCloseIdempotent(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if mock.closeCalls != 1 {
		t.Fatalf("underlying close calls = %d, want 1 (close-once)", mock.closeCalls)
	}
}

// TestRealClientHealthPingsGateway 验证 Health 映射 gateway.ping。
func TestRealClientHealthPingsGateway(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	health, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if !health.OK || health.Backend != "gateway" {
		t.Fatalf("health = %+v", health)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.calls) != 1 || mock.calls[0].method != "gateway.ping" {
		t.Fatalf("calls = %+v", mock.calls)
	}
}

// TestRealClientUnwiredMethodsStayExplicit 验证分阶段实装纪律：
// 未落地方法返回 errRealNotImplemented（显式错误而非静默假成功）。
func TestRealClientUnwiredMethodsStayExplicit(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	ctx := context.Background()
	if _, err := c.ListSessions(ctx); !errors.Is(err, errRealNotImplemented) {
		t.Fatalf("ListSessions err = %v", err)
	}
	if _, err := c.SubscribeEvents(ctx, "s1"); !errors.Is(err, errRealNotImplemented) {
		t.Fatalf("SubscribeEvents err = %v", err)
	}
	if err := c.SetModel(ctx, "s1", "m1"); !errors.Is(err, errRealNotImplemented) {
		t.Fatalf("SetModel err = %v", err)
	}
}

// TestRealSubscriptionCloseOnce 验证订阅条目 close-once：三关闭路径
//（被顶替/ctx cancel/客户端 Close）重叠时不双关（S5 审计钉死项）。
func TestRealSubscriptionCloseOnce(t *testing.T) {
	s := &realSubscription{ch: make(chan GatewayEvent, 1)}
	s.close()
	s.close() // 重叠关闭不得 panic
	_, open := <-s.ch
	if open {
		t.Fatal("subscription channel should be closed")
	}
}

// 编译期锁定：RealClientOptions 复用 v1 RPC 客户端选项（ADR-004 只读复用）。
var _ = gatewayclient.GatewayRPCClientOptions{}
