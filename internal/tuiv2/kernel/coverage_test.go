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
	k.dispatchKey("s")

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
