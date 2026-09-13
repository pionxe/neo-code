package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

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
	s.close()
	s.close() // 重叠关闭不得 panic（事件/完成信号双通道）
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
// 优先、空值回填追踪槽、双槽独立、无可用 ID 本地报错、Allow→allow。
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
	if params.RequestID != "perm-explicit" || params.Decision != "allow" {
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
	if params.RequestID != "perm-pending" || params.Decision != "deny" {
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

	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"phase_changed","payload":{"from":"plan","to":"execute"}}}}`)
	event := recvEvent(t, ch)
	if event.Type != EventPhaseChanged || event.Payload["phase"] != "running" {
		t.Fatalf("event = %+v", event)
	}

	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"phase_changed","payload":{"from":"execute","to":"waiting_user_question"}}}}`)
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

	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"tool_result","payload":{"ToolCallID":"t1","Name":"bash","Content":"ok","IsError":false}}}}`)
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
	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"permission_requested","payload":{"request_id":"perm-42","tool_name":"bash","operation":"ls"}}}}`)
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
	if params.RequestID != "perm-42" || params.Decision != "allow" {
		t.Fatalf("params = %+v", params)
	}

	// resolved 事件清槽 → 再次提交本地报错（防陈旧回填）。
	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"permission_resolved","payload":{"request_id":"perm-42"}}}}`)
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

	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"budget_checked","payload":{"decision":"allow"}}}}`)
	pushNotification(t, mock, `{"session_id":"s1","payload":{"payload":{"runtime_event_type":"token_usage","payload":{"input_tokens":10,"output_tokens":5}}}}`)

	event := recvEvent(t, ch)
	if event.Type != EventTokenUsage {
		t.Fatalf("type = %v, want token_usage after dropped budget_checked", event.Type)
	}
	// runtime 无 total 键：由 input+output 合成（S5 审计裁定）。
	if event.Payload["total"] != 15 {
		t.Fatalf("total = %v, want 15", event.Payload["total"])
	}
}
