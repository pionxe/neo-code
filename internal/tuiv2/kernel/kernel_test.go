package kernel

import (
	"context"
	"strings"
	"testing"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// captureReactor 是可编程的广播捕获器：record 记录消息序，onMsg 可注入行为
// （如"React 内再 Send"用于重入测试）。
type captureReactor struct {
	id     string
	record []tea.Msg
	onMsg  func(h Host, msg tea.Msg)
}

func (c *captureReactor) ID() string                       { return c.id }
func (c *captureReactor) Init(ctx context.Context, h Host) {}
func (c *captureReactor) Close(ctx context.Context)        {}
func (c *captureReactor) React(h Host, msg tea.Msg) {
	c.record = append(c.record, msg)
	if c.onMsg != nil {
		c.onMsg(h, msg)
	}
}

// newTestKernel 构造带捕获器的最小内核（超时参数取极小值便于测试）。
func newTestKernel(t *testing.T) (*Kernel, *captureReactor) {
	t.Helper()
	k := NewKernel(Config{Opts: Options{LeaderTimeout: 1, NotifyExpiry: 1, MaxQueueDepth: 8}})
	r := &captureReactor{id: "capture"}
	if err := k.Register(r); err != nil {
		t.Fatalf("register: %v", err)
	}
	return k, r
}

// keys 构造按键消息。
func keys(s string) tea.KeyMsg { return keyRunes(s) }

func TestReservedCtrlCQuitsAndNeverBroadcasts(t *testing.T) {
	k, r := newTestKernel(t)
	updated, cmd := k.Update(keys("ctrl+c"))
	if len(r.record) != 0 {
		t.Fatalf("ctrl+c must never broadcast, reactors saw %d", len(r.record))
	}
	if updated.(*Kernel) != k || cmd == nil {
		t.Fatal("ctrl+c should request quit (non-nil cmd)")
	}
}

func TestKeyMsgSentByPluginIsDropped(t *testing.T) {
	k, r := newTestKernel(t)
	var hostDuringReact Host
	echoer := &captureReactor{id: "echo", onMsg: func(h Host, msg tea.Msg) {
		hostDuringReact = h
		// 在 React 内广播一条 KeyMsg 形状的消息：内核入口即拒绝。
		h.Send(keys("x"))
	}}
	mustOK(t, k.Register(echoer), "echoer")
	k.Update(state.ConfirmResult{ID: "trigger"})
	if hostDuringReact == nil {
		t.Fatal("reactor should have run")
	}
	for _, msg := range r.record {
		if _, bad := msg.(tea.KeyMsg); bad {
			t.Fatalf("KeyMsg reached broadcast: %v", msg)
		}
	}
}

func TestMouseMsgNeverBroadcast(t *testing.T) {
	k, r := newTestKernel(t)
	k.Update(tea.MouseMsg{Type: tea.MouseLeft})
	if len(r.record) != 0 {
		t.Fatalf("mouse msg must never broadcast, reactors saw %d", len(r.record))
	}
}

func TestPrivateMessagesNeverBroadcast(t *testing.T) {
	k, r := newTestKernel(t)
	k.Update(leaderTimeoutMsg{gen: 999})
	k.Update(notifyExpiryMsg{gen: 999})
	k.Update(eventStreamClosedMsg{})
	if len(r.record) != 0 {
		t.Fatalf("private messages must never broadcast, reactors saw %d", len(r.record))
	}
}

func TestBroadcastDeliversGatewayEventAndPluginMessage(t *testing.T) {
	k, r := newTestKernel(t)
	ev := gateway.GatewayEvent{Type: gateway.EventRunStarted}
	k.Update(ev)
	k.Update(state.ConfirmResult{ID: "c1", Yes: true})
	if len(r.record) != 2 {
		t.Fatalf("reactors saw %d messages, want 2", len(r.record))
	}
	if _, ok := r.record[0].(gateway.GatewayEvent); !ok {
		t.Fatalf("first = %T, want GatewayEvent", r.record[0])
	}
	if res, ok := r.record[1].(state.ConfirmResult); !ok || !res.Yes {
		t.Fatalf("second = %v, want ConfirmResult yes", r.record[1])
	}
}

func TestReactSendIsDeferredInOrder(t *testing.T) {
	k := NewKernel(Config{Opts: Options{MaxQueueDepth: 8}})
	var order []string
	// echoer 在处理 "first" 时通过 Send 发出 "second"：必须延后到两条 first 类消息之后。
	echoer := &captureReactor{id: "echoer", onMsg: func(h Host, msg tea.Msg) {
		if s, ok := msg.(plainMsg); ok && s == "first" {
			h.Send(plainMsg("second"))
		}
		order = append(order, msg.(plainMsg).string())
	}}
	witness := &captureReactor{id: "witness", onMsg: func(h Host, msg tea.Msg) {
		order = append(order, msg.(plainMsg).string())
	}}
	mustOK(t, k.Register(echoer), "echoer")
	mustOK(t, k.Register(witness), "witness")

	k.Update(plainMsg("first"))
	k.Update(plainMsg("first"))
	// 两个 Reactor 都记录：每条消息产生 2 条记录；second 由 echoer 在处理
	// 第一条 first 时 Send、入队尾延后，在该轮 drain 内排空——保序 + 禁嵌套。
	want := "first,first,second,second,first,first,second,second"
	if got := strings.Join(order, ","); got != want {
		t.Fatalf("order = %q, want %q", got, want)
	}
}

// plainMsg 是插件间消息的最小形态。
type plainMsg string

func (p plainMsg) string() string { return string(p) }

func TestQueueBudgetDropsSelfEcho(t *testing.T) {
	k := NewKernel(Config{Opts: Options{MaxQueueDepth: 4}})
	count := 0
	echoer := &captureReactor{id: "echo", onMsg: func(h Host, msg tea.Msg) {
		count++
		h.Send(plainMsg("echo")) // 自激回声
	}}
	mustOK(t, k.Register(echoer), "echoer")
	k.Update(plainMsg("seed"))
	// 派发预算 4：种子 + 3 条回声后预算耗尽、余量清空——
	// 这是无限自激循环的最终守卫（仅队列深度上限挡不住"清空再填一"）。
	if count != 4 {
		t.Fatalf("reactor invoked %d times, want exactly 4 (budget exhausted)", count)
	}
}

func TestKeyRoutingThreeLevels(t *testing.T) {
	var hits []string
	k := NewKernel(Config{})
	mustOK(t, k.Register(keyPlugin{id: "nav", bindings: []Binding{
		{Mode: state.NormalMode, Key: "j", Description: "下滚", OnKey: func(h Host) { hits = append(hits, "j") }},
		{Mode: state.LeaderMode, Key: "p", Description: "面板", OnKey: func(h Host) { hits = append(hits, "leader-p") }},
	}}), "nav")

	// Leader 未命中键 → 静默回落 Normal（不命中任何绑定、不广播）。
	k.setMode(state.LeaderMode)
	k.dispatchKey(keyRunes("x"))
	if k.modes.mode != state.NormalMode {
		t.Fatal("unmatched leader key should fall back to normal")
	}
	if len(hits) != 0 {
		t.Fatalf("hits = %v, want none", hits)
	}
	// Normal 命中 j。
	k.dispatchKey(keyRunes("j"))
	if len(hits) != 1 || hits[0] != "j" {
		t.Fatalf("hits = %v, want [j]", hits)
	}
	// 未命中键被丢弃。
	k.dispatchKey(keyRunes("zz"))
	if len(hits) != 1 {
		t.Fatalf("unmatched key should be dropped, hits = %v", hits)
	}
}

func TestOverlayTopExclusiveAndEscSemantics(t *testing.T) {
	k := NewKernel(Config{})
	k.setMode(state.NormalMode) // 绑定注册在 Normal 模式（内核初始为 Input 模式）
	var hits []string
	mustOK(t, k.Register(keyPlugin{id: "nav", bindings: []Binding{
		{Mode: state.NormalMode, Key: "j", OnKey: func(h Host) { hits = append(hits, "j") }},
	}}), "nav")

	// 消费型浮层：esc 被消费（不弹栈），其他键也到不了绑定。
	consumer := &scriptedOverlay{consume: true}
	k.stack.push(consumer)
	k.dispatchKey(tea.KeyMsg{Type: tea.KeyEsc})
	if k.stack.depth() != 1 {
		t.Fatal("consumed esc should not pop")
	}
	k.dispatchKey(keyRunes("j"))
	if len(hits) != 0 {
		t.Fatal("overlay top is exclusive, binding must not fire")
	}
	// 非消费型浮层：esc 未消费 → 弹栈。
	k.stack.push(&scriptedOverlay{consume: false})
	k.dispatchKey(tea.KeyMsg{Type: tea.KeyEsc})
	if k.stack.depth() != 1 { // 弹出的是第二层，第一层（消费型）仍在
		t.Fatalf("depth = %d, want 1", k.stack.depth())
	}
	// 弹掉剩余浮层后绑定恢复可达。
	k.stack.pop()
	k.dispatchKey(keyRunes("j"))
	if len(hits) != 1 {
		t.Fatalf("binding should fire after overlays gone, hits = %v", hits)
	}
}

// scriptedOverlay 是按脚本应答的浮层桩：consume 决定 HandleKey 返回值。
type scriptedOverlay struct{ consume bool }

func (o *scriptedOverlay) ID() string                        { return "scripted" }
func (o *scriptedOverlay) HandleKey(h Host, msg tea.KeyMsg) bool { return o.consume }
func (o *scriptedOverlay) View(h Host, width int) string     { return "scripted" }

func TestNotifyGenerationalExpiry(t *testing.T) {
	k := NewKernel(Config{Opts: Options{NotifyExpiry: 1}})
	k.host.Notify("first")
	if k.st.Notify.Text != "first" {
		t.Fatalf("notify = %q", k.st.Notify.Text)
	}
	k.host.Notify("second") // gen 递增
	k.Update(notifyExpiryMsg{gen: k.notifyGen - 1})
	if k.st.Notify.Text != "second" {
		t.Fatal("stale expiry must not clear newer notify")
	}
	k.Update(notifyExpiryMsg{gen: k.notifyGen})
	if k.st.Notify.Text != "" {
		t.Fatalf("notify = %q, want cleared", k.st.Notify.Text)
	}
}

func TestConfirmFlowYesAndNo(t *testing.T) {
	k, r := newTestKernel(t)
	k.host.Confirm(state.ConfirmRequest{Title: "⚠ 删除", Message: "不可撤销", Action: "act_x", Data: map[string]any{"id": "s1"}})
	if k.stack.depth() != 1 {
		t.Fatal("confirm should push kernel confirm overlay")
	}
	if !strings.Contains(k.View(), "删除") {
		t.Fatal("overlay view should replace main layout")
	}
	// 未知键被模态消费，不产生应答。
	k.Update(keys("x"))
	if len(r.record) != 0 {
		t.Fatalf("unknown key in confirm should not answer, saw %d", len(r.record))
	}
	// y → 确认结果广播 + 按对象身份自删。
	k.Update(keys("y"))
	if len(r.record) != 1 {
		t.Fatalf("expected one ConfirmResult, saw %d", len(r.record))
	}
	res := r.record[0].(state.ConfirmResult)
	if !res.Yes || res.Action != "act_x" || res.Data["id"] != "s1" || res.ID == "" {
		t.Fatalf("result = %+v", res)
	}
	if k.stack.depth() != 0 {
		t.Fatalf("stack depth = %d, want empty after answer", k.stack.depth())
	}
	// n → 取消路径（重新发起后按 n）。
	k.host.Confirm(state.ConfirmRequest{Title: "⚠ 再次确认", Action: "a2"})
	k.Update(keys("n"))
	res2 := r.record[1].(state.ConfirmResult)
	if res2.Yes {
		t.Fatal("n should answer no")
	}
	if k.stack.depth() != 0 {
		t.Fatal("overlay should be popped after answer")
	}
}

// TestConfirmAnswerDoesNotPopForeignOverlay 是 P2 回归守卫：
// 应答广播期间 Reactor 压入的新浮层，不得被确认框的自关误弹。
func TestConfirmAnswerDoesNotPopForeignOverlay(t *testing.T) {
	k, _ := newTestKernel(t)
	k.host.Confirm(state.ConfirmRequest{Title: "T", Action: "a1"})
	k.host.Confirm(state.ConfirmRequest{Title: "T2", Action: "a2"})
	// 栈序：[confirm1, confirm2]；处理 confirm2 的应答时广播结果，
	// Reactor 收到结果后压入新浮层——该浮层必须存活。
	pushOnResult := &captureReactor{id: "pusher", onMsg: func(h Host, msg tea.Msg) {
		if res, ok := msg.(state.ConfirmResult); ok && res.Action == "a2" {
			h.PushOverlay(namedOverlay{id: "reactor-overlay"})
		}
	}}
	mustOK(t, k.Register(pushOnResult), "pusher")
	k.Update(keys("y")) // 顶层 confirm2 应答：广播 + 仅移除 confirm2
	if k.stack.depth() != 2 {
		t.Fatalf("stack depth = %d, want 2 (confirm1 + reactor overlay)", k.stack.depth())
	}
	if k.stack.top().ID() != "reactor-overlay" {
		t.Fatalf("top = %s, want reactor-overlay", k.stack.top().ID())
	}
}

func TestEventPumpLifecycle(t *testing.T) {
	k, r := newTestKernel(t)
	ch := make(chan gateway.GatewayEvent, 4)
	k.BindEventStream(ch)

	cmd := k.Init()
	if cmd == nil {
		t.Fatal("Init should arm event pump")
	}
	ch <- gateway.GatewayEvent{Type: gateway.EventRunStarted}
	msg := cmd()
	env, ok := msg.(gatewayEventEnvelope)
	if !ok {
		t.Fatalf("pump delivered %T, want gatewayEventEnvelope", msg)
	}
	if env.event.Type != gateway.EventRunStarted {
		t.Fatalf("envelope payload = %v", env.event)
	}
	// 经 Update 广播裸事件给 Reactor，并重挂泵。
	k.Update(msg)
	if len(r.record) != 1 {
		t.Fatalf("event should broadcast, saw %d", len(r.record))
	}
	if ev, ok := r.record[0].(gateway.GatewayEvent); !ok || ev.Type != gateway.EventRunStarted {
		t.Fatalf("broadcast payload = %v, want bare GatewayEvent", r.record[0])
	}
	if !k.pumpArmed {
		t.Fatal("pump should rearm after envelope delivery")
	}
	// 关闭流：泵返回私有关闭消息，Update 后停止重挂。
	close(ch)
	closed := cmd()
	if _, ok := closed.(eventStreamClosedMsg); !ok {
		t.Fatalf("closed pump = %T, want eventStreamClosedMsg", closed)
	}
	k.Update(closed)
	if k.eventCh != nil {
		t.Fatal("pump should stop after stream closed")
	}
	if got := k.flushCmds(); got != nil {
		t.Fatal("no pump, no cmds → flushCmds should be nil")
	}
}

func TestGoCmdCollectedViaUpdate(t *testing.T) {
	k := NewKernel(Config{})
	k.setMode(state.NormalMode) // 绑定注册在 Normal 模式
	called := false
	mustOK(t, k.Register(keyPlugin{id: "g", bindings: []Binding{
		{Mode: state.NormalMode, Key: "g", OnKey: func(h Host) {
			h.GoCmd(func() tea.Msg { called = true; return nil })
		}},
	}}), "g")
	_, cmd := k.Update(keys("g"))
	if cmd == nil {
		t.Fatal("GoCmd should be returned via Update")
	}
	cmd() // Batch 内含我们的命令
	if !called {
		t.Fatal("GoCmd command should be executable")
	}
}

func TestViewComposesWhenNoOverlay(t *testing.T) {
	k := NewKernel(Config{})
	mustOK(t, k.Register(regionPlugin{id: "bar", region: RegionStatusBar}), "bar")
	// regionPlugin 渲染空串 → compose 折叠为空。
	if got := k.View(); got != "" {
		t.Fatalf("view = %q, want empty", got)
	}
}

func TestWindowSizeInternalOnly(t *testing.T) {
	k, r := newTestKernel(t)
	k.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if k.width != 120 || k.st.Layout.Width != 120 || k.st.Layout.Height != 40 {
		t.Fatalf("size not applied: w=%d layout=%+v", k.width, k.st.Layout)
	}
	if len(r.record) != 0 {
		t.Fatal("window size must not broadcast")
	}
}

func TestHostMethodCoverage(t *testing.T) {
	k, _ := newTestKernel(t)
	h := k.host
	h.State() // 只求执行路径覆盖
	h.Gateway()
	h.GoCmd(nil) // nil 命令应被安全忽略
	h.Mode()
	h.SetMode(state.InputModeInput) // 同模式切换：不武装定时器
	h.PopOverlay()                  // 空栈 pop 安全
	h.Notify("")
	if len(k.pendingCmds) == 0 {
		t.Fatal("notify should arm expiry timer")
	}
	// BindEventStream 转发覆盖（P1-③）：Host 方法全路径可达。
	ch := make(chan gateway.GatewayEvent, 1)
	h.BindEventStream(ch)
	if k.eventCh == nil {
		t.Fatal("BindEventStream should update kernel stream")
	}
}
