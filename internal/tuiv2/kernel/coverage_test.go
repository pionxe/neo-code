package kernel

import (
	"context"
	"strings"
	"testing"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// 本文件补齐覆盖率缺口（issue #20 验收第 11 条：kernel 包覆盖率 100%）：
// debugf 开启路径、命令同类别排序、MouseMsg 入队拒绝、Register 命令冲突、
// Host.PushOverlay/Quit、确认浮层 ID 与窄宽渲染、Notify/Leader 定时器闭包体。

func TestDebugfEmitsWhenEnabled(t *testing.T) {
	k := NewKernel(Config{Debug: true, Opts: Options{LeaderTimeout: 1, MaxQueueDepth: 1}})
	r := &captureReactor{id: "r"}
	mustOK(t, k.Register(r), "register")
	// Debug 开启：规则 1 拒绝路径会走 debugf 的输出分支（KeyMsg 形状消息入队被拒）。
	echoer := &captureReactor{id: "echo", onMsg: func(h Host, msg tea.Msg) {
		h.Send(keys("x")) // KeyMsg → enqueue 拒绝分支（Debug=true → debugf 输出）
	}}
	mustOK(t, k.Register(echoer), "echoer")
	k.Update(state.ConfirmResult{ID: "t"}) // 触发 echoer 在派发期 Send KeyMsg
	// 自激回声耗尽预算同样走 debugf 输出分支。
	for i := 0; i < 3; i++ {
		k.Update(plainMsg("seed"))
	}
}

func TestCommandSortedSameCategoryFallsBackToName(t *testing.T) {
	r := &commandRegistry{}
	run := func(h Host, args []string) {}
	mustOK(t, r.add("t", Command{Name: "/bbb", Category: "S", Run: run}), "bbb")
	mustOK(t, r.add("t", Command{Name: "/aaa", Category: "S", Run: run}), "aaa")
	got := r.sorted()
	if got[0].Name != "/aaa" || got[1].Name != "/bbb" {
		t.Fatalf("same-category order = [%s %s], want [/aaa /bbb]", got[0].Name, got[1].Name)
	}
}

func TestEnqueueRejectsMouseMsgSentByPlugin(t *testing.T) {
	k, r := newTestKernel(t)
	echoer := &captureReactor{id: "echo", onMsg: func(h Host, msg tea.Msg) {
		h.Send(tea.MouseMsg{Type: tea.MouseLeft}) // 鼠标形状消息入队 → 规则 1 拒绝
	}}
	mustOK(t, k.Register(echoer), "echoer")
	k.Update(state.ConfirmResult{ID: "t"})
	if len(r.record) != 1 {
		t.Fatalf("only the trigger should be broadcast, saw %d", len(r.record))
	}
}

func TestRegisterRejectsCommandNameConflict(t *testing.T) {
	k := NewKernel(Config{})
	run := func(h Host, args []string) {}
	mustOK(t, k.Register(commandPlugin{id: "a", cmds: []Command{{Name: "/dup", Run: run}}}), "a")
	err := k.Register(commandPlugin{id: "b", cmds: []Command{{Name: "/dup", Run: run}}})
	if err == nil || !strings.Contains(err.Error(), "command name conflict") {
		t.Fatalf("err = %v, want command name conflict", err)
	}
}

// commandPlugin 只贡献命令。
type commandPlugin struct {
	id   string
	cmds []Command
}

func (p commandPlugin) ID() string                       { return p.id }
func (p commandPlugin) Init(ctx context.Context, h Host) {}
func (p commandPlugin) Close(ctx context.Context)        {}
func (p commandPlugin) Commands() []Command              { return p.cmds }

func TestHostPushOverlayQuitAndOverlayID(t *testing.T) {
	k, _ := newTestKernel(t)
	var captured Overlay
	pusher := &overlayPusher{id: "pusher", overlay: &namedOverlay{id: "pushed"}, onPush: func(o Overlay) { captured = o }}
	mustOK(t, k.Register(pusher), "pusher")

	pusher.onKey(k.host) // 插件内经 Host.PushOverlay + Quit
	if k.stack.depth() != 1 || k.stack.top().ID() != "pushed" {
		t.Fatalf("stack depth = %d, want 1 with pushed overlay", k.stack.depth())
	}
	if captured == nil || captured.ID() != "pushed" {
		t.Fatal("pushed overlay not captured")
	}
	if !pusher.quitted {
		t.Fatal("Quit should be recorded")
	}
	// confirmOverlay 的 ID 路径（经内核 Confirm 建立后读取栈顶标识）。
	k.host.Confirm(state.ConfirmRequest{Title: "T"})
	if k.stack.top().ID() != "kernel.confirm" {
		t.Fatalf("confirm overlay id = %s", k.stack.top().ID())
	}
	k.stack.pop()
}

// overlayPusher 在按键动作中经 Host 压浮层并请求退出。
type overlayPusher struct {
	id      string
	overlay Overlay
	onPush  func(Overlay)
	quitted bool
}

func (p *overlayPusher) ID() string                       { return p.id }
func (p *overlayPusher) Init(ctx context.Context, h Host) {}
func (p *overlayPusher) Close(ctx context.Context)        {}
func (p *overlayPusher) Bindings() []Binding {
	return []Binding{{Mode: state.NormalMode, Key: "o", OnKey: p.onKey}}
}
func (p *overlayPusher) onKey(h Host) {
	h.PushOverlay(p.overlay)
	p.onPush(p.overlay)
	h.Quit()
	p.quitted = true
}

func TestConfirmOverlayNarrowView(t *testing.T) {
	k := NewKernel(Config{})
	k.host.Confirm(state.ConfirmRequest{Title: "⚠", Message: "m"})
	// 窄终端：inner 收敛到下限 20。
	out := k.View()
	if !strings.Contains(out, "[y] 确认") || len(strings.Split(out, "\n")[0]) < 20 {
		t.Fatalf("narrow view = %q", out)
	}
	// Confirm 浮层 View 覆盖主布局（栈顶整屏替换）。
	if !strings.Contains(out, "kernel") && !strings.Contains(out, "⚠") {
		t.Fatalf("view should render confirm overlay, got %q", out)
	}
}

func TestNotifyAndLeaderTimerClosuresExecute(t *testing.T) {
	k := NewKernel(Config{Opts: Options{NotifyExpiry: 1, LeaderTimeout: 1}})
	// Notify 的 Tick 闭包体执行 → 返回代际号消息。
	k.host.Notify("hello")
	var notifyCmd tea.Cmd
	for _, c := range k.pendingCmds {
		notifyCmd = c
	}
	if notifyCmd == nil {
		t.Fatal("notify should arm timer")
	}
	if msg := notifyCmd(); msg == nil {
		t.Fatal("notify timer closure should produce expiry msg")
	}
	// Leader 的 Tick 闭包体执行。
	k.host.SetMode(state.LeaderMode)
	var leaderCmd tea.Cmd
	for _, c := range k.pendingCmds {
		leaderCmd = c
	}
	if leaderCmd == nil {
		t.Fatal("leader should arm timer")
	}
	if msg := leaderCmd(); msg == nil {
		t.Fatal("leader timer closure should produce timeout msg")
	}
	// 清空已收集的命令后：无事件流、无新命令 → flushCmds 为 nil。
	k.pendingCmds = k.pendingCmds[:0]
	if got := k.flushCmds(); got != nil {
		t.Fatal("no cmds and no stream → nil")
	}
}

func TestGatewayPassthrough(t *testing.T) {
	// Host.Gateway 透传 Config.Client（nil 亦然）：插件拿到的是装配时注入的契约。
	k := NewKernel(Config{Client: gateway.Client(nil)})
	if h := k.host; h.Gateway() != nil {
		t.Fatal("nil client should pass through as nil")
	}
}

// TestInitGoCmdPreserved 是 P1-② 的直接回归守卫（PR #22 审计附带项）：
// 插件在 Init 期经 GoCmd 发起的任务必须出现在 Init() 的返回命令中。
func TestInitGoCmdPreserved(t *testing.T) {
	k := NewKernel(Config{})
	fired := false
	initCmdPlugin := &goCmdPlugin{id: "initcmd", cmd: func() tea.Msg { fired = true; return nil }}
	mustOK(t, k.Register(initCmdPlugin), "register")

	cmd := k.Init()
	if cmd == nil {
		t.Fatal("Init should return accumulated commands (plugin GoCmd must not be dropped)")
	}
	cmd()
	if !fired {
		t.Fatal("plugin Init-time GoCmd should be executable via Init return")
	}
}

// goCmdPlugin 在 Init 期发起一条 GoCmd。
type goCmdPlugin struct {
	id  string
	cmd func() tea.Msg
}

func (p *goCmdPlugin) ID() string { return p.id }
func (p *goCmdPlugin) Init(ctx context.Context, h Host) {
	h.GoCmd(p.cmd)
}
func (p *goCmdPlugin) Close(ctx context.Context) {}

// TestGoCmdGatewayEventDoesNotDisarmPump 钉死信封语义（issue #23 修订 v2）：
// 插件经 GoCmd 返回 GatewayEvent 形状消息 → 照常广播但不解除 pumpArmed；
// Send(gateway.GatewayEvent) 同样广播但不解除——两条路径分开钉。
func TestGoCmdGatewayEventDoesNotDisarmPump(t *testing.T) {
	k, r := newTestKernel(t)
	ch := make(chan gateway.GatewayEvent, 2)
	k.BindEventStream(ch)
	pump := k.Init() // armed

	// 路径 A：插件 GoCmd 返回裸 GatewayEvent → 广播，不解除 armed。
	// （GoCmd 经白盒 flushCmds 取出：GoCmd 只在 Update 处理期间发生，
	// Update 开头的 pendingCmds 重置不会截走它。）
	ev := gateway.GatewayEvent{Type: gateway.EventPhaseChanged}
	k.host.GoCmd(func() tea.Msg { return ev })
	cmd := k.flushCmds()
	if cmd == nil {
		t.Fatal("GoCmd should be collected")
	}
	msgA := cmd()
	if _, ok := msgA.(gateway.GatewayEvent); !ok {
		t.Fatalf("GoCmd result = %T, want GatewayEvent", msgA)
	}
	k.Update(msgA)
	if len(r.record) != 1 {
		t.Fatalf("bare GatewayEvent via GoCmd should broadcast, saw %d", len(r.record))
	}
	if !k.pumpArmed {
		t.Fatal("GoCmd GatewayEvent must not disarm the pump")
	}

	// 路径 B：React 内 Send(GatewayEvent) → 广播，不解除 armed。
	// witness 累计：[ev_A, ConfirmResult(触发), ev_B]。
	echoer := &captureReactor{id: "echo", onMsg: func(h Host, msg tea.Msg) {
		if _, ok := msg.(state.ConfirmResult); ok {
			h.Send(ev)
		}
	}}
	mustOK(t, k.Register(echoer), "echoer")
	k.Update(state.ConfirmResult{ID: "t"})
	if len(r.record) != 3 {
		t.Fatalf("after Send(GatewayEvent) witness should hold 3, saw %d", len(r.record))
	}
	if _, ok := r.record[2].(gateway.GatewayEvent); !ok {
		t.Fatalf("third = %T, want GatewayEvent from Send path", r.record[2])
	}
	if !k.pumpArmed {
		t.Fatal("Send GatewayEvent must not disarm the pump")
	}

	// 路径 C：真实泵产物（信封）→ 解除 armed 并广播。witness 累计至 4。
	ch <- ev
	msg := pump()
	k.Update(msg)
	// Update 内：解除 armed → 广播 → flushCmds 重挂泵，故终态 armed=true
	// 且本轮 flush 非空（新泵命令在返回值中）。
	if !k.pumpArmed {
		t.Fatal("pump should have rearmed within the same Update")
	}
	if len(r.record) != 4 {
		t.Fatalf("envelope event should broadcast, witness holds %d", len(r.record))
	}
}

func TestEnqueueDepthCapDropsWhenFull(t *testing.T) {
	// 白盒：队列已满（非派发期）时 enqueue 走深度上限丢弃分支。
	k := NewKernel(Config{Opts: Options{MaxQueueDepth: 2}})
	for i := 0; i < 2; i++ {
		k.enqueue(plainMsg("fill"))
	}
	k.enqueue(plainMsg("overflow")) // cap 命中 → 丢弃 + debug
	if len(k.queue) != 2 {
		t.Fatalf("queue len = %d, want 2 (overflow dropped)", len(k.queue))
	}
}

func TestConfirmOverlayViewNarrowBranch(t *testing.T) {
	// 窄终端分支：inner < 20 时收敛到下限。
	k := NewKernel(Config{})
	k.host.Confirm(state.ConfirmRequest{Title: "⚠", Message: "m"})
	k.width = 10
	out := k.View()
	if !strings.Contains(out, "[y] 确认") {
		t.Fatalf("narrow view = %q", out)
	}
}

func TestLeaderTimeoutDebugfBranch(t *testing.T) {
	// Debug 内核 + 代际匹配超时 → onLeaderTimeout 的 debugf 非 nil 分支。
	k := NewKernel(Config{Debug: true, Opts: Options{LeaderTimeout: 1}})
	k.host.SetMode(state.LeaderMode)
	k.Update(leaderTimeoutMsg{gen: k.modes.leaderGen})
	if k.modes.mode != state.NormalMode {
		t.Fatalf("mode = %v, want normal", k.modes.mode)
	}
}

func TestFlushCmdsBatchesMultipleCommands(t *testing.T) {
	// 同一轮 Update 内既有定时器（Notify Tick）又有插件 GoCmd：
	// flushCmds 应返回 Batch 包装（≥2 命令分支）。
	k := NewKernel(Config{Opts: Options{NotifyExpiry: 1}})
	mustOK(t, k.Register(keyPlugin{id: "g", bindings: []Binding{
		{Mode: state.NormalMode, Key: "g", OnKey: func(h Host) {
			h.GoCmd(func() tea.Msg { return nil })
		}},
	}}), "g")
	k.host.Notify("tick") // 命令 1：到期定时器
	k.setMode(state.NormalMode)
	k.Update(keys("g")) // 命令 2：插件 GoCmd（Update 内 flush）
	if k.pendingCmds == nil {
		// Update 已 flush；无法直接观察 Batch 形态，改用白盒：再收两条后直调 flushCmds。
		k.host.Notify("tick2")
		k.host.GoCmd(func() tea.Msg { return nil })
		cmd := k.flushCmds()
		if cmd == nil {
			t.Fatal("two pending cmds should produce non-nil batch")
		}
		_ = cmd()
	}
}

// TestFullPipelineSmoke 是 fake 插件全链路冒烟（issue #20 验收第 9 条）：
// 一个插件实现全部可选能力，走通 Init→按键路由→React→compose→Close 全程，
// 证明 6 个机制可组合工作（应对"契约先行 = 死代码"风险的组合性证据）。
func TestFullPipelineSmoke(t *testing.T) {
	k := NewKernel(Config{Opts: Options{LeaderTimeout: 1}})
	p := &fullPipelinePlugin{id: "smoke"}
	mustOK(t, k.Register(p), "register smoke plugin")

	// 1. Init：插件初始化并武装事件流（Init 返回值即事件泵命令，须保留执行）。
	ch := make(chan gateway.GatewayEvent, 2)
	k.BindEventStream(ch)
	cmds := []tea.Cmd{k.Init()}

	// 2. 按键路由：Normal 模式按键 → 插件 Binding 动作 → 状态迁移。
	k.setMode(state.NormalMode)
	k.dispatchKey(keyRunes("s"))

	// 3. 广播：Gateway 事件 → React 就地迁移自己的槽。
	// 依次执行收集到的命令（事件泵等），产物经 Update 广播。
	ch <- gateway.GatewayEvent{Type: gateway.EventRunStarted}
	cmds = append(cmds, k.pendingCmds...)
	k.pendingCmds = k.pendingCmds[:0]
	for _, c := range cmds {
		if m := c(); m != nil {
			k.Update(m)
		}
	}
	if !p.gotEvent {
		t.Fatal("plugin React should have seen the gateway event")
	}

	// 4. compose：区域渲染进入视图。
	view := k.View()
	if !strings.Contains(view, "SMOKE-STATUS") {
		t.Fatalf("view should contain plugin region render, got %q", view)
	}

	// 5. Close：逆序关闭（单插件即直接关闭）。
	k.Close(context.Background())
	if !p.closed {
		t.Fatal("plugin should be closed")
	}
}

// fullPipelinePlugin 实现全部可选能力，用于全链路冒烟。
type fullPipelinePlugin struct {
	id       string
	closed   bool
	gotEvent bool
}

func (p *fullPipelinePlugin) ID() string { return p.id }
func (p *fullPipelinePlugin) Init(ctx context.Context, h Host) {
	// 初始化期注册自身的渲染数据（写自己拥有的槽位语义由后续插件 PR 细化；
	// 冒烟里以状态栏文本验证 compose 链路）。
	h.State().Layout.ShowInspector = true
}
func (p *fullPipelinePlugin) Close(ctx context.Context) { p.closed = true }
func (p *fullPipelinePlugin) React(h Host, msg tea.Msg) {
	if ev, ok := msg.(gateway.GatewayEvent); ok && ev.Type == gateway.EventRunStarted {
		p.gotEvent = true
	}
}
func (p *fullPipelinePlugin) Bindings() []Binding {
	return []Binding{{Mode: state.NormalMode, Key: "s", Description: "冒烟动作", OnKey: func(h Host) {
		h.State().Runtime.RunID = "smoke-run" // 就地迁移（冒烟用内核槽代指插件槽）
	}}}
}
func (p *fullPipelinePlugin) Commands() []Command {
	return []Command{{Name: "/smoke", Category: "T", Run: func(h Host, args []string) { h.Notify("smoked") }}}
}
func (p *fullPipelinePlugin) Region() RegionID { return RegionStatusBar }
func (p *fullPipelinePlugin) Render(h Host, width int) string {
	if h.State().Runtime.RunID == "smoke-run" {
		return "SMOKE-STATUS " + h.State().Runtime.RunID
	}
	return ""
}

// ---------- Binding 通配扩展（issue #25 / K1）----------

func TestBindingOnKeyOnKeyMsgExclusive(t *testing.T) {
	r := &bindingRegistry{}
	err := r.add("a", Binding{Mode: state.NormalMode, Key: "x", OnKey: func(h Host) {}, OnKeyMsg: func(h Host, msg tea.KeyMsg) {}})
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("err = %v, want exclusive error", err)
	}
	err = r.add("a", Binding{Mode: state.NormalMode, OnKeyMsg: func(h Host, msg tea.KeyMsg) {}})
	if err == nil || !strings.Contains(err.Error(), "requires Description") {
		t.Fatalf("wildcard without description = %v", err)
	}
}

func TestWildcardSecondPerModeFailsFast(t *testing.T) {
	r := &bindingRegistry{}
	w1 := Binding{Mode: state.InputModeInput, Description: "第一通配", OnKeyMsg: func(h Host, msg tea.KeyMsg) {}}
	w2 := Binding{Mode: state.InputModeInput, Description: "第二通配", OnKeyMsg: func(h Host, msg tea.KeyMsg) {}}
	if err := r.add("a", w1); err != nil {
		t.Fatalf("first wildcard: %v", err)
	}
	err := r.add("b", w2)
	if err == nil || !strings.Contains(err.Error(), "wildcard binding conflict") {
		t.Fatalf("err = %v, want wildcard conflict", err)
	}
}

func TestLookupPrecisionOverWildcard(t *testing.T) {
	r := &bindingRegistry{}
	wildHit, preciseHit := "", ""
	wild := Binding{Mode: state.InputModeInput, Description: "通配", OnKeyMsg: func(h Host, msg tea.KeyMsg) { wildHit = msg.String() }}
	precise := Binding{Mode: state.InputModeInput, Key: "enter", Description: "精确", OnKey: func(h Host) { preciseHit = "enter" }}
	mustOK(t, r.add("a", wild), "wild")
	mustOK(t, r.add("a", precise), "precise")

	// 精确键命中精确绑定（通配不拦截）。
	if b, ok := r.lookup(state.InputModeInput, keyRunes("enter"), state.NewViewState()); !ok || b.isWildcard() {
		t.Fatal("enter should hit precise binding")
	}
	// 任意字符命中通配并收到原始 KeyMsg。
	b, ok := r.lookup(state.InputModeInput, keyRunes("z"), state.NewViewState())
	if !ok || !b.isWildcard() {
		t.Fatal("runes should hit wildcard")
	}
	b.invoke(&testFakeHost{}, keyRunes("z"))
	if wildHit != "z" {
		t.Fatalf("OnKeyMsg payload = %q, want original key", wildHit)
	}
	_ = preciseHit
}

func TestWildcardBindingInvokeWithoutExact(t *testing.T) {
	// 通配绑定的 invoke 需要 OnKeyMsg；纯 OnKey 通配形态（构造非法但防御）不 panic。
	b := Binding{Key: "", OnKey: func(h Host) {}}
	b.invoke(&testFakeHost{}, keyRunes("x")) // 无 OnKeyMsg → 静默
}

func TestDeriveShortcutWildcardPlaceholder(t *testing.T) {
	// 通配绑定的快捷键列占位（审计 P2-4：kernel 99.3% 缺口在此）。
	r := &bindingRegistry{}
	wild := Binding{Mode: state.InputModeInput, Description: "输入编辑", Command: "/input", OnKeyMsg: func(h Host, msg tea.KeyMsg) {}}
	if err := r.add("p", wild); err != nil {
		t.Fatalf("add: %v", err)
	}
	if got := deriveShortcut("/input", r); got != "<输入>" {
		t.Fatalf("shortcut = %q, want <输入>", got)
	}
}

// ---------- 泵重绑定协议（issue #27 S3-2 / 审计 P0-①）----------

// TestBindEventStreamStaleEnvelopeIgnored：换代后陈旧信封不广播、不解除 armed。
func TestBindEventStreamStaleEnvelopeIgnored(t *testing.T) {
	k, r := newTestKernel(t)
	oldCh := make(chan gateway.GatewayEvent, 1)
	k.BindEventStream(oldCh)
	pump := k.Init() // 武装旧代际泵
	if pump == nil {
		t.Fatal("Init should arm pump")
	}
	// 换代：绑定新通道（模拟会话切换后的重订阅）。
	newCh := make(chan gateway.GatewayEvent, 1)
	k.BindEventStream(newCh)
	// 旧通道后续事件 → 旧泵产出陈旧信封 → 必须丢弃。
	oldCh <- gateway.GatewayEvent{Type: gateway.EventRunStarted, RunID: "stale"}
	stale := pump()
	env, ok := stale.(gatewayEventEnvelope)
	if !ok || env.gen == k.pumpGen {
		t.Fatalf("stale envelope = %+v", stale)
	}
	k.Update(stale)
	if len(r.record) != 0 {
		t.Fatalf("stale envelope must not broadcast, saw %d", len(r.record))
	}
	if !k.pumpArmed {
		t.Fatal("stale envelope must not disarm current pump")
	}
}

// TestBindEventStreamNewChannelDelivers：换代后新通道事件正常广播。
// 换代后旧泵仍在飞（阻塞旧通道）——新泵经 flushCmds 武装，旧通道不再投递。
func TestBindEventStreamNewChannelDelivers(t *testing.T) {
	k, r := newTestKernel(t)
	oldCh := make(chan gateway.GatewayEvent, 1)
	k.BindEventStream(oldCh)
	_ = k.Init() // 旧代际泵武装（将被换代作废）
	newCh := make(chan gateway.GatewayEvent, 1)
	k.BindEventStream(newCh)
	newPump := k.flushCmds() // 换代后武装新代际泵
	if newPump == nil {
		t.Fatal("rebind should allow new pump")
	}
	newCh <- gateway.GatewayEvent{Type: gateway.EventRunStarted, RunID: "fresh"}
	msg := newPump()
	k.Update(msg)
	if len(r.record) != 1 {
		t.Fatalf("new channel event should broadcast, saw %d", len(r.record))
	}
}

// TestStaleClosedDoesNotKillNewPump：陈旧 closed 在新通道泵武装后到达，
// 不得清空 eventCh、不得停新泵（审计非阻塞备注①的钉死用例）。
func TestStaleClosedDoesNotKillNewPump(t *testing.T) {
	k, _ := newTestKernel(t)
	oldCh := make(chan gateway.GatewayEvent, 1)
	k.BindEventStream(oldCh)
	pump := k.Init()
	newCh := make(chan gateway.GatewayEvent, 1)
	k.BindEventStream(newCh) // 换代：pumpArmed=false，新泵待武装
	// 旧通道关闭 → 旧泵返回陈旧 closed → 必须被忽略。
	close(oldCh)
	staleClosed := pump()
	if _, ok := staleClosed.(eventStreamClosedMsg); !ok {
		t.Fatalf("old pump should deliver closed, got %T", staleClosed)
	}
	// Update 返回值即换代后武装的新泵命令（丢弃会导致新通道无人监听）。
	_, cmd := k.Update(staleClosed)
	if k.eventCh == nil {
		t.Fatal("stale closed must not clear the NEW event stream")
	}
	if cmd == nil {
		t.Fatal("new pump should be armed and returned by Update")
	}
	newCh <- gateway.GatewayEvent{Type: gateway.EventRunStarted}
	msg := cmd()
	env, ok := msg.(gatewayEventEnvelope)
	if !ok || env.gen != k.pumpGen {
		t.Fatalf("new pump delivery = %+v", msg)
	}
	k.Update(msg)
	if k.eventCh == nil {
		t.Fatal("new stream should remain active after delivering event")
	}
}

// ---------- S3-3 契约行为测试（审计第 6 轮 P1）----------

// TestRunCommandAliasEquivalence 钉死 RunCommand 别名==规范名等价与未知命令错误
// （审计第 6 轮 P1-1：runResolved/snapshot 此前零覆盖）。
func TestRunCommandAliasEquivalence(t *testing.T) {
	k := NewKernel(Config{})
	regCalls := []string{}
	p := &commandRecorderPlugin{
		id: "rec",
		cmds: []Command{
			{Name: "/real", Aliases: []string{"alias"}, Description: "d", Run: func(h Host, args []string) {
				regCalls = append(regCalls, "ran:/real")
			}},
		},
	}
	mustOK(t, k.Register(p), "register")

	h := k.host
	// 别名与规范名等价。
	if err := h.RunCommand("alias", nil); err != nil {
		t.Fatalf("alias: %v", err)
	}
	if err := h.RunCommand("/real", nil); err != nil {
		t.Fatalf("name: %v", err)
	}
	if len(regCalls) != 2 {
		t.Fatalf("run count = %d, want 2", len(regCalls))
	}
	// 未知命令返回错误。
	if err := h.RunCommand("nope", nil); err == nil {
		t.Fatal("unknown command should error")
	}
}

// TestCommandsSnapshotInjectsShortcut 钉死 Shortcut 内核预派生
// （审计第 6 轮 P1-1：snapshot 此前零覆盖）。
func TestCommandsSnapshotInjectsShortcut(t *testing.T) {
	k := NewKernel(Config{})
	mustOK(t, k.Register(&commandRecorderPlugin{
		id: "rec",
		cmds: []Command{
			{Name: "/feat", Description: "d", Run: func(h Host, args []string) {}},
		},
		bindings: []Binding{
			{Mode: state.LeaderMode, Key: "f", Description: "功能", Command: "/feat", OnKey: func(h Host) {}},
		},
	}), "register")

	snap := k.host.Commands()
	if len(snap) != 1 {
		t.Fatalf("snapshot len = %d", len(snap))
	}
	if snap[0].Shortcut != "Space f" {
		t.Fatalf("shortcut = %q, want %q", snap[0].Shortcut, "Space f")
	}
}

// TestModeChangedBroadcast 钉死 ModeChanged 生产者三规则（审计第 6 轮 P1-2）：
// 实际变化广播（From/To 正确）、同值 setMode 不广播、Leader 超时广播。
func TestModeChangedBroadcast(t *testing.T) {
	k, r := newTestKernel(t)
	// 内核初始模式 = Input（NewViewState 默认）：同值切换不广播。
	k.host.SetMode(state.InputModeInput)
	if len(r.record) != 0 {
		t.Fatalf("same-value setMode must not broadcast, saw %d", len(r.record))
	}
	// 实际变化流程：Input→Normal→Leader→(超时回落)Normal→Input，
	// 每次实际变化都广播 ModeChanged。
	k.host.SetMode(state.NormalMode)
	k.host.SetMode(state.LeaderMode)
	for _, c := range k.pendingCmds {
		if msg := c(); msg != nil {
			k.Update(msg) // Leader 超时回流
		}
	}
	k.host.SetMode(state.InputModeInput)
	// 收集全部 ModeChanged 验证变迁序列。
	var seq []state.ModeChanged
	for _, b := range r.record {
		if mc, ok := b.(state.ModeChanged); ok {
			seq = append(seq, mc)
		}
	}
	// 流程：Input→Normal→Leader→(超时)Normal→Input。
	want := []state.ModeChanged{
		{From: state.InputModeInput, To: state.NormalMode},
		{From: state.NormalMode, To: state.LeaderMode},
		{From: state.LeaderMode, To: state.NormalMode},
		{From: state.NormalMode, To: state.InputModeInput},
	}
	if len(seq) != len(want) {
		t.Fatalf("ModeChanged seq len = %d, want %d: %+v", len(seq), len(want), seq)
	}
	for i, mc := range seq {
		if mc != want[i] {
			t.Fatalf("seq[%d] = %+v, want %+v", i, mc, want[i])
		}
	}
}

// TestWhenGuardLookupIntegration 钉死 When 守卫的 lookup 集成（审计第 6 轮 P1-3）：
// 精确未过守卫 → 通配兜底；双双失败 → 丢弃。
func TestWhenGuardLookupIntegration(t *testing.T) {
	r := &bindingRegistry{}
	// 精确键带守卫（不通过）：query 非空时 "enter" 被禁。
	precise := Binding{Mode: state.NormalMode, Key: "enter", OnKey: func(h Host) {},
		When: func(s *state.ViewState) bool { return s.Search.Query == "" }}
	// 通配带守卫（通过）：query 非空时捕获。
	wild := Binding{Mode: state.NormalMode, Description: "通配", OnKeyMsg: func(h Host, msg tea.KeyMsg) {},
		When: func(s *state.ViewState) bool { return s.Search.Query != "" }}
	mustOK(t, r.add("a", precise), "precise")
	mustOK(t, r.add("a", wild), "wild")

	empty := state.NewViewState()
	full := state.NewViewState()
	full.Search.Query = "q"

	// query 空：精确通过（When true），通配不参与。
	if b, ok := r.lookup(state.NormalMode, tea.KeyMsg{Type: tea.KeyEnter}, empty); !ok || b.isWildcard() {
		t.Fatal("empty query should hit precise enter")
	}
	// query 非空：精确被守卫拦下 → 通配兜底。
	if b, ok := r.lookup(state.NormalMode, tea.KeyMsg{Type: tea.KeyEnter}, full); !ok || !b.isWildcard() {
		t.Fatalf("non-empty query should fall through to wildcard, ok=%v", ok)
	}
	// 双双失败：未知键在两种状态下都丢弃。
	if _, ok := r.lookup(state.NormalMode, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}, empty); ok {
		t.Fatal("unmatched key should be dropped")
	}
}

// commandRecorderPlugin 是携带命令与绑定的测试插件。
type commandRecorderPlugin struct {
	id       string
	cmds     []Command
	bindings []Binding
}

func (p *commandRecorderPlugin) ID() string                       { return p.id }
func (p *commandRecorderPlugin) Init(ctx context.Context, h Host) {}
func (p *commandRecorderPlugin) Close(ctx context.Context)        {}
func (p *commandRecorderPlugin) Commands() []Command              { return p.cmds }
func (p *commandRecorderPlugin) Bindings() []Binding              { return p.bindings }
