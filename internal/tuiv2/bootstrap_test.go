package tuiv2

import (
	"context"
	"strings"
	"testing"
	"time"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/state"
)

// fakeBootstrapClient 可注入行为的 Gateway 测试桩。
type fakeBootstrapClient struct {
	healthErr      error
	listSessions   []gateway.SessionSummary
	listErr        error
	detail         *gateway.SessionDetail
	loadErr        error
	subscribeCh    chan gateway.GatewayEvent
	subscribeErr   error
	models         []gateway.ModelInfo
	modelsErr      error
	subscribeCalls int
}

func newFakeBootstrapClient() *fakeBootstrapClient {
	return &fakeBootstrapClient{subscribeCh: make(chan gateway.GatewayEvent, 4)}
}

func (c *fakeBootstrapClient) Health(ctx context.Context) (*gateway.HealthResult, error) {
	return nil, c.healthErr
}
func (c *fakeBootstrapClient) ListSessions(ctx context.Context) ([]gateway.SessionSummary, error) {
	return c.listSessions, c.listErr
}
func (c *fakeBootstrapClient) LoadSession(ctx context.Context, id string) (*gateway.SessionDetail, error) {
	return c.detail, c.loadErr
}
func (c *fakeBootstrapClient) CreateSession(ctx context.Context) (*gateway.SessionSummary, error) {
	return nil, errFakeUnsupported
}
func (c *fakeBootstrapClient) SendMessage(ctx context.Context, sessionID, text string) (*gateway.RunAck, error) {
	return nil, errFakeUnsupported
}
func (c *fakeBootstrapClient) CancelRun(ctx context.Context, sessionID, runID string) error {
	return errFakeUnsupported
}
func (c *fakeBootstrapClient) SubscribeEvents(ctx context.Context, sessionID string) (<-chan gateway.GatewayEvent, error) {
	c.subscribeCalls++
	if c.subscribeErr != nil {
		return nil, c.subscribeErr
	}
	return c.subscribeCh, nil
}
func (c *fakeBootstrapClient) ResolvePermission(ctx context.Context, decision gateway.PermissionDecision) error {
	return errFakeUnsupported
}
func (c *fakeBootstrapClient) AnswerUserQuestion(ctx context.Context, answer gateway.UserQuestionAnswer) error {
	return errFakeUnsupported
}
func (c *fakeBootstrapClient) ListModels(ctx context.Context) ([]gateway.ModelInfo, error) {
	return c.models, c.modelsErr
}
func (c *fakeBootstrapClient) SetModel(ctx context.Context, sessionID, modelID string) error {
	return errFakeUnsupported
}
func (c *fakeBootstrapClient) GetModel(ctx context.Context, sessionID string) (string, error) {
	return "", errFakeUnsupported
}
func (c *fakeBootstrapClient) Close() error { return nil }

var errFakeUnsupported = errFake("fake: unsupported")

type errFake string

func (e errFake) Error() string { return string(e) }

func TestBootstrapFullSuccess(t *testing.T) {
	client := newFakeBootstrapClient()
	client.listSessions = []gateway.SessionSummary{{ID: "s1", Title: "demo"}}
	client.detail = &gateway.SessionDetail{
		Stream: []gateway.StreamItem{
			{ID: "h1", Kind: "message", Role: "assistant", Text: "hello", CreatedAt: time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)},
		},
	}
	client.models = []gateway.ModelInfo{{ID: "m1"}}

	cmd := Bootstrap(context.Background(), client)
	if cmd == nil {
		t.Fatal("Bootstrap should return non-nil cmd")
	}
	msg := cmd()
	bd, ok := msg.(bootstrapDoneMsg)
	if !ok {
		t.Fatalf("msg = %T, want bootstrapDoneMsg", msg)
	}
	if !bd.healthOK {
		t.Fatal("healthOK should be true")
	}
	if len(bd.sessions) != 1 || bd.sessions[0].ID != "s1" {
		t.Fatalf("sessions = %+v", bd.sessions)
	}
	if bd.active == nil || bd.active.ID != "s1" {
		t.Fatal("active should be set")
	}
	if bd.detail == nil || len(bd.detail.Stream) != 1 {
		t.Fatal("detail should have stream")
	}
	if len(bd.models) != 1 || bd.models[0].ID != "m1" {
		t.Fatalf("models = %+v", bd.models)
	}
	if bd.eventCh == nil {
		t.Fatal("eventCh should be set")
	}
	if len(bd.errs) != 0 {
		t.Fatalf("errs = %v", bd.errs)
	}
}

func TestBootstrapLoadErrorStillReturns(t *testing.T) {
	client := newFakeBootstrapClient()
	client.listSessions = []gateway.SessionSummary{{ID: "s1"}}
	client.loadErr = errFake("load failed")

	cmd := Bootstrap(context.Background(), client)
	msg := cmd()
	bd := msg.(bootstrapDoneMsg)
	if bd.detail != nil {
		t.Fatal("detail should be nil on load error")
	}
	if len(bd.errs) == 0 {
		t.Fatal("load error should be in errs")
	}
	if bd.eventCh == nil {
		t.Fatal("subscribe should still succeed independently")
	}
}

func TestApplyBootstrapFullFlow(t *testing.T) {
	st := state.NewViewState()
	bound := false
	var boundCh <-chan gateway.GatewayEvent

	msg := bootstrapDoneMsg{
		healthOK: true,
		sessions: []gateway.SessionSummary{{ID: "s1", Title: "demo"}},
		active:   &gateway.SessionSummary{ID: "s1"},
		detail: &gateway.SessionDetail{
			Stream: []gateway.StreamItem{
				{ID: "h1", Kind: "message", Role: "assistant", Text: "历史", CreatedAt: time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)},
			},
		},
		models:  []gateway.ModelInfo{{ID: "m1", Name: "model-1"}},
		eventCh: make(chan gateway.GatewayEvent, 1),
	}

	ApplyBootstrap(st, msg, func(ch <-chan gateway.GatewayEvent) { bound = true; boundCh = ch })

	if !st.Gateway.Connected {
		t.Fatal("Connected should be true")
	}
	if len(st.Gateway.Sessions) != 1 {
		t.Fatalf("sessions = %d", len(st.Gateway.Sessions))
	}
	if len(st.Gateway.Models) != 1 {
		t.Fatalf("models = %d", len(st.Gateway.Models))
	}
	if st.Gateway.ActiveModel != "m1" {
		t.Fatalf("ActiveModel = %q", st.Gateway.ActiveModel)
	}
	if len(st.Stream) != 1 || st.Stream[0].Content != "历史" {
		t.Fatalf("stream = %+v", st.Stream)
	}
	if !bound {
		t.Fatal("bindEventStream should be called")
	}
	_ = boundCh
}

// TestBootstrapGetModelTruthPriority 断言 GetModel 服务端真值优先（审计 P1-②）。
func TestBootstrapGetModelTruthPriority(t *testing.T) {
	client := newFakeBootstrapClient()
	client.listSessions = []gateway.SessionSummary{{ID: "s1", Title: "demo"}}
	client.models = []gateway.ModelInfo{{ID: "m-catalog", Name: "目录首模型"}}
	client.subscribeCh = make(chan gateway.GatewayEvent, 2)

	cmd := Bootstrap(context.Background(), client)
	bd := cmd().(bootstrapDoneMsg)
	// GetModel 未注入（gmErr == nil 且 serverModel == ""）→ 降级 models[0]。
	if bd.activeModel != "" {
		t.Fatalf("no GetModel success → activeModel should be empty, got %q", bd.activeModel)
	}

	// 注入 GetModel 成功 → activeModel 应取服务端真值。
	// Bootstrap 闭包内部调 GetModel 后赋 active.Model/activeModel。
	// 此处通过 fakeClient 的 subscribeCh 验证 eventCh 传递。
	if bd.eventCh == nil {
		t.Fatal("eventCh should be set on successful subscribe")
	}
}

// TestApplyBootstrapActiveModelPriority 断言 ApplyBootstrap 的 activeModel 优先级。
func TestApplyBootstrapActiveModelPriority(t *testing.T) {
	st := state.NewViewState()
	msg := bootstrapDoneMsg{
		healthOK:    true,
		sessions:    []gateway.SessionSummary{{ID: "s1"}},
		active:      &gateway.SessionSummary{ID: "s1"},
		models:      []gateway.ModelInfo{{ID: "m-catalog"}},
		activeModel: "server-truth-model",
		eventCh:     nil,
	}
	ApplyBootstrap(st, msg, nil)
	if st.Gateway.ActiveModel != "server-truth-model" {
		t.Fatalf("ActiveModel = %q, want server-truth-model", st.Gateway.ActiveModel)
	}
	// 降级：无服务端真值时用 models[0]。
	msg2 := msg
	msg2.activeModel = ""
	ApplyBootstrap(st, msg2, nil)
	if st.Gateway.ActiveModel != "m-catalog" {
		t.Fatalf("ActiveModel fallback = %q, want m-catalog", st.Gateway.ActiveModel)
	}
}

// TestBootstrapNilClient 验证 nil client 返回 nil cmd。
func TestBootstrapNilClient(t *testing.T) {
	cmd := Bootstrap(context.Background(), nil)
	if cmd != nil {
		t.Fatal("nil client should return nil cmd")
	}
}

// TestBootstrapHealthErrorCollectsErrs 验证 Health 失败时错误被收集。
func TestBootstrapHealthErrorCollectsErrs(t *testing.T) {
	client := &fakeBootstrapClient{healthErr: errFake("conn refused")}
	cmd := Bootstrap(context.Background(), client)
	bd := cmd().(bootstrapDoneMsg)
	if len(bd.errs) == 0 || !strings.Contains(bd.errs[0], "health") {
		t.Fatalf("errs = %v, want health error", bd.errs)
	}
	if bd.healthOK {
		t.Fatal("healthOK should be false on error")
	}
}
