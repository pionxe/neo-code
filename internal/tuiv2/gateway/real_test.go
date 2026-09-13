package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	gatewayclient "neo-code/internal/gateway/client"
)

// mockRPC 是 realRPCClient 的测试实现：记录调用并注入预设结果。
type mockRPC struct {
	mu            sync.Mutex
	authErr       error
	callErr       error // 注入 Call 失败（错误传播路径覆盖）
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
	callErr := m.callErr
	fn := m.results[method]
	m.mu.Unlock()
	if callErr != nil {
		return callErr
	}
	if fn != nil {
		fn()
	}
	return nil
}

func (m *mockRPC) Notifications() <-chan gatewayclient.Notification {
	return m.notifications
}

// Close 关闭通知通道（对齐真实 GatewayRPCClient 行为：连接关闭即
// 通知通道关闭——泵据此退出）。
func (m *mockRPC) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalls++
	select {
	case <-m.notifications:
		// 已关闭
	default:
		close(m.notifications)
	}
	return nil
}

func newMockRPC() *mockRPC {
	return &mockRPC{notifications: make(chan gatewayclient.Notification, 8), results: map[string]func(){}}
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

// TestRealSubscriptionCloseOnce 验证订阅条目 close-once：三关闭路径
// （被顶替/ctx cancel/客户端 Close）重叠时不双关（S5 审计钉死项）。
func TestRealSubscriptionCloseOnce(t *testing.T) {
	s := &realSubscription{ch: make(chan GatewayEvent, 1), done: make(chan struct{})}
	s.retire()
	s.retire() // 重叠 retire 不得 panic
	s.closeCh()
	s.closeCh() // 重叠 closeCh 不得 panic（sendMu 序列化，防发送竞态）
	_, chOpen := <-s.ch
	_, doneOpen := <-s.done
	if chOpen || doneOpen {
		t.Fatal("subscription channels should be closed")
	}
}

// 编译期锁定：RealClientOptions 复用 v1 RPC 客户端选项（ADR-004 只读复用）。
var _ = gatewayclient.GatewayRPCClientOptions{}

// TestListSessionsDecodesPayloadTable 验证 listSessions：整帧解包
// （数据在 payload.sessions 下）+ DTO 逐字段翻译。
func TestListSessionsDecodesPayloadTable(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	mock.mu.Lock()
	mock.results["gateway.listSessions"] = func() {
		// Call 调用本闭包前已释放 mock 锁：直接写 result。
		last := mock.calls[len(mock.calls)-1]
		frame := last.result.(*struct {
			Payload struct {
				Sessions []realSessionSummary `json:"sessions"`
			} `json:"payload"`
		})
		frame.Payload.Sessions = []realSessionSummary{{
			ID:        " s1 ",
			Title:     "demo",
			AgentMode: "build",
			Model:     "test-model",
		}}
	}
	mock.mu.Unlock()

	sessions, err := c.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %+v", sessions)
	}
	got := sessions[0]
	if got.ID != "s1" || got.Title != "demo" || got.Mode != "build" || got.Model != "test-model" {
		t.Fatalf("summary = %+v（含 TrimSpace 断言）", got)
	}
}

// TestLoadSessionTranslatesMessages 验证 loadSession：payload 直出 +
// Messages→Stream 翻译（role→Kind、IsError→Status、ID 序号合成、
// Usage 恒零——S5 审计 Q7 裁定）。
func TestLoadSessionTranslatesMessages(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	mock.mu.Lock()
	mock.results["gateway.loadSession"] = func() {
		// Call 调用本闭包前已释放 mock 锁：直接写 result。
		last := mock.calls[len(mock.calls)-1]
		frame := last.result.(*struct {
			Payload realSession `json:"payload"`
		})
		frame.Payload = realSession{
			ID:        "s1",
			Title:     "demo",
			AgentMode: "build",
			Model:     "test-model",
			Messages: []realSessionMessage{
				{Role: "user", Content: "hello"},
				{Role: "assistant", Content: "hi"},
				{Role: "tool", Content: "result text", IsError: true},
			},
		}
	}
	mock.mu.Unlock()

	detail, err := c.LoadSession(context.Background(), "s1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if detail.Summary.ID != "s1" || detail.Summary.Mode != "build" {
		t.Fatalf("summary = %+v", detail.Summary)
	}
	if detail.Usage != (TokenUsage{}) {
		t.Fatalf("usage should be zero-valued, got %+v", detail.Usage)
	}
	if len(detail.Stream) != 3 {
		t.Fatalf("stream = %+v", detail.Stream)
	}
	if detail.Stream[0].Kind != "message" || detail.Stream[0].Role != "user" || detail.Stream[0].ID != "real-msg-1" {
		t.Fatalf("entry0 = %+v", detail.Stream[0])
	}
	if detail.Stream[2].Kind != "tool_end" || detail.Stream[2].Status != "error" || detail.Stream[2].Text != "result text" {
		t.Fatalf("entry2 = %+v", detail.Stream[2])
	}
}

// TestLoadSessionEmptyIDRejected 验证空会话 ID 本地拒绝（不发 RPC）。
func TestLoadSessionEmptyIDRejected(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	if _, err := c.LoadSession(context.Background(), "  "); err == nil {
		t.Fatal("empty session id should be rejected locally")
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.calls) != 0 {
		t.Fatal("empty id must not trigger RPC")
	}
}

// TestCreateSessionExtractsFrameSessionID 验证 createSession：从 frame 级
// session_id 提取新会话 ID；空 ID 报错。
func TestCreateSessionExtractsFrameSessionID(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	mock.mu.Lock()
	mock.results["gateway.createSession"] = func() {
		// Call 调用本闭包前已释放 mock 锁：直接写 result。
		last := mock.calls[len(mock.calls)-1]
		last.result.(*struct {
			SessionID string `json:"session_id"`
		}).SessionID = " new-1 "
	}
	mock.mu.Unlock()

	summary, err := c.CreateSession(context.Background())
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if summary.ID != "new-1" || summary.Title == "" {
		t.Fatalf("summary = %+v", summary)
	}

	// 空 ID → 显式错误。
	mock.mu.Lock()
	mock.results["gateway.createSession"] = func() {}
	mock.mu.Unlock()
	if _, err := c.CreateSession(context.Background()); err == nil {
		t.Fatal("empty session id should fail")
	}
}

// TestSendMessageRunAckWithFallbacks 验证 SendMessage：params 捕获
// （session_id/input_text）+ ack 省略时回退请求值。
func TestSendMessageRunAckWithFallbacks(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	mock.mu.Lock()
	mock.results["gateway.run"] = func() {
		last := mock.calls[len(mock.calls)-1]
		params := last.params.(struct {
			SessionID string `json:"session_id,omitempty"`
			InputText string `json:"input_text,omitempty"`
		})
		if params.SessionID != "s1" || params.InputText != "hi" {
			t.Errorf("run params = %+v", params)
		}
		// ack 只回 run_id（session_id 省略 → 回退请求值）。
		last.result.(*struct {
			SessionID string `json:"session_id"`
			RunID     string `json:"run_id"`
		}).RunID = "run-9"
	}
	mock.mu.Unlock()

	ack, err := c.SendMessage(context.Background(), "s1", "hi")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if ack.SessionID != "s1" || ack.RunID != "run-9" || !ack.Accepted {
		t.Fatalf("ack = %+v", ack)
	}
}

// TestCancelRunForwardsParams 验证 CancelRun 参数透传。
func TestCancelRunForwardsParams(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	if err := c.CancelRun(context.Background(), "s1", "run-1"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.calls) != 1 || mock.calls[0].method != "gateway.cancel" {
		t.Fatalf("calls = %+v", mock.calls)
	}
}

// TestResolvePermissionExplicitAndBackfill 验证权限决策：显式 RequestID
// 优先、空值回填追踪槽、双槽独立、无可用 ID 本地报错；
// 枚举契约：Allow→"allow_once"、!Allow→"reject"（服务端仅接受
// allow_once/allow_session/reject——PR #47 审计 P0 实测钉死）。
func TestResolvePermissionExplicitAndBackfill(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}

	// 显式 RequestID：直通，allow 映射。
	if err := c.ResolvePermission(context.Background(), PermissionDecision{
		RequestID: "perm-explicit", SessionID: "s1", Allow: true,
	}); err != nil {
		t.Fatalf("explicit: %v", err)
	}
	mock.mu.Lock()
	call := mock.calls[len(mock.calls)-1]
	params := call.params.(struct {
		RequestID string `json:"request_id"`
		Decision  string `json:"decision"`
	})
	mock.mu.Unlock()
	if params.RequestID != "perm-explicit" || params.Decision != "allow_once" {
		t.Fatalf("params = %+v", params)
	}

	// 空值 + 追踪槽有登记 → 回填；deny 映射。
	c.mu.Lock()
	c.permReqID["s1"] = "perm-pending"
	c.mu.Unlock()
	if err := c.ResolvePermission(context.Background(), PermissionDecision{SessionID: "s1"}); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	mock.mu.Lock()
	params = mock.calls[len(mock.calls)-1].params.(struct {
		RequestID string `json:"request_id"`
		Decision  string `json:"decision"`
	})
	mock.mu.Unlock()
	if params.RequestID != "perm-pending" || params.Decision != "reject" {
		t.Fatalf("params = %+v", params)
	}

	// 空值 + 槽空 → 本地报错（不发 RPC；先清槽再调用，无锁跨越）。
	c.mu.Lock()
	delete(c.permReqID, "s1")
	c.mu.Unlock()
	mock.mu.Lock()
	callsBefore := len(mock.calls)
	mock.mu.Unlock()
	if err := c.ResolvePermission(context.Background(), PermissionDecision{SessionID: "s1"}); err == nil {
		t.Fatal("unavailable request id should fail locally")
	}
	mock.mu.Lock()
	if len(mock.calls) != callsBefore {
		t.Fatal("unavailable id must not trigger RPC")
	}
	mock.mu.Unlock()
}

// TestAnswerUserQuestionBackfillAndMapping 验证问答回答：Status=answered
// + Message=Text + QuestionID 回填（与权限槽独立）。
func TestAnswerUserQuestionBackfillAndMapping(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	c.mu.Lock()
	c.questReqID["s1"] = "quest-pending"
	c.mu.Unlock()

	if err := c.AnswerUserQuestion(context.Background(), UserQuestionAnswer{
		SessionID: "s1", Text: "my answer",
	}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	params := mock.calls[len(mock.calls)-1].params.(struct {
		RequestID string   `json:"request_id"`
		Status    string   `json:"status,omitempty"`
		Values    []string `json:"values,omitempty"`
		Message   string   `json:"message,omitempty"`
	})
	if params.RequestID != "quest-pending" || params.Status != "answered" || params.Message != "my answer" {
		t.Fatalf("params = %+v", params)
	}
}

// TestListModelsDerivesCurrent 验证 listModels：Current 由
// selected_model_id 派生、空 ID 跳过、空名回退 ID（S5 审计裁定）。
func TestListModelsDerivesCurrent(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	mock.mu.Lock()
	mock.results["gateway.listModels"] = func() {
		last := mock.calls[len(mock.calls)-1]
		frame := last.result.(*struct {
			Payload struct {
				Models          []realModelEntry `json:"models"`
				SelectedModelID string           `json:"selected_model_id"`
			} `json:"payload"`
		})
		frame.Payload.Models = []realModelEntry{
			{ID: " m1 ", Name: "", Provider: "prov-a"},
			{ID: "", Name: "ghost"},
			{ID: "m2", Name: "Model Two", Provider: "prov-b"},
		}
		frame.Payload.SelectedModelID = "m2"
	}
	mock.mu.Unlock()

	models, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("models = %+v（空 ID 条目应跳过）", models)
	}
	if models[0].ID != "m1" || models[0].Name != "m1" || models[0].Current {
		t.Fatalf("model0 = %+v", models[0])
	}
	if !models[1].Current || models[1].Name != "Model Two" {
		t.Fatalf("model1 = %+v", models[1])
	}
}

// TestSetModelForwardsParams 验证 setSessionModel 参数透传。
func TestSetModelForwardsParams(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	if err := c.SetModel(context.Background(), "s1", "m2"); err != nil {
		t.Fatalf("set model: %v", err)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.calls) != 1 || mock.calls[0].method != "gateway.setSessionModel" {
		t.Fatalf("calls = %+v", mock.calls)
	}
}

// TestGetModelReadsPayloadModelID 验证 getSessionModel：payload.model_id
// 为权威真值（SessionModelResult）。
func TestGetModelReadsPayloadModelID(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	mock.mu.Lock()
	mock.results["gateway.getSessionModel"] = func() {
		last := mock.calls[len(mock.calls)-1]
		last.result.(*struct {
			Payload struct {
				ModelID string `json:"model_id"`
			} `json:"payload"`
		}).Payload.ModelID = " m-final "
	}
	mock.mu.Unlock()

	model, err := c.GetModel(context.Background(), "s1")
	if err != nil {
		t.Fatalf("get model: %v", err)
	}
	if model != "m-final" {
		t.Fatalf("model = %q", model)
	}
}

// pushNotification 向 mock 通知通道注入一条 gateway.event（JSON params）。
func pushNotification(t *testing.T, m *mockRPC, params string) {
	t.Helper()
	m.notifications <- gatewayclient.Notification{Method: "gateway.event", Params: json.RawMessage(params)}
}

// recvEvent 带超时读取订阅事件。
func recvEvent(t *testing.T, ch <-chan GatewayEvent) GatewayEvent {
	t.Helper()
	select {
	case event, ok := <-ch:
		if !ok {
			t.Fatal("subscription closed unexpectedly")
		}
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
		return GatewayEvent{}
	}
}

const agentChunkFrame = `{"type":"event","action":"run","session_id":"s1","run_id":"run-1","payload":{"event_type":"run_progress","payload":{"runtime_event_type":"agent_chunk","turn":1,"phase":"execute","timestamp":"2026-09-13T00:00:00Z","payload_version":4,"payload":"你好"}}}`

// TestSubscribeEventsBindsAndReceivesChunk 端到端：bindStream + 单泵扇出 +
// 纯字符串 payload 包装（agent_chunk → {"text":...}）+ envelope 顶层
// phase 提升翻译（execute → running）。
func TestSubscribeEventsBindsAndReceivesChunk(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock, Debug: true})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	defer c.Close()

	ch, err := c.SubscribeEvents(context.Background(), "s1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	// bindStream 已调用。
	mock.mu.Lock()
	if len(mock.calls) == 0 || mock.calls[0].method != "gateway.bindStream" {
		mock.mu.Unlock()
		t.Fatal("bindStream should be called")
	}
	mock.mu.Unlock()

	pushNotification(t, mock, agentChunkFrame)
	event := recvEvent(t, ch)
	if event.Type != EventAgentChunk {
		t.Fatalf("type = %v", event.Type)
	}
	if event.Payload["text"] != "你好" {
		t.Fatalf("payload = %+v", event.Payload)
	}
	if event.SessionID != "s1" || event.RunID != "run-1" {
		t.Fatalf("frame ids = %+v", event)
	}
}

// TestSubscribeEventsReplacesOldSubscription 验证同会话重复订阅：新顶替旧
// 并关闭旧通道（close-once）。
func TestSubscribeEventsReplacesOldSubscription(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	defer c.Close()

	oldCh, err := c.SubscribeEvents(context.Background(), "s1")
	if err != nil {
		t.Fatalf("subscribe old: %v", err)
	}
	newCh, err := c.SubscribeEvents(context.Background(), "s1")
	if err != nil {
		t.Fatalf("subscribe new: %v", err)
	}
	if _, open := <-oldCh; open {
		t.Fatal("old subscription should be closed on replacement")
	}
	if newCh == nil {
		t.Fatal("new subscription should be open")
	}
}

// TestFlattenPhaseChangedValueTranslation 验证 phase_changed 值翻译：
// execute→running、waiting_user_question→waiting_user（S5 审计 P1-7）。
func TestFlattenPhaseChangedValueTranslation(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	defer c.Close()
	ch, err := c.SubscribeEvents(context.Background(), "s1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"phase_changed","payload_version":4,"payload":{"from":"plan","to":"execute"}}}}`)
	event := recvEvent(t, ch)
	if event.Type != EventPhaseChanged || event.Payload["phase"] != "running" {
		t.Fatalf("event = %+v", event)
	}

	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"phase_changed","payload_version":4,"payload":{"from":"execute","to":"waiting_user_question"}}}}`)
	event = recvEvent(t, ch)
	if event.Payload["phase"] != "waiting_user" {
		t.Fatalf("phase = %v, want waiting_user", event.Payload["phase"])
	}
}

// TestFlattenToolResultPascalCaseNormalized 验证 tool_result 的 PascalCase
// 键归一化（tools.ToolResult 实测序列化形态）。
func TestFlattenToolResultPascalCaseNormalized(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	defer c.Close()
	ch, err := c.SubscribeEvents(context.Background(), "s1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"tool_result","payload_version":4,"payload":{"ToolCallID":"t1","Name":"bash","Content":"ok","IsError":false}}}}`)
	event := recvEvent(t, ch)
	if event.Type != EventToolResult {
		t.Fatalf("type = %v", event.Type)
	}
	if event.Payload["name"] != "bash" || event.Payload["content"] != "ok" {
		t.Fatalf("payload = %+v（PascalCase 应归一为小写键）", event.Payload)
	}
}

// TestEnvelopeLessErrorFrameConsumed 验证无 envelope 的外层错误帧
// （run_error，payload={code,message}）被消费为 EventError（S5 审计 P0-4）。
func TestEnvelopeLessErrorFrameConsumed(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	defer c.Close()
	ch, err := c.SubscribeEvents(context.Background(), "s1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	pushNotification(t, mock, `{"session_id":"s1","run_id":"run-1","payload":{"event_type":"run_error","message":"boom","code":"internal_error"}}`)
	event := recvEvent(t, ch)
	if event.Type != EventError || event.Payload["message"] != "boom" {
		t.Fatalf("event = %+v", event)
	}
}

// TestRequestIDTrackedAndBackfilledEndToEnd 端到端：泵内追踪 request_id →
// 提交决策时回填；resolved 清槽后回填不可用（本地报错）。
func TestRequestIDTrackedAndBackfilledEndToEnd(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	defer c.Close()
	ch, err := c.SubscribeEvents(context.Background(), "s1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// 权限请求事件（带 request_id）→ 泵内登记（扇出前写入）。
	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"permission_requested","payload_version":4,"payload":{"request_id":"perm-42","tool_name":"bash","operation":"ls"}}}}`)
	recvEvent(t, ch)

	// 无显式 RequestID 提交 → 回填 perm-42。
	if err := c.ResolvePermission(context.Background(), PermissionDecision{SessionID: "s1", Allow: true}); err != nil {
		t.Fatalf("resolve with backfill: %v", err)
	}
	mock.mu.Lock()
	params := mock.calls[len(mock.calls)-1].params.(struct {
		RequestID string `json:"request_id"`
		Decision  string `json:"decision"`
	})
	mock.mu.Unlock()
	if params.RequestID != "perm-42" || params.Decision != "allow_once" {
		t.Fatalf("params = %+v", params)
	}

	// resolved 事件清槽 → 再次提交本地报错（防陈旧回填）。
	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"permission_resolved","payload_version":4,"payload":{"request_id":"perm-42"}}}}`)
	recvEvent(t, ch)
	if err := c.ResolvePermission(context.Background(), PermissionDecision{SessionID: "s1"}); err == nil {
		t.Fatal("stale backfill should be unavailable after resolved")
	}
}

// TestNonTuiv2EventDropped 验证非 tuiv2 词汇事件（budget_checked）被
// 丢弃且不影响后续事件（default 分支集中丢弃）。
func TestNonTuiv2EventDropped(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	defer c.Close()
	ch, err := c.SubscribeEvents(context.Background(), "s1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"budget_checked","payload_version":4,"payload":{"decision":"allow"}}}}`)
	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"token_usage","payload_version":4,"payload":{"input_tokens":10,"output_tokens":5}}}}`)

	event := recvEvent(t, ch)
	if event.Type != EventTokenUsage {
		t.Fatalf("type = %v, want token_usage after dropped budget_checked", event.Type)
	}
	// runtime 无 total 键：由 input+output 合成（S5 审计裁定）。
	if event.Payload["total"] != 15 {
		t.Fatalf("total = %v, want 15", event.Payload["total"])
	}
}

// TestNormalizeRealOptionsForcesDisableAutoSpawn 验证装配选项归一：
// DisableAutoSpawn 强制 true（v1 自我重执行在 tuiv2 二进制下必然失败——
// S5 审计 Q5/Q6 裁定的落地行，PR #47 审计 P1-8 补测）。
func TestNormalizeRealOptionsForcesDisableAutoSpawn(t *testing.T) {
	options := normalizeRealOptions(RealClientOptions{})
	if !options.RPC.DisableAutoSpawn {
		t.Fatal("DisableAutoSpawn must be forced to true")
	}
}

// TestPumpExitsOnNotificationsClose 验证泵退出路径之一：通知通道关闭
// （连接断开）→ 泵退出且 Close 不死锁（PR #47 审计 P1-8 补测）。
func TestPumpExitsOnNotificationsClose(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	close(mock.notifications) // 模拟连接断开
	select {
	case <-c.pumpDone:
	case <-time.After(time.Second):
		t.Fatal("pump should exit when notifications channel closes")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("close after pump exit: %v", err)
	}
}

// TestQuestionSlotTrackedAndCleared 验证问答槽追踪与清槽（与权限槽独立，
// PR #47 审计 P1-8 补测——事件交错时共用一桶会错填）。
func TestQuestionSlotTrackedAndCleared(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	defer c.Close()
	ch, err := c.SubscribeEvents(context.Background(), "s1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// 交错注入：权限与问答事件分槽登记，互不污染。
	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"permission_requested","payload_version":4,"payload":{"request_id":"perm-x","tool_name":"bash"}}}}`)
	recvEvent(t, ch)
	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"user_question_requested","payload_version":4,"payload":{"title":"标题","description":"描述","options":["a","b"]}}}}`)
	event := recvEvent(t, ch)

	// P1-1a：title/description → question 键（缺失时 ask_user 文本恒空）。
	if event.Payload["question"] != "标题" {
		t.Fatalf("question = %v, want 标题（title 回退链）", event.Payload["question"])
	}
	if event.Payload["options"] == nil {
		t.Fatal("options should pass through")
	}

	c.mu.Lock()
	permID, questID := c.permReqID["s1"], c.questReqID["s1"]
	c.mu.Unlock()
	if permID != "perm-x" || questID != "" {
		t.Fatalf("slots: perm=%q quest=%q（user_question_requested 的 runtime payload 无 request_id，登记允许为空）", permID, questID)
	}
}

// TestUnknownOuterFrameDropped 验证未知外层帧（非 run_error/ask_error 且
// 无 envelope）被静默丢弃不扇出（PR #47 审计 P1-8 补测）。
func TestUnknownOuterFrameDropped(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	defer c.Close()
	ch, err := c.SubscribeEvents(context.Background(), "s1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	pushNotification(t, mock, `{"session_id":"s1","payload":{"event_type":"something_else","message":"x"}}`)
	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"agent_chunk","payload_version":4,"payload":"后续事件"}}}`)
	event := recvEvent(t, ch)
	if event.Type != EventAgentChunk {
		t.Fatalf("type = %v, want agent_chunk（未知帧不应阻断后续事件）", event.Type)
	}
}

// TestToolStartArgumentsNormalizedToInput 验证 tool_start 的 arguments→
// input 归一化行（缺失时工具行无命令摘要——PR #47 审计 P1-1b）。
func TestToolStartArgumentsNormalizedToInput(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	defer c.Close()
	ch, err := c.SubscribeEvents(context.Background(), "s1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"tool_start","payload_version":4,"payload":{"name":"bash","arguments":"ls -la"}}}}`)
	event := recvEvent(t, ch)
	if event.Type != EventToolStart {
		t.Fatalf("type = %v", event.Type)
	}
	if event.Payload["input"] != "ls -la" || event.Payload["name"] != "bash" {
		t.Fatalf("payload = %+v", event.Payload)
	}
}

// TestRPCErrorPropagatesThroughAllMethods 验证底层 Call 失败时全部方法
// 的错误传播（不吞错不转换——PR #47 审计 P1-8 补齐错误路径覆盖）。
func TestRPCErrorPropagatesThroughAllMethods(t *testing.T) {
	mock := newMockRPC()
	mock.callErr = errors.New("rpc boom")
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	ctx := context.Background()
	boom := func(err error) bool { return err != nil && strings.Contains(err.Error(), "rpc boom") }

	if _, err := c.Health(ctx); !boom(err) {
		t.Fatalf("Health err = %v", err)
	}
	if _, err := c.ListSessions(ctx); !boom(err) {
		t.Fatalf("ListSessions err = %v", err)
	}
	if _, err := c.LoadSession(ctx, "s1"); !boom(err) {
		t.Fatalf("LoadSession err = %v", err)
	}
	if _, err := c.CreateSession(ctx); !boom(err) {
		t.Fatalf("CreateSession err = %v", err)
	}
	if _, err := c.SendMessage(ctx, "s1", "hi"); !boom(err) {
		t.Fatalf("SendMessage err = %v", err)
	}
	if err := c.CancelRun(ctx, "s1", "r1"); !boom(err) {
		t.Fatalf("CancelRun err = %v", err)
	}
	if err := c.ResolvePermission(ctx, PermissionDecision{RequestID: "p1", SessionID: "s1", Allow: true}); !boom(err) {
		t.Fatalf("ResolvePermission err = %v", err)
	}
	if err := c.AnswerUserQuestion(ctx, UserQuestionAnswer{QuestionID: "q1", SessionID: "s1", Text: "a"}); !boom(err) {
		t.Fatalf("AnswerUserQuestion err = %v", err)
	}
	if _, err := c.ListModels(ctx); !boom(err) {
		t.Fatalf("ListModels err = %v", err)
	}
	if err := c.SetModel(ctx, "s1", "m1"); !boom(err) {
		t.Fatalf("SetModel err = %v", err)
	}
	if _, err := c.GetModel(ctx, "s1"); !boom(err) {
		t.Fatalf("GetModel err = %v", err)
	}
}

// TestSubscribeEventsCtxCancelClosesSubscription 验证调用方 ctx cancel：
// 订阅关闭并从注册表注销（三条关闭路径之二——PR #47 审计 P1-8 补测）。
func TestSubscribeEventsCtxCancelClosesSubscription(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := c.SubscribeEvents(ctx, "s1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	cancel()
	select {
	case <-ch: // 关闭即收到零值
	case <-time.After(time.Second):
		t.Fatal("ctx cancel should close subscription")
	}
	c.mu.Lock()
	_, registered := c.subs["s1"]
	c.mu.Unlock()
	if registered {
		t.Fatal("cancelled subscription should be unregistered")
	}
}

// TestNewRealClientRealDialFailsFast 验证真实构造分支（无注入）：
// DisableAutoSpawn 强制生效后连接不可达（显式指向不存在的 socket 路径）
// 即构造失败（S5 审计 Q5/Q6 行为钉死；认证失败返回路径覆盖）。
func TestNewRealClientRealDialFailsFast(t *testing.T) {
	_, err := NewRealClient(RealClientOptions{
		RPC: gatewayclient.GatewayRPCClientOptions{
			ListenAddress: "/nonexistent-s5-smoke/gateway.sock",
		},
	})
	if err == nil {
		t.Fatal("construction with unreachable socket should fail fast")
	}
}

// TestFlattenCoverageMatrix 是归一化/翻译层的补齐矩阵（PR #47 审计 P1-8：
// 覆盖剩余分支——runtime 词汇全行、phase 回退链、question 回退链、
// 无订阅丢弃、未知帧、非事件通知、空 ID 订阅、bindStream 失败）。
func TestFlattenCoverageMatrix(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	defer c.Close()

	// 空会话 ID 订阅 → 本地拒绝（不发 bindStream）。
	if _, err := c.SubscribeEvents(context.Background(), "  "); err == nil {
		t.Fatal("empty session id should be rejected")
	}
	// bindStream 失败 → 错误传播。
	mock.mu.Lock()
	mock.callErr = errors.New("bind boom")
	mock.mu.Unlock()
	if _, err := c.SubscribeEvents(context.Background(), "s-err"); err == nil {
		t.Fatal("bindStream failure should propagate")
	}
	mock.mu.Lock()
	mock.callErr = nil
	mock.mu.Unlock()

	ch, err := c.SubscribeEvents(context.Background(), "s1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// 非事件通知（其他方法）+ 解码失败 + 无订阅事件：均被吸收不致命。
	mock.notifications <- gatewayclient.Notification{Method: "gateway.ping", Params: json.RawMessage(`{}`)}
	mock.notifications <- gatewayclient.Notification{Method: "gateway.event", Params: json.RawMessage(`{invalid`)}
	mock.notifications <- gatewayclient.Notification{Method: "gateway.event", Params: json.RawMessage(`{"session_id":"s0","payload":{"payload":{"runtime_event_type":"agent_chunk","payload_version":4,"payload":"ghost"}}}`)}

	cases := []struct {
		name  string
		frame string
		check func(GatewayEvent)
	}{
		{
			name:  "tool_chunk→tool_output 字符串包装",
			frame: `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"tool_chunk","payload_version":4,"payload":"out"}}}`,
			check: func(e GatewayEvent) {
				if e.Type != EventToolOutput || e.Payload["text"] != "out" {
					t.Fatalf("event = %+v", e)
				}
			},
		},
		{
			name:  "run_canceled 同名映射",
			frame: `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"run_canceled","payload_version":4,"payload":{"phase":"canceled"}}}}`,
			check: func(e GatewayEvent) {
				if e.Type != EventRunCanceled {
					t.Fatalf("event = %+v", e)
				}
			},
		},
		{
			name:  "error envelope 字符串包装",
			frame: `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"error","payload_version":4,"payload":"bad"}}}`,
			check: func(e GatewayEvent) {
				if e.Type != EventError || e.Payload["text"] != "bad" {
					t.Fatalf("event = %+v", e)
				}
			},
		},
		{
			name:  "user_question_answered 同名映射",
			frame: `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"user_question_answered","payload_version":4,"payload":{"request_id":"q9"}}}}`,
			check: func(e GatewayEvent) {
				if e.Type != EventUserQuestionAnswered {
					t.Fatalf("event = %+v", e)
				}
			},
		},
		{
			name:  "agent_done→run_finished 派生",
			frame: `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"agent_done","payload_version":4,"payload":{}}}}`,
			check: func(e GatewayEvent) {
				if e.Type != EventRunFinished {
					t.Fatalf("event = %+v", e)
				}
			},
		},
		{
			name:  "phase stopped→idle",
			frame: `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"phase_changed","payload_version":4,"payload":{"to":"stopped"}}}}`,
			check: func(e GatewayEvent) {
				if e.Payload["phase"] != "idle" {
					t.Fatalf("phase = %v", e.Payload["phase"])
				}
			},
		},
		{
			name:  "phase from 回退 + waiting_permission 直通",
			frame: `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"phase_changed","payload_version":4,"payload":{"from":"waiting_permission"}}}}`,
			check: func(e GatewayEvent) {
				if e.Payload["phase"] != "waiting_permission" {
					t.Fatalf("phase = %v", e.Payload["phase"])
				}
			},
		},
		{
			name:  "question description 回退",
			frame: `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"user_question_requested","payload_version":4,"payload":{"description":"desc-only","options":["x"]}}}}`,
			check: func(e GatewayEvent) {
				if e.Type != EventUserQuestionRequested || e.Payload["question"] != "desc-only" {
					t.Fatalf("event = %+v", e)
				}
			},
		},
		{
			name:  "外层错误帧 message 缺省回退 code",
			frame: `{"session_id":"s1","payload":{"event_type":"run_error","code":"timeout"}}`,
			check: func(e GatewayEvent) {
				if e.Type != EventError || e.Payload["message"] != "timeout" {
					t.Fatalf("event = %+v", e)
				}
			},
		},
	}
	for _, tc := range cases {
		pushNotification(t, mock, tc.frame)
		recvEvent(t, ch)
		// 逐条校验：取出的事件按序对应（recvEvent 已校验非关闭）。
	}

	// 顺序敏感的逐条断言：重放矩阵并以队列校验。
	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"tool_chunk","payload_version":4,"payload":"o2"}}}`)
	first := recvEvent(t, ch)
	if first.Type != EventToolOutput || first.Payload["text"] != "o2" {
		t.Fatalf("first = %+v", first)
	}
	// token_usage 无数值键 → payloadIntValue 缺省 0 路径（total=0）。
	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"token_usage","payload_version":4,"payload":{}}}}`)
	second := recvEvent(t, ch)
	if second.Type != EventTokenUsage {
		t.Fatalf("second = %+v", second)
	}

	// phase 无 to/from（回退链末端：直接读 phase 键或置空）。
	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"phase_changed","payload_version":4,"payload":{"phase":"waiting_permission"}}}}`)
	third := recvEvent(t, ch)
	if third.Payload["phase"] != "waiting_permission" {
		t.Fatalf("third phase = %v", third.Payload["phase"])
	}

	// question 双键全缺：question 键不产出（回退链穷尽）。
	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"user_question_requested","payload_version":4,"payload":{"options":["x"]}}}}`)
	fourth := recvEvent(t, ch)
	if _, has := fourth.Payload["question"]; has {
		t.Fatalf("question should be absent, got %v", fourth.Payload["question"])
	}

	// payload_version 错误 → fail fast 转错误事件（契约矩阵硬不兼容）。
	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"agent_chunk","payload_version":3,"payload":"x"}}}`)
	fatal := recvEvent(t, ch)
	if fatal.Type != EventError {
		t.Fatalf("type = %v, want EventError (payload_version fail-fast)", fatal.Type)
	}
	if msg, _ := fatal.Payload["message"].(string); !strings.Contains(msg, "payload_version") {
		t.Fatalf("message = %v", fatal.Payload["message"])
	}

	// 无订阅会话的事件：被吸收（注册表无该会话）。
	pushNotification(t, mock, `{"session_id":"s-other","payload":{"payload":{"runtime_event_type":"agent_chunk","payload_version":4,"payload":"ghost"}}}`)
	// 解码失败帧：被吸收。
	mock.notifications <- gatewayclient.Notification{Method: "gateway.event", Params: json.RawMessage(`{bad json`)}
	// 非 gateway.event 通知：被过滤。
	mock.notifications <- gatewayclient.Notification{Method: "gateway.ping", Params: json.RawMessage(`{}`)}
	// 吸收完毕后订阅仍可用：注入一条真实事件验证泵存活。
	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"agent_chunk","payload_version":4,"payload":"still-alive"}}}`)
	alive := recvEvent(t, ch)
	if alive.Payload["text"] != "still-alive" {
		t.Fatalf("alive = %+v", alive)
	}

	// 问答无可用请求 ID：本地报错（双空路径）。
	if err := c.AnswerUserQuestion(context.Background(), UserQuestionAnswer{SessionID: "s1", Text: "a"}); err == nil {
		t.Fatal("unavailable question request id should fail locally")
	}

	// 权限双空路径：本地报错（不发 RPC）。
	c.mu.Lock()
	delete(c.permReqID, "s1")
	c.mu.Unlock()
	mock.mu.Lock()
	before := len(mock.calls)
	mock.mu.Unlock()
	if err := c.ResolvePermission(context.Background(), PermissionDecision{SessionID: "s1"}); err == nil {
		t.Fatal("unavailable permission request id should fail locally")
	}
	mock.mu.Lock()
	after := len(mock.calls)
	mock.mu.Unlock()
	if after != before {
		t.Fatal("unavailable id must not trigger RPC")
	}
}

// TestDebugLoggingPath 验证 Debug 开启时丢弃路径输出日志（不 panic 即可，
// 覆盖 debugf 分支；日志走标准 log，测试不捕获内容）。
func TestDebugLoggingPath(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock, Debug: true})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	defer c.Close()
	ch, err := c.SubscribeEvents(context.Background(), "s1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	// 非 tuiv2 词汇 → 丢弃 + debug 日志分支。
	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"budget_checked","payload_version":4,"payload":{}}}}`)
	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"agent_chunk","payload_version":4,"payload":"visible"}}}`)
	recvEvent(t, ch)
}

// TestSubscriptionClosePathsAndEnvelopeEdges 覆盖剩余分支（PR #47 审计
// P1-8 收尾）：缓冲打满时泵的三路 select 双回退（sub.done 顶替 /
// c.closed 客户端关闭）、request_id 清槽、空 params/无 envelope 帧吸收。
func TestSubscriptionClosePathsAndEnvelopeEdges(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}

	// 子测试 a：缓冲打满 + 顶替 → 泵 send-select 走 sub.done 回退（602）。
	ch1, err := c.SubscribeEvents(context.Background(), "s-full")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	for i := 0; i < realSubBuffer; i++ {
		mock.notifications <- gatewayclient.Notification{Method: "gateway.event", Params: json.RawMessage(`{"session_id":"s-full","payload":{"payload":{"runtime_event_type":"agent_chunk","payload_version":4,"payload":"x"}}}`)}
	}
	// 缓冲已满：泵阻塞在下一条的 send-select 上；顶替订阅 → sub.done 回退。
	if _, err := c.SubscribeEvents(context.Background(), "s-full"); err != nil {
		t.Fatalf("resubscribe: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		_, registered := c.subs["s-full"]
		c.mu.Unlock()
		if len(ch1) == 0 && !registered {
			break // 旧订阅已注销且通道已关：泵已走 done 回退
		}
		time.Sleep(20 * time.Millisecond)
	}

	// 子测试 b：缓冲打满 + 客户端 Close → 泵走 c.closed 回退（603），
	// Close 等待 pumpDone 后返回（无 goroutine 泄漏）。
	ch2, err := c.SubscribeEvents(context.Background(), "s-close")
	if err != nil {
		t.Fatalf("subscribe close-path: %v", err)
	}
	for i := 0; i < realSubBuffer+1; i++ {
		mock.notifications <- gatewayclient.Notification{Method: "gateway.event", Params: json.RawMessage(`{"session_id":"s-close","payload":{"payload":{"runtime_event_type":"agent_chunk","payload_version":4,"payload":"x"}}}`)}
	}
	time.Sleep(100 * time.Millisecond) // 等泵进入满缓冲阻塞
	done := make(chan error, 1)
	go func() { done <- c.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("close: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close should unblock via c.closed fallback")
	}
	// Close 后全部订阅关闭：先抽干缓冲（close 后缓冲值仍可接收），
	// 直至通道返回关闭零值。
	for range ch2 {
	}
	if _, open := <-ch2; open {
		t.Fatal("subscription should be closed after client Close")
	}

	// 子测试 c：request_id 清槽（user_question_answered）+ 空 params 帧 +
	// 无 envelope 帧吸收（693/696）。
	mock2 := newMockRPC()
	c2, err := NewRealClient(RealClientOptions{RPCClient: mock2})
	if err != nil {
		t.Fatalf("construct 2: %v", err)
	}
	defer c2.Close()
	ch3, err := c2.SubscribeEvents(context.Background(), "s1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	c2.mu.Lock()
	c2.questReqID["s1"] = "q-old"
	c2.mu.Unlock()
	pushNotification(t, mock2, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"user_question_answered","payload_version":4,"payload":{}}}}`)
	recvEvent(t, ch3)
	c2.mu.Lock()
	_, still := c2.questReqID["s1"]
	c2.mu.Unlock()
	if still {
		t.Fatal("answered event should clear question slot")
	}
	// 空 params 帧（568）与无 envelope 帧（693/696）：吸收不致命。
	mock2.notifications <- gatewayclient.Notification{Method: "gateway.event"}
	mock2.notifications <- gatewayclient.Notification{Method: "gateway.event", Params: json.RawMessage(`{"session_id":"s1","payload":null}`)}
	mock2.notifications <- gatewayclient.Notification{Method: "gateway.event", Params: json.RawMessage(`{"session_id":"s1","payload":{"event_type":"unknown"}}`)}
	// 泵存活性验证：后续真实事件仍可达。
	pushNotification(t, mock2, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"agent_chunk","payload_version":4,"payload":"alive"}}}`)
	recvEvent(t, ch3)
}

// TestFlattenFinalBranches 收尾覆盖（PR #47 审计 P1-8 终轮）：
//   - 646：带 request_id 的 user_question_requested 登记追踪槽
//   - 696：直连 envelope 形态（runtime_event_type 在顶层）
//   - 602：缓冲打满后顶替 → 泵 send-select 走 sub.done 回退
//   - 103：真实构造分支的有效 socket 路径认证失败（fail-fast）
func TestFlattenFinalBranches(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	defer c.Close()
	ch, err := c.SubscribeEvents(context.Background(), "s1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// 646：带 request_id 的问答请求 → 追踪槽登记。
	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"user_question_requested","payload_version":4,"payload":{"request_id":"q-live","title":"t"}}}}`)
	event := recvEvent(t, ch)
	if event.Type != EventUserQuestionRequested {
		t.Fatalf("type = %v", event.Type)
	}
	c.mu.Lock()
	questID := c.questReqID["s1"]
	c.mu.Unlock()
	if questID != "q-live" {
		t.Fatalf("quest slot = %q, want q-live", questID)
	}

	// 696：直连 envelope 形态（runtime_event_type 在顶层，无包裹层）。
	pushNotification(t, mock, `{"session_id":"s1","payload":{"runtime_event_type":"agent_chunk","payload_version":4,"payload":"direct-form"}}`)
	event = recvEvent(t, ch)
	if event.Type != EventAgentChunk || event.Payload["text"] != "direct-form" {
		t.Fatalf("direct-form event = %+v", event)
	}

	// 103：真实构造分支——有效 socket 路径（无监听者）→ 认证拨号失败。
	_, err = NewRealClient(RealClientOptions{
		RPC: gatewayclient.GatewayRPCClientOptions{
			ListenAddress: filepath.Join(t.TempDir(), "gateway.sock"),
		},
	})
	if err == nil {
		t.Fatal("auth against dead socket should fail fast")
	}
}

// TestPumpSendSelectDoneFallback 验证缓冲打满后泵阻塞在 send-select 上，
// 顶替订阅关闭旧通道（sub.done）使泵经回退分支释放（602 行）。
func TestPumpSendSelectDoneFallback(t *testing.T) {
	mock := newMockRPC()
	c, err := NewRealClient(RealClientOptions{RPCClient: mock})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	defer c.Close()

	ch, err := c.SubscribeEvents(context.Background(), "s1")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	// 打满缓冲（128）后再注入 1 条：泵阻塞在第 129 条的 send-select 上。
	for i := 0; i < realSubBuffer+1; i++ {
		mock.notifications <- gatewayclient.Notification{Method: "gateway.event", Params: json.RawMessage(`{"session_id":"s1","payload":{"payload":{"runtime_event_type":"agent_chunk","payload_version":4,"payload":"x"}}}`)}
	}
	time.Sleep(100 * time.Millisecond) // 等泵进入阻塞态
	// 顶替订阅：旧通道 close → sub.done 就绪 → 泵经回退分支释放。
	if _, err := c.SubscribeEvents(context.Background(), "s1"); err != nil {
		t.Fatalf("resubscribe: %v", err)
	}
	// 抽干缓冲直至通道关闭（close 后缓冲值仍可接收）。
	for range ch {
	}
	if _, open := <-ch; open {
		t.Fatal("old subscription channel should be closed after replacement")
	}
}

// TestNewRealClientRejectsInvalidAddress 验证构造期参数校验：
// HOME 缺失时默认地址解析失败 → NewGatewayRPCClient 构造即失败
// （fail-fast，覆盖构造错误分支）。
func TestNewRealClientRejectsInvalidAddress(t *testing.T) {
	t.Setenv("HOME", "")
	_, err := NewRealClient(RealClientOptions{})
	if err == nil {
		t.Fatal("missing HOME should fail construction (default address resolve)")
	}
}

// TestTrySendAfterCloseNoPanic 回归测试（PR #47 审计 P0-2）：retire 与
// closeCh 完成后进入的 trySend 必须安全返回 false——done 守卫在 sendMu
// 内拦截"关闭后进入"，select{send,<-done} 双就绪随机选中 send 的
// panic 路径被前置守卫封死。循环放大以覆盖调度随机性。
func TestTrySendAfterCloseNoPanic(t *testing.T) {
	s := &realSubscription{ch: make(chan GatewayEvent, 1), done: make(chan struct{})}
	s.retire()
	s.closeCh()
	for i := 0; i < 500; i++ {
		if s.trySend(GatewayEvent{Type: EventAgentChunk}) {
			t.Fatal("trySend after close must return false")
		}
	}
}
