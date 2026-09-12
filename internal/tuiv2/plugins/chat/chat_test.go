package chat

import (
	"context"
	"strings"
	"testing"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// recordingHost 是插件测试用的 fake Host（Host 第二实现的插件侧复用，
// 与 kernel 包的 testFakeHost 同源语义）。
var _ kernel.Host = (*recordingHost)(nil)

type recordingHost struct {
	st         *state.ViewState
	notifies   []string
	broadcasts []tea.Msg
	client     gateway.Client
}

func newRecordingHost() *recordingHost {
	return &recordingHost{st: state.NewViewState()}
}

func (h *recordingHost) State() *state.ViewState                        { return h.st }
func (h *recordingHost) Gateway() gateway.Client                        { return h.client }
func (h *recordingHost) BindEventStream(ch <-chan gateway.GatewayEvent) {}

// GoCmd 同步执行以观察副作用（生产路径由 bubbletea 异步执行）。
func (h *recordingHost) GoCmd(cmd tea.Cmd) {
	if cmd != nil {
		cmd()
	}
}
func (h *recordingHost) Send(msg tea.Msg)             { h.broadcasts = append(h.broadcasts, msg) }
func (h *recordingHost) Mode() state.InputMode        { return h.st.Mode }
func (h *recordingHost) SetMode(m state.InputMode)    { h.st.Mode = m }
func (h *recordingHost) PushOverlay(o kernel.Overlay) {}
func (h *recordingHost) PopOverlay()                  {}
func (h *recordingHost) Confirm(req state.ConfirmRequest) {
	h.notifies = append(h.notifies, "confirm:"+req.Title)
}
func (h *recordingHost) Notify(text string) { h.notifies = append(h.notifies, text) }
func (h *recordingHost) Quit()              {}

// newTestPlugin 构造完成 Init 的 chat 插件与配套 fake Host。
func newTestPlugin(t *testing.T) (*Plugin, *recordingHost) {
	t.Helper()
	p := New()
	h := newRecordingHost()
	p.Init(context.Background(), h)
	if p.st == nil || p.stream == nil {
		t.Fatal("Init should fix state pointer and construct stream renderer")
	}
	return p, h
}

func ev(t gateway.EventType, payload map[string]any) gateway.GatewayEvent {
	if payload == nil {
		payload = map[string]any{}
	}
	return gateway.GatewayEvent{Type: t, Payload: payload}
}

func TestPluginIdentity(t *testing.T) {
	p := New()
	if p.ID() != "chat" {
		t.Fatalf("id = %q", p.ID())
	}
	if p.Region() != kernel.RegionStream {
		t.Fatalf("region = %q", p.Region())
	}
}

func TestReactConversationEventsMigrateSlots(t *testing.T) {
	p, _ := newTestPlugin(t)
	// 流式合并：两条 chunk 合并为一条消息。
	p.React(nil, ev(gateway.EventAgentChunk, map[string]any{"text": "hel"}))
	p.React(nil, ev(gateway.EventAgentChunk, map[string]any{"text": "lo"}))
	if len(p.st.Stream) != 1 || p.st.Stream[0].Content != "hello" {
		t.Fatalf("stream = %+v", p.st.Stream)
	}
	// 工具生命周期。
	p.React(nil, ev(gateway.EventToolStart, map[string]any{"tool": "bash", "input": "ls"}))
	if p.st.Stream[1].Type != "tool_start" || p.st.Stream[1].ToolName != "bash" {
		t.Fatalf("tool entry = %+v", p.st.Stream[1])
	}
	p.React(nil, ev(gateway.EventToolEnd, map[string]any{"tool": "bash", "output": "ok"}))
	if p.st.Stream[2].Type != "tool_end" {
		t.Fatal("tool_end missing")
	}
}

func TestReactGatewayOfflineBespoke(t *testing.T) {
	p, _ := newTestPlugin(t)
	p.React(nil, ev(gateway.EventGatewayOffline, map[string]any{"message": "conn refused"}))
	if p.st.Runtime.Phase != state.RuntimePhaseError {
		t.Fatalf("phase = %q", p.st.Runtime.Phase)
	}
	if last := p.st.Stream[len(p.st.Stream)-1]; last.Type != "error" || last.Content != "conn refused" {
		t.Fatalf("error entry = %+v", last)
	}
	// bespoke 不碰 Gateway.Connected（health 插件槽）。
	if p.st.Gateway.Connected {
		t.Fatal("bespoke must not touch Gateway.Connected")
	}
}

func TestReactStreamGrowthResetsScroll(t *testing.T) {
	p, _ := newTestPlugin(t)
	// 预置：用户已上滚（AutoScroll=false，offset=5）。
	p.st.Layout.AutoScroll = false
	p.st.Layout.ScrollOffset = 5
	p.React(nil, ev(gateway.EventAgentChunk, map[string]any{"text": "new"}))
	if !p.st.Layout.AutoScroll || p.st.Layout.ScrollOffset != 0 {
		t.Fatalf("stream growth should reset scroll: auto=%v offset=%d", p.st.Layout.AutoScroll, p.st.Layout.ScrollOffset)
	}
	// 非流增长事件（token_usage）不触发复位。
	p.st.Layout.AutoScroll = false
	p.st.Layout.ScrollOffset = 5
	p.React(nil, ev(gateway.EventTokenUsage, map[string]any{"total": 3}))
	if p.st.Layout.AutoScroll || p.st.Layout.ScrollOffset != 5 {
		t.Fatal("token usage must not reset scroll")
	}
}

func TestReactNonDialogueEventsIgnored(t *testing.T) {
	p, _ := newTestPlugin(t)
	// sessions/models/health 类事件：chat 不处理（槽纪律——由对应插件处理）。
	before := *p.st
	p.React(nil, ev(gateway.EventSessionCreated, map[string]any{"id": "s9"}))
	p.React(nil, ev(gateway.EventModelChanged, map[string]any{"model": "m9"}))
	p.React(nil, ev(gateway.EventHealthChanged, map[string]any{"connected": false}))
	if len(p.st.Stream) != len(before.Stream) || p.st.Gateway.ActiveModel == "m9" || p.st.Gateway.Connected {
		t.Fatal("non-dialogue events must be ignored by chat")
	}
}

func TestReactIgnoresNonGatewayMessages(t *testing.T) {
	p, _ := newTestPlugin(t)
	p.React(nil, state.ConfirmResult{ID: "x"})
	p.React(nil, tea.KeyMsg{Type: tea.KeyEnter})
	if len(p.st.Stream) != 0 {
		t.Fatal("non-gateway messages must be ignored")
	}
}

func TestRenderDelegatesToStream(t *testing.T) {
	p, h := newTestPlugin(t)
	p.React(nil, ev(gateway.EventAgentChunk, map[string]any{"text": "visible line"}))
	out := p.Render(h, 100)
	if !strings.Contains(out, "visible line") {
		t.Fatalf("render = %q", out)
	}
	// 空流渲染不 panic 且输出合法（可为空串）。
	p.st.Stream = nil
	_ = p.Render(h, 100)
}

func TestRenderBeforeInitIsSafe(t *testing.T) {
	p := New()
	if out := p.Render(nil, 100); out != "" {
		t.Fatalf("render before init = %q, want empty", out)
	}
}

func TestBindingsShapeAndScrollSemantics(t *testing.T) {
	p, _ := newTestPlugin(t)
	// 预置流内容（有可滚动余量）。
	for i := 0; i < 30; i++ {
		p.React(nil, ev(gateway.EventAgentMessageStart, map[string]any{"text": "line"}))
		p.React(nil, ev(gateway.EventAgentMessageEnd, map[string]any{}))
	}
	bindings := p.Bindings()
	if len(bindings) != 8 {
		t.Fatalf("bindings = %d, want 8 (j/k/g/G + 4 paging)", len(bindings))
	}
	lookup := map[string]kernel.Binding{}
	for _, b := range bindings {
		if b.Mode != state.NormalMode || b.OnKey == nil {
			t.Fatalf("binding %q: mode/onKey invalid", b.Key)
		}
		lookup[b.Key] = b
	}
	// 合成 KeyMsg 形状锁定：keyRunes("j").String() == "j"（与真实按键等价）。
	if keyRunes("j").String() != "j" || keyRunes("G").String() != "G" {
		t.Fatal("synthetic key shape drifted from real key String()")
	}
	// j：下滚一行 → offset 减、AutoScroll 联动。
	lookup["j"].OnKey(newRecordingHost())
	if p.st.Layout.ScrollOffset != 0 {
		t.Fatalf("offset = %d, want 0 after j from 0", p.st.Layout.ScrollOffset)
	}
	// k：上滚 → offset 增、AutoScroll=false。
	lookup["k"].OnKey(newRecordingHost())
	if p.st.Layout.AutoScroll {
		t.Fatal("k should disable auto scroll")
	}
	// g：顶部（offset=max）。
	lookup["g"].OnKey(newRecordingHost())
	if p.st.Layout.ScrollOffset == 0 {
		t.Fatal("g should jump to max offset")
	}
	// G：底部（offset=0）+ AutoScroll 恢复。
	lookup["G"].OnKey(newRecordingHost())
	if p.st.Layout.ScrollOffset != 0 || !p.st.Layout.AutoScroll {
		t.Fatal("G should return to bottom with auto scroll")
	}
	// 翻页键存在且可执行（行为由组件内建，这里锁可用性）。
	for _, key := range []string{"ctrl+d", "ctrl+u", "ctrl+f", "ctrl+b"} {
		lookup[key].OnKey(newRecordingHost())
	}
}

func TestCommandsRegistry(t *testing.T) {
	p, h := newTestPlugin(t)
	cmds := p.Commands()
	if len(cmds) != 4 {
		t.Fatalf("commands = %d, want 4", len(cmds))
	}
	byName := map[string]kernel.Command{}
	for _, c := range cmds {
		if c.Run == nil || c.Category != "chat" {
			t.Fatalf("command %s invalid", c.Name)
		}
		byName[c.Name] = c
	}
	// /clear：清空流 + 提示。
	p.React(nil, ev(gateway.EventAgentChunk, map[string]any{"text": "x"}))
	byName["/clear"].Run(h, nil)
	if len(p.st.Stream) != 0 || len(h.notifies) != 1 {
		t.Fatalf("clear: stream=%d notifies=%v", len(p.st.Stream), h.notifies)
	}
	// /cancel 空闲态：静默 no-op（无通知）。
	byName["/cancel"].Run(h, nil)
	if len(h.notifies) != 1 {
		t.Fatalf("idle cancel should be silent, notifies=%v", h.notifies)
	}
	// /compact：占位提示。
	byName["/compact"].Run(h, nil)
	if !strings.Contains(h.notifies[len(h.notifies)-1], "compact") {
		t.Fatalf("compact notify = %v", h.notifies)
	}
	// /retry 无历史：诚实降级。
	byName["/retry"].Run(h, nil)
	if !strings.Contains(h.notifies[len(h.notifies)-1], "没有可重试") {
		t.Fatalf("retry notify = %v", h.notifies)
	}
}

func TestCommandCancelWithClient(t *testing.T) {
	p, h := newTestPlugin(t)
	// client 为 nil（fake Host 默认）+ 运行中：应提示取消失败而非 panic。
	p.st.Runtime.Phase = state.RuntimePhaseRunning
	p.st.Runtime.RunID = "run-1"
	byName := map[string]kernel.Command{}
	for _, c := range p.Commands() {
		byName[c.Name] = c
	}
	byName["/cancel"].Run(h, nil)
	if !strings.Contains(h.notifies[len(h.notifies)-1], "无可用后端") {
		t.Fatalf("cancel without client = %v", h.notifies)
	}
	// stale RunID 场景（审计 P1-b）：run 已结束（Phase=idle）但 RunID 残留，
	// 不得误发 CancelRun——静默 no-op。
	p.st.Runtime.Phase = state.RuntimePhaseIdle
	before := len(h.notifies)
	byName["/cancel"].Run(h, nil)
	if len(h.notifies) != before {
		t.Fatal("idle phase with stale RunID must stay silent")
	}
}

func TestRetryArmedAfterSubmission(t *testing.T) {
	p, h := newTestPlugin(t)
	byName := map[string]kernel.Command{}
	for _, c := range p.Commands() {
		byName[c.Name] = c
	}
	// lastText 现由 UserSubmitted 广播更新（recordSubmittedText 已删）。
	p.React(h, state.UserSubmitted{Text: "fix the bug"})
	byName["/retry"].Run(h, nil)
	if strings.Contains(h.notifies[len(h.notifies)-1], "没有可重试") {
		t.Fatal("retry with history should not degrade to no-history hint")
	}
}

// fakeClient 是 gateway.Client 的最小 fake：仅记录 CancelRun 调用，
// 其余方法返回 ErrUnsupported 语义的空实现。
type fakeClient struct {
	cancelCalls []cancelCall
}

// cancelCall 记录一次 CancelRun 的入参（审计 P2：仅计数不校验参数不充分）。
type cancelCall struct {
	sessionID string
	runID     string
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
	c.cancelCalls = append(c.cancelCalls, cancelCall{sessionID: sessionID, runID: runID})
	return nil
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

func TestCommandCancelInvokesClient(t *testing.T) {
	client := &fakeClient{}
	p := New()
	h := newRecordingHost()
	h.client = client // 必须在 Init 前注入：插件在 Init 时记录 Gateway 透传
	p.Init(context.Background(), h)
	p.st.Runtime.Phase = state.RuntimePhaseRunning
	p.st.Runtime.RunID = "run-9"
	p.st.Gateway.ActiveSess = &gateway.SessionSummary{ID: "sess-1"}

	byName := map[string]kernel.Command{}
	for _, c := range p.Commands() {
		byName[c.Name] = c
	}
	byName["/cancel"].Run(h, nil)
	// recordingHost 执行 GoCmd；断言 CancelRun 携带正确参数（审计 P2）。
	if len(client.cancelCalls) != 1 {
		t.Fatalf("cancelCalls = %d, want 1", len(client.cancelCalls))
	}
	if got := client.cancelCalls[0]; got.sessionID != "sess-1" || got.runID != "run-9" {
		t.Fatalf("cancel args = %+v, want sess-1/run-9", got)
	}
}

func TestPluginCloseIsSafe(t *testing.T) {
	p, _ := newTestPlugin(t)
	p.Close(context.Background()) // 无外部资源，对称生命周期
}

func TestStringOfFallbacks(t *testing.T) {
	if got := stringOf(map[string]any{"message": "m"}, "message", "error"); got != "m" {
		t.Fatalf("hit = %q", got)
	}
	if got := stringOf(map[string]any{"other": "m"}, "message", "error"); got != "" {
		t.Fatalf("miss = %q", got)
	}
	if got := stringOf(nil, "message"); got != "" {
		t.Fatalf("nil payload = %q", got)
	}
}

// TestReactUserSubmittedHandover 是审计 P1-1 的闭环断言（issue #25 验收）：
// prompt 插件的 UserSubmitted 广播 → chat 更新 lastText + 追加 role=user
// 流条目 → /retry 激活。
func TestReactUserSubmittedHandover(t *testing.T) {
	p, h := newTestPlugin(t)
	p.React(h, state.UserSubmitted{Text: "fix the login bug"})

	if p.lastText != "fix the login bug" {
		t.Fatalf("lastText = %q", p.lastText)
	}
	last := p.st.Stream[len(p.st.Stream)-1]
	if last.Type != "message" || last.Content != "fix the login bug" {
		t.Fatalf("user entry = %+v", last)
	}
	if last.Metadata["role"] != "user" || last.Metadata["done"] != true {
		t.Fatalf("metadata = %v, want role=user done=true", last.Metadata)
	}
	if p.st.Layout.AutoScroll != true || p.st.Layout.ScrollOffset != 0 {
		t.Fatal("user entry should reset scroll (stream growth)")
	}
	// /retry 激活：不再降级为无历史提示。
	byName := map[string]kernel.Command{}
	for _, c := range p.Commands() {
		byName[c.Name] = c
	}
	byName["/retry"].Run(h, nil)
	if strings.Contains(h.notifies[len(h.notifies)-1], "没有可重试") {
		t.Fatal("retry should be armed after UserSubmitted")
	}
}
