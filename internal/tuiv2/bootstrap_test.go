package tuiv2

import (
	"context"
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
