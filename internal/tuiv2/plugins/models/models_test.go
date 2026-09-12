package models

import (
	"context"
	"strings"
	"testing"

	"neo-code/internal/tuiv2/components"
	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// recordingHost 是 fake Host：同步执行 GoCmd 并回流广播。
var _ kernel.Host = (*recordingHost)(nil)

type recordingHost struct {
	st         *state.ViewState
	notifies   []string
	broadcasts []tea.Msg
	client     gateway.Client
	overlays   []kernel.Overlay
	react      func(tea.Msg)
}

func newRecordingHost() *recordingHost { return &recordingHost{st: state.NewViewState()} }

func (h *recordingHost) State() *state.ViewState { return h.st }
func (h *recordingHost) Gateway() gateway.Client { return h.client }
func (h *recordingHost) GoCmd(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	if msg := cmd(); msg != nil {
		h.Send(msg)
	}
}
func (h *recordingHost) Send(msg tea.Msg) {
	h.broadcasts = append(h.broadcasts, msg)
	if h.react != nil {
		h.react(msg)
	}
}
func (h *recordingHost) Mode() state.InputMode        { return h.st.Mode }
func (h *recordingHost) SetMode(m state.InputMode)    { h.st.Mode = m }
func (h *recordingHost) PushOverlay(o kernel.Overlay) { h.overlays = append(h.overlays, o) }
func (h *recordingHost) PopOverlay()                  {}
func (h *recordingHost) Confirm(req state.ConfirmRequest) {
	h.notifies = append(h.notifies, "confirm:"+req.Title)
}
func (h *recordingHost) Notify(text string)                             { h.notifies = append(h.notifies, text) }
func (h *recordingHost) Quit()                                          {}
func (h *recordingHost) BindEventStream(ch <-chan gateway.GatewayEvent) {}

func newTestPlugin(t *testing.T) (*Plugin, *recordingHost) {
	t.Helper()
	p := New()
	h := newRecordingHost()
	p.Init(context.Background(), h)
	if p.st == nil || p.picker == nil || p.client != nil {
		t.Fatal("Init state mismatch")
	}
	return p, h
}

func newTestPluginWithClient(t *testing.T, client gateway.Client) (*Plugin, *recordingHost) {
	t.Helper()
	p := New()
	h := newRecordingHost()
	h.client = client
	p.Init(context.Background(), h)
	return p, h
}

func TestPluginIdentity(t *testing.T) {
	if got := New().ID(); got != "models" {
		t.Fatalf("id = %q", got)
	}
}

func TestReactModelChangedMigratesSlot(t *testing.T) {
	p, h := newTestPlugin(t)
	_ = h
	p.React(h, ev(gateway.EventModelChanged, map[string]any{"model_id": "m2"}))
	if p.st.Gateway.ActiveModel != "m2" {
		t.Fatalf("active model = %q", p.st.Gateway.ActiveModel)
	}
}

func TestReactIgnoresNonGateway(t *testing.T) {
	p, h := newTestPlugin(t)
	p.React(h, state.UserSubmitted{Text: "x"})
	if len(p.st.Gateway.Models) != 0 {
		t.Fatal("non-gateway messages must be ignored")
	}
}

func TestHandleSelectSuccessAndFailure(t *testing.T) {
	client := &fakeClient{}
	p, h := newTestPluginWithClient(t, client)
	p.st.Gateway.ActiveSess = &gateway.SessionSummary{ID: "s1"}
	p.st.Runtime.RunID = "run-1"

	// 失败：SetModel 返回错误 → EventError 广播（流错误由 chat 呈现）。
	client.setModelErr = errFake("boom")
	p.React(h, components.ModelSelectMsg{ModelID: "m-broken"})
	foundErr := false
	for _, b := range h.broadcasts {
		if m, ok := b.(gateway.GatewayEvent); ok && m.Type == gateway.EventError {
			foundErr = true
		}
	}
	if !foundErr {
		t.Fatal("model switch failure should broadcast EventError")
	}
	// 失败不得改动 ActiveModel。
	if p.st.Gateway.ActiveModel == "m-broken" {
		t.Fatal("failed switch must not update ActiveModel")
	}

	// 成功：清注入后合成 model_changed → React 迁移 ActiveModel。
	client.setModelErr = nil
	p.React(h, components.ModelSelectMsg{ModelID: "m-good"})
	foundOK := false
	for _, b := range h.broadcasts {
		if m, ok := b.(gateway.GatewayEvent); ok && m.Type == gateway.EventModelChanged && m.Payload["model_id"] == "m-good" {
			foundOK = true
		}
	}
	if !foundOK {
		t.Fatal("model switch success should broadcast model_changed")
	}
}

func TestModelCommandAndLeaderOpenPicker(t *testing.T) {
	p, h := newTestPlugin(t)
	byName := map[string]kernel.Command{}
	for _, c := range p.Commands() {
		byName[c.Name] = c
	}
	byName["/model"].Run(h, nil)
	if len(h.overlays) != 1 {
		t.Fatal("/model should push picker")
	}
	for _, b := range p.Bindings() {
		if b.Key == "m" && b.Mode == state.LeaderMode {
			b.OnKey(h)
		}
	}
	if len(h.overlays) != 2 {
		t.Fatalf("overlays = %d, want 2", len(h.overlays))
	}
}

func TestPickerOverlayDelegates(t *testing.T) {
	p, h := newTestPlugin(t)
	o := &pickerOverlay{p: p}
	if o.ID() != "models.picker" {
		t.Fatalf("id = %q", o.ID())
	}
	consumed := o.HandleKey(h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if !consumed {
		t.Fatal("picker is modal")
	}
	if o.View(h, 60) == "" {
		t.Fatal("picker view should not be empty")
	}
}

func TestHandleSelectWithoutClientDegrades(t *testing.T) {
	p, h := newTestPlugin(t)
	p.React(h, components.ModelSelectMsg{ModelID: "m1"})
	if !strings.Contains(h.notifies[len(h.notifies)-1], "无可用后端") {
		t.Fatalf("notifies = %v", h.notifies)
	}
}

func ev(t gateway.EventType, payload map[string]any) gateway.GatewayEvent {
	if payload == nil {
		payload = map[string]any{}
	}
	return gateway.GatewayEvent{Type: t, Payload: payload}
}

// fakeClient 记录 SetModel 调用（可注入错误）。
type fakeClient struct {
	setModelErr error
	setCalls    []string
}

func (c *fakeClient) SetModel(ctx context.Context, sessionID, modelID string) error {
	c.setCalls = append(c.setCalls, sessionID+"|"+modelID)
	return c.setModelErr
}

func (c *fakeClient) Health(ctx context.Context) (*gateway.HealthResult, error) {
	return nil, errFakeUnsupported
}
func (c *fakeClient) ListSessions(ctx context.Context) ([]gateway.SessionSummary, error) {
	return nil, errFakeUnsupported
}
func (c *fakeClient) LoadSession(ctx context.Context, id string) (*gateway.SessionDetail, error) {
	return nil, errFakeUnsupported
}
func (c *fakeClient) CreateSession(ctx context.Context) (*gateway.SessionSummary, error) {
	return nil, errFakeUnsupported
}
func (c *fakeClient) SendMessage(ctx context.Context, sessionID, text string) (*gateway.RunAck, error) {
	return nil, errFakeUnsupported
}
func (c *fakeClient) CancelRun(ctx context.Context, sessionID, runID string) error {
	return errFakeUnsupported
}
func (c *fakeClient) SubscribeEvents(ctx context.Context, sessionID string) (<-chan gateway.GatewayEvent, error) {
	return nil, errFakeUnsupported
}
func (c *fakeClient) ResolvePermission(ctx context.Context, decision gateway.PermissionDecision) error {
	return errFakeUnsupported
}
func (c *fakeClient) AnswerUserQuestion(ctx context.Context, answer gateway.UserQuestionAnswer) error {
	return errFakeUnsupported
}
func (c *fakeClient) ListModels(ctx context.Context) ([]gateway.ModelInfo, error) {
	return nil, errFakeUnsupported
}
func (c *fakeClient) GetModel(ctx context.Context, sessionID string) (string, error) {
	return "", errFakeUnsupported
}
func (c *fakeClient) Close() error { return nil }

var errFakeUnsupported = errFake("fake: unsupported")

type errFake string

func (e errFake) Error() string { return string(e) }

func TestModelsCloseIsSafe(t *testing.T) {
	p, _ := newTestPlugin(t)
	p.Close(context.Background()) // 无外部资源，对称生命周期
}

func TestPickerGoCmdBranchForSelection(t *testing.T) {
	// 选择器产出选择消息（非 nil 命令）→ GoCmd 分支：回流广播合成事件。
	client := &fakeClient{}
	p, h := newTestPluginWithClient(t, client)
	p.st.Gateway.ActiveSess = &gateway.SessionSummary{ID: "s1"}
	p.st.Gateway.Models = []gateway.ModelInfo{{ID: "m-good", Name: "Good Model"}}
	h.react = func(msg tea.Msg) { p.React(h, msg) }
	o := &pickerOverlay{p: p}
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyEnter})
	found := false
	for _, b := range h.broadcasts {
		if m, ok := b.(gateway.GatewayEvent); ok && m.Type == gateway.EventModelChanged && m.Payload["model_id"] == "m-good" {
			found = true
		}
	}
	if !found {
		t.Fatal("picker enter should produce model_changed broadcast")
	}
}
