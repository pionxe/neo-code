package prompt

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"neo-code/internal/tuiv2/components"
	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// recordingHost 是插件测试用的 fake Host（Host 第二实现的插件侧复用），
// 同步执行 GoCmd 以观察 RPC 副作用。
var _ kernel.Host = (*recordingHost)(nil)

type recordingHost struct {
	st           *state.ViewState
	notifies     []string
	broadcasts   []tea.Msg
	client       gateway.Client
	quitted      bool
	react        func(tea.Msg) // 广播回插件 React（模拟 kernel 分发；nil 则只记录）
	runCmds      []string      // RunCommand 调用记录（名称）
	runCommandFn func(string, []string) error // 可注入的 RunCommand 行为（nil 返回 nil）
}

func newRecordingHost() *recordingHost {
	return &recordingHost{st: state.NewViewState()}
}

func (h *recordingHost) State() *state.ViewState                        { return h.st }
func (h *recordingHost) Gateway() gateway.Client                        { return h.client }
func (h *recordingHost) Commands() []kernel.Command                     { return nil }
func (h *recordingHost) RunCommand(name string, args []string) error {
	h.runCmds = append(h.runCmds, name)
	if h.runCommandFn != nil {
		return h.runCommandFn(name, args)
	}
	return nil
}
func (h *recordingHost) Bindings() []kernel.Binding                     { return nil }
func (h *recordingHost) BindEventStream(ch <-chan gateway.GatewayEvent) {}
func (h *recordingHost) GoCmd(cmd tea.Cmd) {
	if cmd != nil {
		// 同步执行并把产物回流广播——模拟 bubbletea 的
		// "命令执行 → 消息回流 Update → 广播"真实链路。
		if msg := cmd(); msg != nil {
			h.Send(msg)
		}
	}
}
func (h *recordingHost) Send(msg tea.Msg) {
	h.broadcasts = append(h.broadcasts, msg)
	if h.react != nil {
		h.react(msg) // 模拟内核广播：消息回流插件 React
	}
}
func (h *recordingHost) Mode() state.InputMode        { return h.st.Mode }
func (h *recordingHost) SetMode(m state.InputMode)    { h.st.Mode = m }
func (h *recordingHost) PushOverlay(o kernel.Overlay) {}
func (h *recordingHost) PopOverlay()                  {}
func (h *recordingHost) Confirm(req state.ConfirmRequest) {
	h.notifies = append(h.notifies, "confirm:"+req.Title)
}
func (h *recordingHost) Notify(text string) { h.notifies = append(h.notifies, text) }
func (h *recordingHost) Quit()              { h.quitted = true }

func newTestPlugin(t *testing.T) (*Plugin, *recordingHost) {
	t.Helper()
	p := New()
	h := newRecordingHost()
	p.Init(context.Background(), h)
	if p.st == nil || p.prompt == nil {
		t.Fatal("Init should fix state pointer and construct prompt renderer")
	}
	return p, h
}

func TestPluginIdentity(t *testing.T) {
	p := New()
	if p.ID() != "prompt" || p.Region() != kernel.RegionPrompt {
		t.Fatalf("identity = %q/%q", p.ID(), p.Region())
	}
}

func TestReactInputWritingEvents(t *testing.T) {
	p, h := newTestPlugin(t)
	// 六类临时越权事件的 Input 部分现在由 prompt 承接。
	p.React(h, ev(gateway.EventPermissionRequested, map[string]any{"prompt": "allow bash?"}))
	if p.st.Input.Mode != state.InputStateModePermissionResponse || p.st.Input.Prompt != "allow bash?" {
		t.Fatalf("input = %+v", p.st.Input)
	}
	p.React(h, ev(gateway.EventPermissionResolved, map[string]any{"decision": "allow"}))
	if p.st.Input.Mode != state.InputStateModeMessage || p.st.Input.Prompt != "" {
		t.Fatalf("input after resolve = %+v", p.st.Input)
	}
	p.React(h, ev(gateway.EventAskUserQuestion, map[string]any{"question": "Q", "options": []any{"a", "b"}}))
	if p.st.Input.Mode != state.InputStateModeQuestionAnswer || len(p.st.Input.Options) != 2 {
		t.Fatalf("input = %+v", p.st.Input)
	}
	p.React(h, ev(gateway.EventUserQuestionAnswered, map[string]any{"answer": "a"}))
	if p.st.Input.Mode != state.InputStateModeMessage || p.st.Input.Text != "" {
		t.Fatalf("input after answered = %+v", p.st.Input)
	}
}

func TestReactApplyInputNoopForOthers(t *testing.T) {
	p, h := newTestPlugin(t)
	before := *p.st
	p.React(h, ev(gateway.EventAgentChunk, map[string]any{"text": "x"}))
	p.React(h, ev(gateway.EventSessionCreated, map[string]any{"id": "s"}))
	if p.st.Input.Mode != before.Input.Mode || p.st.Input.Prompt != before.Input.Prompt {
		t.Fatal("non-input-writing events must not touch Input via prompt")
	}
}

func TestSubmitFlowHappyPath(t *testing.T) {
	p, h := newTestPlugin(t)
	client := &fakeClient{}
	h.client = client
	p.Init(context.Background(), h) // 重新注入 client
	p.st.Gateway.ActiveSess = &gateway.SessionSummary{ID: "sess-1"}
	p.st.Input.Text = "  fix the bug  "

	// 模拟组件产出的提交消息（真实路径：Enter 键经通配委托组件 Update 产出）。
	p.React(h, components.SubmitMessageMsg{Text: "  fix the bug  "})
	// Init 的 blink 续订经回流广播是已知噪声，断言按类型过滤。
	var us state.UserSubmitted
	found := false
	for _, b := range h.broadcasts {
		if u, ok := b.(state.UserSubmitted); ok {
			us, found = u, true
		}
	}
	if !found || us.Text != "fix the bug" {
		t.Fatalf("UserSubmitted missing among %d broadcasts", len(h.broadcasts))
	}
	if len(client.sendCalls) != 1 || client.sendCalls[0] != "sess-1|fix the bug" {
		t.Fatalf("sendCalls = %v", client.sendCalls)
	}
}

func TestSubmitEmptyTextIgnored(t *testing.T) {
	p, h := newTestPlugin(t)
	p.React(h, components.SubmitMessageMsg{Text: "   "})
	for _, b := range h.broadcasts {
		if _, ok := b.(state.UserSubmitted); ok {
			t.Fatal("empty submit must not produce UserSubmitted")
		}
	}
}

func TestSubmitWithoutSessionOrClientDegrades(t *testing.T) {
	p, h := newTestPlugin(t)
	client := &fakeClient{}
	h.client = client
	p.Init(context.Background(), h) // 注入 client 后测"无活跃会话"
	p.React(h, components.SubmitMessageMsg{Text: "x"})
	if !strings.Contains(h.notifies[len(h.notifies)-1], "无活跃会话") {
		t.Fatalf("no-session notify = %v", h.notifies)
	}
	// 无 client（client 是 Init 时透传的，置回 nil 需重新 Init）。
	h.client = nil
	p.Init(context.Background(), h)
	p.st.Gateway.ActiveSess = &gateway.SessionSummary{ID: "s"}
	p.React(h, components.SubmitMessageMsg{Text: "x"})
	if !strings.Contains(h.notifies[len(h.notifies)-1], "无可用后端") {
		t.Fatalf("no-client notify = %v", h.notifies)
	}
}

func TestPermissionAndQuestionRPCParams(t *testing.T) {
	p, h := newTestPlugin(t)
	client := &fakeClient{}
	h.client = client
	p.Init(context.Background(), h)
	p.st.Gateway.ActiveSess = &gateway.SessionSummary{ID: "sess-1"}
	p.st.Runtime.RunID = "run-1"

	p.React(h, components.PermissionActionMsg{Decision: "y"})
	if len(client.permDecisions) != 1 {
		t.Fatalf("permDecisions = %d", len(client.permDecisions))
	}
	d := client.permDecisions[0]
	if !d.Allow || d.SessionID != "sess-1" || d.RunID != "run-1" || d.Reason != "y" {
		t.Fatalf("decision = %+v", d)
	}
	p.React(h, components.QuestionAnswerMsg{Text: "2"})
	if len(client.answers) != 1 || client.answers[0].Text != "2" || client.answers[0].RunID != "run-1" {
		t.Fatalf("answers = %+v", client.answers)
	}
}

func TestExitCommandQuits(t *testing.T) {
	p, h := newTestPlugin(t)
	byName := map[string]kernel.Command{}
	for _, c := range p.Commands() {
		byName[c.Name] = c
	}
	byName["/exit"].Run(h, nil)
	if !h.quitted {
		t.Fatal("/exit should quit")
	}
}

// TestModeCommandTogglesAgentMode 验证 /mode 命令（issue #41 接线补齐）：
// 空值或 plan → build、build → plan（对齐旧 toggleAgentMode 语义），
// 并经弱提示反馈 "Agent mode: <mode>"；别名含无斜杠 "mode"（:mode 可达）。
func TestModeCommandTogglesAgentMode(t *testing.T) {
	p, h := newTestPlugin(t)
	byName := map[string]kernel.Command{}
	for _, c := range p.Commands() {
		byName[c.Name] = c
	}
	mode, ok := byName["/mode"]
	if !ok {
		t.Fatal("/mode should be registered")
	}
	if len(mode.Aliases) != 1 || mode.Aliases[0] != "mode" {
		t.Fatalf("aliases = %v, want [mode]", mode.Aliases)
	}
	// 空值 → build。
	mode.Run(h, nil)
	if h.st.Runtime.AgentMode != state.AgentModeBuild {
		t.Fatalf("from empty: mode = %q, want build", h.st.Runtime.AgentMode)
	}
	// build → plan。
	mode.Run(h, nil)
	if h.st.Runtime.AgentMode != state.AgentModePlan {
		t.Fatalf("from build: mode = %q, want plan", h.st.Runtime.AgentMode)
	}
	// plan → build。
	mode.Run(h, nil)
	if h.st.Runtime.AgentMode != state.AgentModeBuild {
		t.Fatalf("from plan: mode = %q, want build", h.st.Runtime.AgentMode)
	}
	// 弱提示反馈逐次断言。
	if len(h.notifies) != 3 ||
		h.notifies[0] != "Agent mode: build" ||
		h.notifies[1] != "Agent mode: plan" ||
		h.notifies[2] != "Agent mode: build" {
		t.Fatalf("notifies = %v", h.notifies)
	}
}

// TestExitQuitAliasesComplete 验证 /exit 别名表完整性（issue #41 审计
// r3 别名完备性 F3）：无斜杠 "exit"/"q"/"quit" 通 Ex 行（:q/:quit/:exit），
// 带斜杠 "/quit" 通 slash 路径（带斜杠按名查表）。
func TestExitQuitAliasesComplete(t *testing.T) {
	p, _ := newTestPlugin(t)
	for _, c := range p.Commands() {
		if c.Name != "/exit" {
			continue
		}
		want := map[string]bool{"exit": true, "q": true, "quit": true, "/quit": true}
		if len(c.Aliases) != len(want) {
			t.Fatalf("aliases = %v, want %v", c.Aliases, want)
		}
		for _, a := range c.Aliases {
			if !want[a] {
				t.Fatalf("unexpected alias %q in %v", a, c.Aliases)
			}
		}
		return
	}
	t.Fatal("/exit should be registered")
}

// TestSlashCommandRoutesToRegistry 验证 SlashCommandMsg 路由（issue #41
// 审计 P1-2——此前 kernel 路径对该消息零消费者）：已知命令转 RunCommand
// （名称与参数透传）、未知命令弱提示、空命令 no-op。
func TestSlashCommandRoutesToRegistry(t *testing.T) {
	p, h := newTestPlugin(t)
	// 已知命令：名称透传 RunCommand。
	p.React(h, components.SlashCommandMsg{Command: "/mode"})
	if len(h.runCmds) != 1 || h.runCmds[0] != "/mode" {
		t.Fatalf("runCmds = %v, want [/mode]", h.runCmds)
	}
	// 带参数：参数按空白拆分透传（对齐 CommandPrompt.Cut 的 args 语义）。
	h.runCmds = nil
	p.React(h, components.SlashCommandMsg{Command: "/theme", Args: " tokyo-night extra "})
	if len(h.runCmds) != 1 || h.runCmds[0] != "/theme" {
		t.Fatalf("runCmds = %v, want [/theme]", h.runCmds)
	}
	// 未知命令：弱提示呈现（文案对齐旧路径 "unknown command: %s"）。
	h.runCmds, h.notifies = nil, nil
	h.runCommandFn = func(string, []string) error { return fmt.Errorf("unknown") }
	p.React(h, components.SlashCommandMsg{Command: "/nope"})
	if len(h.notifies) != 1 || h.notifies[0] != "unknown command: /nope" {
		t.Fatalf("notifies = %v", h.notifies)
	}
	// 空命令：no-op（不路由、不提示）。
	h.runCmds, h.notifies = nil, nil
	p.React(h, components.SlashCommandMsg{})
	if len(h.runCmds) != 0 || len(h.notifies) != 0 {
		t.Fatalf("empty command should be no-op, runCmds=%v notifies=%v", h.runCmds, h.notifies)
	}
}

// TestInputBindingWhenGuard 验证 i 键 When 守卫（PR #43 审计 P1-1）：
// 搜索/Ex 激活期 "i" 属 cmdline 输入字符，不得切入输入模式。
func TestInputBindingWhenGuard(t *testing.T) {
	p := New()
	for _, b := range p.Bindings() {
		if b.Key != "i" {
			continue
		}
		if b.When == nil {
			t.Fatal("i binding should carry When guard")
		}
		clean := state.NewViewState()
		if !b.When(clean) {
			t.Fatal("i should enter input in normal navigation")
		}
		searching := state.NewViewState()
		searching.Search.Active = true
		if b.When(searching) {
			t.Fatal("i must yield during search")
		}
		ex := state.NewViewState()
		ex.Ex.Active = true
		if b.When(ex) {
			t.Fatal("i must yield during ex")
		}
		return
	}
	t.Fatal("i binding should exist")
}

func TestBindingsShape(t *testing.T) {
	p, _ := newTestPlugin(t)
	bindings := p.Bindings()
	if len(bindings) != 3 {
		t.Fatalf("bindings = %d, want 3 (esc + wildcard + i)", len(bindings))
	}
	wildcards, precise := 0, 0
	for _, b := range bindings {
		if b.Key == "" && b.OnKeyMsg != nil {
			wildcards++
			if b.Mode != state.InputModeInput || b.Description == "" {
				t.Fatal("wildcard must be input-mode with description")
			}
			continue
		}
		precise++
		if b.OnKey == nil {
			t.Fatalf("precise binding %q missing OnKey", b.Key)
		}
	}
	if wildcards != 1 || precise != 2 {
		t.Fatalf("wildcards=%d precise=%d, want 1/2", wildcards, precise)
	}
}

func TestWildcardDelegatesToComponent(t *testing.T) {
	p, h := newTestPlugin(t)
	// Input 模式下通配委托：字符插入（组件 default 分支）。
	p.st.Input.Text = ""
	p.st.Input.Cursor = 0
	p.st.Mode = state.InputModeInput
	bindings := p.Bindings()
	var wildcard kernel.Binding
	for _, b := range bindings {
		if b.Key == "" && b.OnKeyMsg != nil {
			wildcard = b
		}
	}
	wildcard.OnKeyMsg(newRecordingHost(), keyRunes("h"))
	if p.st.Input.Text != "h" {
		t.Fatalf("text = %q, want h (delegated insertion)", p.st.Input.Text)
	}
	// esc 精确绑定：模式切换（SetMode 写原 host 的 Mode 槽）。
	for _, b := range bindings {
		if b.Key == "esc" {
			b.OnKey(h)
		}
	}
	if p.st.Mode != state.NormalMode {
		t.Fatalf("mode = %v, want normal after esc", p.st.Mode)
	}
	// Normal 模式 i 精确绑定：回 Input。
	for _, b := range bindings {
		if b.Key == "i" {
			b.OnKey(h)
		}
	}
	if p.st.Mode != state.InputModeInput {
		t.Fatalf("mode = %v, want input after i", p.st.Mode)
	}
}

func TestCursorBlinkContinuation(t *testing.T) {
	p, h := newTestPlugin(t)
	before := p.st.Input.CursorVisible
	p.React(h, components.CursorBlinkMsg{})
	if p.st.Input.CursorVisible == before {
		t.Fatal("blink should toggle cursor visibility")
	}
}

func TestRenderDelegatesToPrompt(t *testing.T) {
	p, h := newTestPlugin(t)
	out := p.Render(h, 100)
	if !strings.Contains(out, "[input]") {
		t.Fatalf("render = %q, want mode indicator", out)
	}
	p2 := New()
	if out := p2.Render(nil, 100); out != "" {
		t.Fatalf("render before init = %q", out)
	}
}

// ---------- 桩 ----------

func ev(t gateway.EventType, payload map[string]any) gateway.GatewayEvent {
	if payload == nil {
		payload = map[string]any{}
	}
	return gateway.GatewayEvent{Type: t, Payload: payload}
}

func keyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// fakeClient 记录 RPC 入参并支持注入成功/失败行为（断言 ACK/Error 映射）。
type fakeClient struct {
	sendCalls     []string
	permDecisions []gateway.PermissionDecision
	answers       []gateway.UserQuestionAnswer
	// 可注入行为：nil = 失败（ErrUnsupported）；非 nil = 成功返回该 ACK。
	sendAck   *gateway.RunAck
	permErr   error // 非 nil = ResolvePermission 返回该错误
	answerErr error // 非 nil = AnswerUserQuestion 返回该错误
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
	c.sendCalls = append(c.sendCalls, sessionID+"|"+text)
	if c.sendAck != nil {
		return c.sendAck, nil
	}
	return nil, errFakeUnsupported
}
func (c *fakeClient) CancelRun(ctx context.Context, sessionID, runID string) error {
	return errFakeUnsupported
}
func (c *fakeClient) SubscribeEvents(ctx context.Context, sessionID string) (<-chan gateway.GatewayEvent, error) {
	return nil, errFakeUnsupported
}
func (c *fakeClient) ResolvePermission(ctx context.Context, decision gateway.PermissionDecision) error {
	c.permDecisions = append(c.permDecisions, decision)
	return c.permErr
}
func (c *fakeClient) AnswerUserQuestion(ctx context.Context, answer gateway.UserQuestionAnswer) error {
	c.answers = append(c.answers, answer)
	return c.answerErr
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

func TestPromptCloseIsSafe(t *testing.T) {
	p, _ := newTestPlugin(t)
	p.Close(context.Background()) // 无外部资源，对称生命周期
}

func TestDelegateUpdateNilPromptSafe(t *testing.T) {
	p := New()
	p.delegateUpdate(newRecordingHost(), keyRunes("x")) // prompt 未构造 → 静默
}

func TestHandlePermissionAndQuestionNilClientSilent(t *testing.T) {
	p, h := newTestPlugin(t) // client = nil（fake Host 默认）
	p.st.Gateway.ActiveSess = &gateway.SessionSummary{ID: "s"}
	before := len(h.notifies)
	p.handlePermission(h, components.PermissionActionMsg{Decision: "y"})
	p.handleQuestion(h, components.QuestionAnswerMsg{Text: "1"})
	if len(h.notifies) != before {
		t.Fatal("nil client should be silent (no notify path in permission/question)")
	}
}

func TestReactPromptCancelNotifies(t *testing.T) {
	p, h := newTestPlugin(t)
	p.React(h, components.PromptCancelMsg{})
	if !strings.Contains(h.notifies[len(h.notifies)-1], "已取消当前输入") {
		t.Fatalf("notifies = %v", h.notifies)
	}
}

func TestDelegateUpdateComponentReturnsNilCmd(t *testing.T) {
	p, h := newTestPlugin(t)
	// esc 在 Input.Mode=message 下组件返回 nil 命令 → delegateUpdate 的
	// cmd==nil 分支（不 GoCmd）。
	p.st.Input.Mode = state.InputStateModeMessage
	p.delegateUpdate(h, tea.KeyMsg{Type: tea.KeyEsc})
}

func TestDelegateUpdateProducesCmdInPermissionMode(t *testing.T) {
	// 权限模式下 y 经通配委托 → 组件产出 PermissionActionMsg 命令（非 nil）
	// → delegateUpdate 的 GoCmd 分支 → 命令产物回流广播 → React → fakeClient
	// 记录决策（完整模拟 kernel 的"命令执行→消息回流→广播"链路）。
	client := &fakeClient{}
	p := New()
	h := newRecordingHost()
	h.client = client
	p.Init(context.Background(), h)
	h.react = func(msg tea.Msg) { p.React(h, msg) }
	p.st.Gateway.ActiveSess = &gateway.SessionSummary{ID: "sess-1"}
	p.st.Runtime.RunID = "run-1"
	p.st.Input.Mode = state.InputStateModePermissionResponse
	p.delegateUpdate(h, keyRunes("y"))
	if len(client.permDecisions) != 1 || !client.permDecisions[0].Allow {
		t.Fatalf("permDecisions = %+v", client.permDecisions)
	}
}

// TestSubmitAckMapsToRunStartedEvent 断言 ACK → EventRunStarted 映射输出
// （审计第 3 轮 P1：映射三分支此前零覆盖）。
func TestSubmitAckMapsToRunStartedEvent(t *testing.T) {
	p, h := newTestPlugin(t)
	client := &fakeClient{}
	h.client = client
	p.Init(context.Background(), h)
	client.sendAck = &gateway.RunAck{SessionID: "sess-1", RunID: "run-7", Message: "accepted", Accepted: true}
	p.st.Gateway.ActiveSess = &gateway.SessionSummary{ID: "sess-1"}
	p.st.Input.Text = "do it"

	p.React(h, components.SubmitMessageMsg{Text: "do it"})
	// 回流广播链：GoCmd 产物 EventRunStarted + Send 的 UserSubmitted。
	var started *gateway.GatewayEvent
	var submitted state.UserSubmitted
	for _, b := range h.broadcasts {
		switch m := b.(type) {
		case gateway.GatewayEvent:
			if m.Type == gateway.EventRunStarted {
				started = &m
			}
		case state.UserSubmitted:
			submitted = m
		}
	}
	if started == nil {
		t.Fatal("ACK should map to EventRunStarted broadcast")
	}
	if started.SessionID != "sess-1" || started.RunID != "run-7" || started.Payload["accepted"] != true {
		t.Fatalf("EventRunStarted = %+v", started)
	}
	if submitted.Text != "do it" {
		t.Fatalf("UserSubmitted = %+v", submitted)
	}
}

// TestPermissionErrorMapsToErrorEvent 断言 ResolvePermission 失败 → EventError。
func TestPermissionErrorMapsToErrorEvent(t *testing.T) {
	p, h := newTestPlugin(t)
	client := &fakeClient{}
	h.client = client
	p.Init(context.Background(), h)
	client.permErr = errFake("perm boom")
	p.st.Gateway.ActiveSess = &gateway.SessionSummary{ID: "s"}
	p.st.Runtime.RunID = "r"
	p.st.Input.Mode = state.InputStateModePermissionResponse

	p.React(h, components.PermissionActionMsg{Decision: "n"})
	found := false
	for _, b := range h.broadcasts {
		if m, ok := b.(gateway.GatewayEvent); ok && m.Type == gateway.EventError {
			found = true
			if m.Payload["message"] != "perm boom" {
				t.Fatalf("error payload = %v", m.Payload)
			}
		}
	}
	if !found {
		t.Fatal("perm error should map to EventError broadcast")
	}
}

// TestQuestionErrorMapsToErrorEvent 断言 AnswerUserQuestion 失败 → EventError。
func TestQuestionErrorMapsToErrorEvent(t *testing.T) {
	p, h := newTestPlugin(t)
	client := &fakeClient{}
	h.client = client
	p.Init(context.Background(), h)
	client.answerErr = errFake("answer boom")
	p.st.Gateway.ActiveSess = &gateway.SessionSummary{ID: "s"}
	p.st.Input.Mode = state.InputStateModeQuestionAnswer

	p.React(h, components.QuestionAnswerMsg{Text: "1"})
	found := false
	for _, b := range h.broadcasts {
		if m, ok := b.(gateway.GatewayEvent); ok && m.Type == gateway.EventError {
			found = true
		}
	}
	if !found {
		t.Fatal("answer error should map to EventError broadcast")
	}
}
