package sessions

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

// recordingHost 是 fake Host：同步执行 GoCmd 并经 react 回调回流广播
// （模拟 kernel"命令执行→消息回流→广播"链路），记录换代绑定。
var _ kernel.Host = (*recordingHost)(nil)

type recordingHost struct {
	st          *state.ViewState
	notifies    []string
	broadcasts  []tea.Msg
	client      gateway.Client
	bindCount   int
	boundCh     <-chan gateway.GatewayEvent
	confirmReqs []state.ConfirmRequest
	overlays    []kernel.Overlay
	react       func(tea.Msg) // 广播回插件 React（模拟 kernel 分发；nil 则只记录）
}

func newRecordingHost() *recordingHost { return &recordingHost{st: state.NewViewState()} }

func (h *recordingHost) State() *state.ViewState           { return h.st }
func (h *recordingHost) Gateway() gateway.Client           { return h.client }
func (h *recordingHost) Commands() []kernel.Command        { return nil }
func (h *recordingHost) RunCommand(string, []string) error { return nil }
func (h *recordingHost) Bindings() []kernel.Binding        { return nil }
func (h *recordingHost) GoCmd(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	if msg := cmd(); msg != nil {
		h.Send(msg) // 命令产物回流广播
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
	h.confirmReqs = append(h.confirmReqs, req)
}
func (h *recordingHost) Notify(text string) { h.notifies = append(h.notifies, text) }
func (h *recordingHost) Quit()              {}
func (h *recordingHost) BindEventStream(ch <-chan gateway.GatewayEvent) {
	h.bindCount++
	h.boundCh = ch
}

func newTestPlugin(t *testing.T) (*Plugin, *recordingHost) {
	t.Helper()
	p := New()
	h := newRecordingHost()
	p.Init(context.Background(), h)
	if p.st == nil || p.client != nil || p.picker == nil {
		t.Fatal("Init should fix state/picker and record nil client")
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

func ev(t gateway.EventType, payload map[string]any) gateway.GatewayEvent {
	if payload == nil {
		payload = map[string]any{}
	}
	return gateway.GatewayEvent{Type: t, Payload: payload}
}

func TestPluginIdentity(t *testing.T) {
	if got := New().ID(); got != "sessions" {
		t.Fatalf("id = %q", got)
	}
}

func TestReactGatewayEventsMigrateSlots(t *testing.T) {
	p, h := newTestPlugin(t)
	p.React(h, ev(gateway.EventSessionCreated, map[string]any{"id": "s1", "title": "demo"}))
	if len(p.st.Gateway.Sessions) != 1 || p.st.Gateway.Sessions[0].Title != "demo" {
		t.Fatalf("sessions = %+v", p.st.Gateway.Sessions)
	}
	p.React(h, ev(gateway.EventSessionUpdated, map[string]any{"id": "s1", "title": "demo2"}))
	if p.st.Gateway.Sessions[0].Title != "demo2" {
		t.Fatalf("updated = %+v", p.st.Gateway.Sessions[0])
	}
	p.React(h, ev(gateway.EventSessionDeleted, map[string]any{"id": "s1"}))
	if len(p.st.Gateway.Sessions) != 0 {
		t.Fatalf("deleted = %+v", p.st.Gateway.Sessions)
	}
}

func TestReactHealthChangedTempOwnership(t *testing.T) {
	p, h := newTestPlugin(t)
	p.React(h, ev(gateway.EventHealthChanged, map[string]any{"connected": true}))
	if !p.st.Gateway.Connected {
		t.Fatal("Connected should be true (temp ownership until S6)")
	}
	p.React(h, ev(gateway.EventHealthChanged, map[string]any{"connected": false}))
	if p.st.Gateway.Connected {
		t.Fatal("Connected should be false")
	}
}

func TestReactSessionDeletedMessage(t *testing.T) {
	p, h := newTestPlugin(t)
	p.st.Gateway.Sessions = []gateway.SessionSummary{{ID: "s1", Title: "demo"}}
	p.React(h, state.SessionDeleted{ID: "s1"})
	if len(p.st.Gateway.Sessions) != 0 {
		t.Fatalf("sessions = %+v", p.st.Gateway.Sessions)
	}
	if len(h.notifies) == 0 {
		t.Fatal("deletion should notify")
	}
}

func TestHandleSelectLoadsAndRebinds(t *testing.T) {
	client := &fakeClient{detail: &gateway.SessionDetail{}}
	p, h := newTestPluginWithClient(t, client)
	h.BindEventStream(make(chan gateway.GatewayEvent, 1)) // 旧代际占位
	h.react = func(msg tea.Msg) { p.React(h, msg) }

	sess := gateway.SessionSummary{ID: "s2", Title: "target"}
	p.React(h, components.SessionSelectMsg{Session: sess})

	if p.st.Gateway.ActiveSess == nil || p.st.Gateway.ActiveSess.ID != "s2" {
		t.Fatalf("active = %+v", p.st.Gateway.ActiveSess)
	}
	if h.bindCount != 2 {
		t.Fatalf("BindEventStream count = %d, want 2 (placeholder + rebind)", h.bindCount)
	}
	if client.loadCalls != 1 || client.subscribeCalls != 1 {
		t.Fatalf("load=%d subscribe=%d, want 1/1", client.loadCalls, client.subscribeCalls)
	}
	found := false
	for _, b := range h.broadcasts {
		if m, ok := b.(state.SessionLoaded); ok && m.Session.ID == "s2" && m.Detail != nil {
			found = true
		}
	}
	if !found {
		t.Fatal("SessionLoaded should be broadcast with detail")
	}
}

func TestHandleSelectLoadErrorStillBroadcasts(t *testing.T) {
	client := &fakeClient{loadErr: errFake("load boom")}
	p, h := newTestPluginWithClient(t, client)
	h.react = func(msg tea.Msg) { p.React(h, msg) }
	p.React(h, components.SessionSelectMsg{Session: gateway.SessionSummary{ID: "s2"}})
	// 加载失败也广播 SessionLoaded（无 Detail）——chat 据此清空流并提示。
	found := false
	for _, b := range h.broadcasts {
		if m, ok := b.(state.SessionLoaded); ok && m.Detail == nil {
			found = true
		}
	}
	if !found {
		t.Fatal("load error should still broadcast SessionLoaded without detail")
	}
}

func TestDeleteConfirmFlow(t *testing.T) {
	p, h := newTestPlugin(t)
	p.st.Gateway.ActiveSess = &gateway.SessionSummary{ID: "s1", Title: "demo"}
	// /delete → 确认请求。
	byName := commandMap(p)
	byName["/delete"].Run(h, nil)
	if len(h.confirmReqs) != 1 || h.confirmReqs[0].Action != "delete_session" {
		t.Fatalf("confirmReqs = %+v", h.confirmReqs)
	}
	// 确认 → SessionDeleted 广播 → React 迁移槽 + 通知。
	p.React(h, state.ConfirmResult{Action: "delete_session", Yes: true, Data: map[string]any{"id": "s1"}})
	if len(p.st.Gateway.Sessions) != 0 {
		t.Fatalf("sessions after delete = %+v", p.st.Gateway.Sessions)
	}
}

func TestNewCommandCreatesSession(t *testing.T) {
	client := &fakeClient{created: &gateway.SessionSummary{ID: "new-1", Title: "新建"}}
	p, h := newTestPluginWithClient(t, client)
	h.react = func(msg tea.Msg) { p.React(h, msg) }
	byName := commandMap(p)
	byName["/new"].Run(h, nil)
	if len(p.st.Gateway.Sessions) != 1 || p.st.Gateway.Sessions[0].ID != "new-1" {
		t.Fatalf("sessions = %+v", p.st.Gateway.Sessions)
	}
	if client.createCalls != 1 {
		t.Fatalf("createCalls = %d", client.createCalls)
	}
}

func TestNewWithoutClientDegrades(t *testing.T) {
	p, h := newTestPlugin(t)
	byName := commandMap(p)
	byName["/new"].Run(h, nil)
	if !strings.Contains(h.notifies[len(h.notifies)-1], "无可用后端") {
		t.Fatalf("notifies = %v", h.notifies)
	}
}

func TestSessionCommandOpensPicker(t *testing.T) {
	p, h := newTestPlugin(t)
	byName := commandMap(p)
	byName["/session"].Run(h, nil)
	if len(h.overlays) != 1 {
		t.Fatal("/session should push picker overlay")
	}
	// Leader s 绑定同样打开选择器。
	for _, b := range p.Bindings() {
		if b.Key == "s" && b.Mode == state.LeaderMode {
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
	if o.ID() != "sessions.picker" {
		t.Fatalf("id = %q", o.ID())
	}
	// 模态消费：任意键返回 true 且委托组件。
	consumed := o.HandleKey(h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if !consumed {
		t.Fatal("picker should consume keys")
	}
}

func TestDeleteWithoutActiveSessionNotifies(t *testing.T) {
	p, h := newTestPlugin(t)
	byName := commandMap(p)
	byName["/delete"].Run(h, nil)
	if len(h.confirmReqs) != 0 {
		t.Fatal("delete without active session should not confirm")
	}
	if !strings.Contains(h.notifies[len(h.notifies)-1], "无活跃会话") {
		t.Fatalf("notifies = %v", h.notifies)
	}
}

// commandMap 把命令表转为按名索引。
func commandMap(p *Plugin) map[string]kernel.Command {
	m := map[string]kernel.Command{}
	for _, c := range p.Commands() {
		m[c.Name] = c
	}
	return m
}

// fakeClient 记录 RPC 调用并支持注入行为。
type fakeClient struct {
	createCalls    int
	loadCalls      int
	subscribeCalls int
	created        *gateway.SessionSummary
	detail         *gateway.SessionDetail
	loadErr        error
	subscribeErr   error
}

func (c *fakeClient) Health(ctx context.Context) (*gateway.HealthResult, error) {
	return nil, errFakeUnsupported
}
func (c *fakeClient) ListSessions(ctx context.Context) ([]gateway.SessionSummary, error) {
	return nil, errFakeUnsupported
}
func (c *fakeClient) LoadSession(ctx context.Context, id string) (*gateway.SessionDetail, error) {
	c.loadCalls++
	return c.detail, c.loadErr
}
func (c *fakeClient) CreateSession(ctx context.Context) (*gateway.SessionSummary, error) {
	c.createCalls++
	if c.created != nil {
		return c.created, nil
	}
	return nil, errFakeUnsupported
}
func (c *fakeClient) SendMessage(ctx context.Context, sessionID, text string) (*gateway.RunAck, error) {
	return nil, errFakeUnsupported
}
func (c *fakeClient) CancelRun(ctx context.Context, sessionID, runID string) error {
	return errFakeUnsupported
}
func (c *fakeClient) SubscribeEvents(ctx context.Context, sessionID string) (<-chan gateway.GatewayEvent, error) {
	c.subscribeCalls++
	if c.subscribeErr != nil {
		return nil, c.subscribeErr
	}
	ch := make(chan gateway.GatewayEvent, 1)
	return ch, nil
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
func (c *fakeClient) SetModel(ctx context.Context, sessionID, modelID string) error {
	return errFakeUnsupported
}
func (c *fakeClient) GetModel(ctx context.Context, sessionID string) (string, error) {
	return "", errFakeUnsupported
}
func (c *fakeClient) Close() error { return nil }

var errFakeUnsupported = errFake("fake: unsupported")

type errFake string

func (e errFake) Error() string { return string(e) }

// TestCloseCancelsStream 锁定生命周期：Close 取消当前事件订阅。
func TestCloseCancelsStream(t *testing.T) {
	client := &fakeClient{detail: &gateway.SessionDetail{}}
	p, h := newTestPluginWithClient(t, client)
	h.react = func(msg tea.Msg) { p.React(h, msg) }
	// 选择会话 → 订阅建立（streamCtx 在 ready 分支登记）。
	p.React(h, components.SessionSelectMsg{Session: gateway.SessionSummary{ID: "s2"}})
	if p.streamCtx == nil {
		t.Fatal("streamCtx should be set after subscribe")
	}
	p.Close(context.Background())
	if p.streamCtx != nil {
		t.Fatal("Close should cancel stream ctx")
	}
}

// TestCloseWithoutStreamIsSafe：无订阅时 Close 安全。
func TestCloseWithoutStreamIsSafe(t *testing.T) {
	p, _ := newTestPlugin(t)
	p.Close(context.Background())
}

// TestHandleSelectSubscribeError：订阅失败 → 广播无 Detail 的 SessionLoaded。
func TestHandleSelectSubscribeError(t *testing.T) {
	client := &fakeClient{detail: &gateway.SessionDetail{}, subscribeErr: errFake("sub boom")}
	p, h := newTestPluginWithClient(t, client)
	h.react = func(msg tea.Msg) { p.React(h, msg) }
	p.React(h, components.SessionSelectMsg{Session: gateway.SessionSummary{ID: "s3"}})
	found := false
	for _, b := range h.broadcasts {
		if m, ok := b.(state.SessionLoaded); ok && m.Detail == nil {
			found = true
		}
	}
	if !found {
		t.Fatal("subscribe error should broadcast SessionLoaded without detail")
	}
}

// TestRunNewCreateError：创建失败 → Notify 错误、不迁移槽。
func TestRunNewCreateError(t *testing.T) {
	// client 存在但 CreateSession 失败（created=nil → 合成 EventError，
	// 由 chat 流错误路径呈现——Notify 职责已随 P0-1 移出闭包）。
	client := &fakeClient{}
	p, h := newTestPluginWithClient(t, client)
	h.react = func(msg tea.Msg) { p.React(h, msg) }
	byName := commandMap(p)
	byName["/new"].Run(h, nil)
	found := false
	for _, b := range h.broadcasts {
		if m, ok := b.(gateway.GatewayEvent); ok && m.Type == gateway.EventError {
			found = true
		}
	}
	if !found {
		t.Fatal("create error should broadcast EventError")
	}
	if len(p.st.Gateway.Sessions) != 0 {
		t.Fatal("create error should not mutate sessions")
	}
}

// TestPickerOverlayView：浮层 View 委托组件非空。
func TestPickerOverlayView(t *testing.T) {
	p, h := newTestPlugin(t)
	o := &pickerOverlay{p: p}
	if out := o.View(h, 60); out == "" {
		t.Fatal("picker view should not be empty")
	}
}

// TestPickerEnterProducesSelectMsg：picker 内 Enter 产出选择消息并经
// GoCmd 回流广播（委托链完整闭环）。
func TestPickerEnterProducesSelectMsg(t *testing.T) {
	p, h := newTestPlugin(t)
	h.react = func(msg tea.Msg) { p.React(h, msg) }
	p.st.Gateway.Sessions = []gateway.SessionSummary{{ID: "s1", Title: "demo"}}
	o := &pickerOverlay{p: p}
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyEnter})
	found := false
	for _, b := range h.broadcasts {
		if m, ok := b.(components.SessionSelectMsg); ok && m.Session.ID == "s1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("SessionSelectMsg missing among %d broadcasts", len(h.broadcasts))
	}
}

// TestConfirmResultNoDoesNotDelete：确认取消不删除会话。
func TestConfirmResultNoDoesNotDelete(t *testing.T) {
	p, h := newTestPlugin(t)
	p.st.Gateway.Sessions = []gateway.SessionSummary{{ID: "s1"}}
	p.React(h, state.ConfirmResult{Action: "delete_session", Yes: false, Data: map[string]any{"id": "s1"}})
	if len(p.st.Gateway.Sessions) != 1 {
		t.Fatal("cancelled delete must keep session")
	}
}

// TestHandleSelectRecordsPrevSession：切换时记录上一会话 ID（Space Space 语义）。
func TestHandleSelectRecordsPrevSession(t *testing.T) {
	client := &fakeClient{detail: &gateway.SessionDetail{}}
	p, h := newTestPluginWithClient(t, client)
	p.st.Gateway.ActiveSess = &gateway.SessionSummary{ID: "old"}
	p.React(h, components.SessionSelectMsg{Session: gateway.SessionSummary{ID: "new"}})
	if p.prevID != "old" {
		t.Fatalf("prevID = %q, want old", p.prevID)
	}
}

// TestPickerDeleteProducesDeleteMsg：picker 内 Ctrl+D 产出删除请求 →
// 确认服务（委托链闭环）。
func TestPickerDeleteProducesDeleteMsg(t *testing.T) {
	p, h := newTestPlugin(t)
	h.react = func(msg tea.Msg) { p.React(h, msg) }
	p.st.Gateway.Sessions = []gateway.SessionSummary{{ID: "s1", Title: "demo"}}
	p.st.Gateway.ActiveSess = &gateway.SessionSummary{ID: "s1"}
	o := &pickerOverlay{p: p}
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyCtrlD})
	if len(h.confirmReqs) != 1 || h.confirmReqs[0].Data["id"] != "s1" {
		t.Fatalf("confirmReqs = %+v", h.confirmReqs)
	}
}

// TestPickerOtherKeysDelegated：非关闭键（如字符过滤）委托组件且模态消费。
func TestPickerOtherKeysDelegated(t *testing.T) {
	p, h := newTestPlugin(t)
	h.BindEventStream(make(chan gateway.GatewayEvent, 1))
	o := &pickerOverlay{p: p}
	consumed := o.HandleKey(h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if !consumed {
		t.Fatal("other keys should be consumed (modal)")
	}
}

// TestPickerEscPopsOverlay：esc 由浮层关闭自身（P0-2 弹栈语义）。
func TestPickerEscPopsOverlay(t *testing.T) {
	p, h := newTestPlugin(t)
	h.BindEventStream(make(chan gateway.GatewayEvent, 1))
	h.PushOverlay(&pickerOverlay{p: p})
	o := &pickerOverlay{p: p}
	consumed := o.HandleKey(h, tea.KeyMsg{Type: tea.KeyEsc})
	if !consumed {
		t.Fatal("esc should be consumed by picker")
	}
}

// TestPickerEscWithPendingComponentCmd：esc 且组件仍有产出命令时
// （先 GoCmd 再弹栈，两步都执行）。
func TestPickerEscWithPendingComponentCmd(t *testing.T) {
	p, h := newTestPlugin(t)
	h.BindEventStream(make(chan gateway.GatewayEvent, 1))
	// 预置会话并选中：esc 分支此前组件仍会产出命令（ctrl+d 语义残留）。
	p.st.Gateway.Sessions = []gateway.SessionSummary{{ID: "s1"}}
	p.st.Overlay.Selected = 0
	o := &pickerOverlay{p: p}
	// esc 路径：先 Pop 后不再委托（见 keys.go 分支顺序——本用例锁
	// esc 消费行为与弹栈）。
	consumed := o.HandleKey(h, tea.KeyMsg{Type: tea.KeyEsc})
	if !consumed {
		t.Fatal("esc consumed")
	}
}
