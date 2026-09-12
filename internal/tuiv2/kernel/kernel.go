package kernel

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// eventStreamClosedMsg 表示事件流关闭的内核私有消息：此后不再重挂事件泵。
type eventStreamClosedMsg struct{}

// Kernel 是 TUI v2 的唯一 tea.Model：装配插件并驱动全部机制。
// 状态纪律（ADR-001/009）：持有全局唯一的 *state.ViewState，指针全程稳定；
// 派发纪律（§4.1）：KeyMsg/鼠标/内核私有消息永不入广播；广播经队列排空，
// 派发期间的 Send/SetMode 产生的新消息一律入队尾延后处理（禁嵌套）。
type Kernel struct {
	ctx    context.Context
	client gateway.Client
	st     *state.ViewState
	opts   Options

	host     kernelHost // Host 接口的内核实现，注册时注入各插件
	plugins  []Plugin
	reactors []Reactor

	bindings  bindingRegistry
	commands  commandRegistry
	renderers map[RegionID]RegionRenderer

	modes       modeMachine
	stack       overlayStack
	queue       []tea.Msg // 广播队列：派发中入队尾，循环排空
	dispatching bool      // 正在排空队列（禁嵌套标记）
	pumpArmed   bool      // 事件泵单实例守卫：仅一个泵命令在飞（P1 修复）
	pendingCmds []tea.Cmd // 本轮 Update 累积的命令（定时器、插件 GoCmd、退出）
	eventCh     <-chan gateway.GatewayEvent
	width       int
	confirmSeq  int
	notifyGen   int
}

// kernelHost 将 Kernel 暴露为插件可见的 Host 接口（编译期断言见测试文件）。
type kernelHost struct{ k *Kernel }

// Config 是内核装配参数。
type Config struct {
	Client gateway.Client   // Gateway 契约（fake/real 对插件透明）
	State  *state.ViewState // 全局唯一状态；nil 时内核自建
	Opts   Options          // 机制参数（零值取默认）
	Debug  bool             // 调试行与内核日志
}

// NewKernel 创建内核。注册插件用 Register；启动用 Init（tea.Model 契约）。
func NewKernel(cfg Config) *Kernel {
	if cfg.State == nil {
		cfg.State = state.NewViewState()
	}
	k := &Kernel{
		ctx:       context.Background(),
		client:    cfg.Client,
		st:        cfg.State,
		opts:      cfg.Opts.setDefaults(),
		renderers: make(map[RegionID]RegionRenderer),
		width:     80,
	}
	k.opts.Debug = k.opts.Debug || cfg.Debug
	k.modes = modeMachine{mode: cfg.State.Mode, timeout: k.opts.LeaderTimeout, debugf: k.debugf}
	k.host = kernelHost{k: k}
	return k
}

// debugf 仅在 Debug 开启时输出内核日志（丢弃路径、超时等机制事件）。
func (k *Kernel) debugf(format string, args ...any) {
	if k.opts.Debug {
		log.Printf("[tuiv2-kernel] "+format, args...)
	}
}

// Register 注册一个插件并发现其可选能力（类型断言，ADR-008）。
// 冲突 fail-fast：插件 ID、区域所有权、键位绑定、命令名任一重复即报错，
// 装配阶段（早于进入 TUI）即失败，符合"非法配置尽早失败"。
func (k *Kernel) Register(p Plugin) error {
	id := p.ID()
	if id == "" {
		return fmt.Errorf("kernel: plugin id must not be empty")
	}
	for _, existing := range k.plugins {
		if existing.ID() == id {
			return fmt.Errorf("%w: %s", ErrDuplicatePlugin, id)
		}
	}
	k.plugins = append(k.plugins, p)

	if r, ok := p.(Reactor); ok {
		k.reactors = append(k.reactors, r)
	}
	if b, ok := p.(KeyBinder); ok {
		for _, binding := range b.Bindings() {
			if err := k.bindings.add(id, binding); err != nil {
				return err
			}
		}
	}
	if cp, ok := p.(CommandProvider); ok {
		for _, c := range cp.Commands() {
			if err := k.commands.add(id, c); err != nil {
				return err
			}
		}
	}
	if rr, ok := p.(RegionRenderer); ok {
		region := rr.Region()
		if _, dup := k.renderers[region]; dup {
			return fmt.Errorf("%w: %s by %s", ErrDuplicateRegion, region, id)
		}
		k.renderers[region] = rr
	}
	return nil
}

// BindEventStream 绑定 Gateway 事件流：内核此后自动重挂事件泵（Init/Update 均会）。
func (k *Kernel) BindEventStream(ch <-chan gateway.GatewayEvent) {
	k.eventCh = ch
}

// Init 实现 tea.Model：顺序调用各插件 Init，并返回全部累积命令
// （含插件 Init 期经 GoCmd 发起的任务与事件泵——P1 修复：此前仅返回泵，
// 插件 Init 期的异步命令会被丢弃）。
func (k *Kernel) Init() tea.Cmd {
	for _, p := range k.plugins {
		p.Init(k.ctx, k.host)
	}
	return k.flushCmds()
}

// Close 逆序关闭各插件（后注册先关闭）。
func (k *Kernel) Close(ctx context.Context) {
	for i := len(k.plugins) - 1; i >= 0; i-- {
		k.plugins[i].Close(ctx)
	}
}

// Update 实现 tea.Model：分发一条消息（键路由或广播）并回流全部累积命令。
func (k *Kernel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k.pendingCmds = k.pendingCmds[:0]
	switch m := msg.(type) {
	case tea.KeyMsg:
		k.dispatchKey(m.String())
	case tea.MouseMsg:
		// 鼠标消息同禁入广播（§4.1 同级契约），路由语义由 S7 鼠标插件钉死。
	case tea.WindowSizeMsg:
		// 尺寸是内核拥有槽（Layout），内部消化不广播；渲染器经 Render 参数获得宽度。
		k.width, k.st.Layout.Width = m.Width, m.Width
		k.st.Layout.Height = m.Height
	case leaderTimeoutMsg:
		// 内核私有消息：前置过滤，永不入广播。
		// 超时回落后同步 Mode 槽（模式机是唯一事实源，P1 修复：此前槽与机分叉）。
		k.modes.onLeaderTimeout(m)
		k.st.Mode = k.modes.mode
	case notifyExpiryMsg:
		// 内核私有消息：按代际对号清除，旧定时器不覆盖新提示。
		if m.gen == k.notifyGen {
			k.st.Notify = state.NotifyState{}
		}
	case eventStreamClosedMsg:
		k.eventCh = nil
		k.pumpArmed = false // 泵已停，允许 BindEventStream 后重新武装
		k.debugf("event stream closed, pump stopped")
	case gatewayEventEnvelope:
		// 泵产物信封（内核私有）：armed 解除只认信封类型——插件经 GoCmd
		// 返回 GatewayEvent 形状的消息不会误解除单泵不变量（审计附带项）。
		k.pumpArmed = false // 事件即泵的产物：交付即消费，允许 flushCmds 重挂下一泵
		k.broadcast(m.event)
	case gateway.GatewayEvent:
		// 非泵来源的 GatewayEvent（插件 GoCmd/State 显式回流）：照常广播，
		// 但不解除 pumpArmed（单泵不变量）。
		k.broadcast(msg)
	default:
		// 插件间消息：广播给全部 Reactor。口径：凡非键/鼠标/窗口/内核私有
		// 的消息（含 Paste/Focus 等 bubbletea 内建消息）均视为插件间消息广播——
		// 键类消息永远只走路由，除此之外不做白名单（契约见 §4.1）。
		k.broadcast(msg)
	}
	return k, k.flushCmds()
}

// View 实现 tea.Model：浮层栈非空时整屏替换为栈顶浮层，否则拼装主布局。
func (k *Kernel) View() string {
	if top := k.stack.top(); top != nil {
		return top.View(k.host, k.width)
	}
	return Compose(k.host, k.renderers, k.width)
}

// dispatchKey 实现三级按键路由（§4.1，详见 modeMachine 注释）。
func (k *Kernel) dispatchKey(key string) {
	// 1. 内核保留键：ctrl+c 请求退出。
	if key == "ctrl+c" {
		k.debugf("reserved ctrl+c, quit requested")
		k.pendingCmds = append(k.pendingCmds, tea.Quit)
		return
	}
	// 2. 浮层栈顶独占：esc 未被栈顶消费时弹栈；其余键未消费即丢弃。
	if k.stack.depth() > 0 {
		top := k.stack.top()
		consumed := top.HandleKey(k.host, key)
		if key == "esc" && !consumed {
			k.stack.pop()
		}
		return
	}
	// 3. 当前模式绑定：未命中丢弃；Leader 未命中额外静默回落 Normal。
	if b, ok := k.bindings.lookup(k.modes.mode, key); ok {
		b.OnKey(k.host)
		return
	}
	if k.modes.mode == state.LeaderMode {
		k.setMode(state.NormalMode)
	}
}

// broadcast 将消息入队并排空队列：派发期间的新消息由排空循环继续处理（禁嵌套）。
func (k *Kernel) broadcast(msg tea.Msg) {
	k.enqueue(msg)
	k.drain()
}

// enqueue 入队；键与鼠标形状的消息在入口即拒绝（§4.1 规则 1：
// KeyMsg 永不广播，即使从插件侧发出）；超过深度上限（自激回声）同样
// 丢弃并记 debug 日志。
func (k *Kernel) enqueue(msg tea.Msg) {
	switch msg.(type) {
	case tea.KeyMsg, tea.MouseMsg:
		k.debugf("rejected %T entering broadcast queue (rule 1)", msg)
		return
	}
	if len(k.queue) >= k.opts.MaxQueueDepth {
		k.debugf("queue depth cap %d reached, dropping message %T", k.opts.MaxQueueDepth, msg)
		return
	}
	k.queue = append(k.queue, msg)
}

// drain 按注册顺序把队列中的每条消息派发给全部 Reactor；
// 派发期间入队的新消息由本循环继续处理（禁嵌套、保序）。
// **派发预算**：每轮 drain 处理的消息总量受 MaxQueueDepth 硬上限约束，
// 超限即清空余量并记 debug 日志。这是自激回声的最终守卫——仅靠队列
// 深度上限无法阻止"清空再填一"式的自我回声无限循环（每轮排空时
// 队列恒短于上限），必须以派发总量封顶（issue #20 修订 v2 P0）。
func (k *Kernel) drain() {
	if k.dispatching {
		return
	}
	k.dispatching = true
	defer func() { k.dispatching = false }()
	processed := 0
	for len(k.queue) > 0 {
		if processed >= k.opts.MaxQueueDepth {
			dropped := len(k.queue)
			k.queue = k.queue[:0]
			k.debugf("dispatch budget %d exhausted, dropped %d queued messages", k.opts.MaxQueueDepth, dropped)
			return
		}
		msg := k.queue[0]
		k.queue = k.queue[1:]
		processed++
		for _, r := range k.reactors {
			r.React(k.host, msg)
		}
	}
}

// setMode 切换模式：同步内核模式机与 Mode 槽（内核是该槽唯一写者），
// 进入 Leader 时武装带代际号的超时命令。
func (k *Kernel) setMode(next state.InputMode) {
	if cmd := k.modes.setMode(next); cmd != nil {
		k.pendingCmds = append(k.pendingCmds, cmd)
	}
	k.st.Mode = k.modes.mode
}

// flushCmds 返回本轮累积命令（含事件泵重挂），空时返回 nil。
// P1 修复：事件泵单实例——pumpArmed 守卫下仅武装一次；上一泵命令被
// bubbletea 执行并回流消息前，本方法不会重复挂泵，消除"每次 Update
// 泄漏一个阻塞在通道上的 goroutine"（旧实现每次 Update 无条件重挂）。
func (k *Kernel) flushCmds() tea.Cmd {
	cmds := k.pendingCmds
	k.pendingCmds = nil
	if pump := k.waitEvent(); pump != nil {
		cmds = append(cmds, pump)
	}
	if len(cmds) == 0 {
		return nil
	}
	if len(cmds) == 1 {
		return cmds[0] // 单命令直返：避免 Batch 包装改变 cmd() 的返回形态
	}
	return tea.Batch(cmds...)
}

// gatewayEventEnvelope 是事件泵产物的内核私有信封（永不入广播本体，
// Update 内拆包后广播裸事件）：pumpArmed 的解除只认信封类型，
// 与"插件经 GoCmd 返回 GatewayEvent 形状消息"严格区分（审计附带项）。
type gatewayEventEnvelope struct {
	event gateway.GatewayEvent
}

// waitEvent 在事件流已绑定且泵未武装时返回泵命令（占用即置 pumpArmed）；
// 流关闭消息经 Update 重置 pumpArmed 后才允许再次武装。
func (k *Kernel) waitEvent() tea.Cmd {
	if k.eventCh == nil || k.pumpArmed {
		return nil
	}
	k.pumpArmed = true
	ch := k.eventCh
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return eventStreamClosedMsg{}
		}
		return gatewayEventEnvelope{event: event}
	}
}

// ---------- Host 接口的内核实现 ----------

// State 返回全局唯一状态（读任意槽合法，写权归各槽 owner）。
func (h kernelHost) State() *state.ViewState { return h.k.st }

// Gateway 返回 Gateway 客户端契约。
func (h kernelHost) Gateway() gateway.Client { return h.k.client }

// GoCmd 收集异步命令，经 Update 返回值交还 bubbletea 执行。
func (h kernelHost) GoCmd(cmd tea.Cmd) {
	if cmd != nil {
		h.k.pendingCmds = append(h.k.pendingCmds, cmd)
	}
}

// Send 广播消息：派发期间入队尾延后（禁嵌套），否则立即排空。
func (h kernelHost) Send(msg tea.Msg) { h.k.broadcast(msg) }

// Mode 返回当前键位模式。
func (h kernelHost) Mode() state.InputMode { return h.k.modes.mode }

// SetMode 切换键位模式（见 Kernel.setMode）。
func (h kernelHost) SetMode(m state.InputMode) { h.k.setMode(m) }

// PushOverlay 压入浮层。
func (h kernelHost) PushOverlay(o Overlay) { h.k.stack.push(o) }

// PopOverlay 弹出栈顶浮层。
func (h kernelHost) PopOverlay() { h.k.stack.pop() }

// Confirm 发起确认：生成请求 ID，压入内核自带确认浮层；
// 用户应答后经 Send 广播 state.ConfirmResult（请求方在 React 中按 ID 消费）。
// 浮层自关按对象身份精确移除（而非 LIFO 弹栈）：应答广播期间 Reactor 若压入
// 新浮层（P2 场景），弹栈会误弹他层。
func (h kernelHost) Confirm(req state.ConfirmRequest) {
	h.k.confirmSeq++
	req.ID = fmt.Sprintf("confirm-%d", h.k.confirmSeq)
	h.k.stack.push(&confirmOverlay{k: h.k, req: req})
}

// Notify 写入弱提示并武装到期清除定时器（代际对号，旧定时器失效）。
func (h kernelHost) Notify(text string) {
	h.k.notifyGen++
	h.k.st.Notify = state.NotifyState{Text: text, At: time.Now()}
	gen := h.k.notifyGen
	expiry := h.k.opts.NotifyExpiry
	h.k.pendingCmds = append(h.k.pendingCmds, tea.Tick(expiry, func(time.Time) tea.Msg {
		return notifyExpiryMsg{gen: gen}
	}))
}

// Quit 请求退出程序。
func (h kernelHost) Quit() {
	h.k.pendingCmds = append(h.k.pendingCmds, tea.Quit)
}

// confirmOverlay 是内核自带的确认浮层（ADR-012 内核服务）：
// y/enter 确认，n/esc 取消；应答广播后经 PopOverlay 自关。
type confirmOverlay struct {
	k   *Kernel
	req state.ConfirmRequest
}

// ID 返回确认浮层标识。
func (c *confirmOverlay) ID() string { return "kernel.confirm" }

// HandleKey 处理应答键：y/enter 确认，n/esc 取消；应答即按对象身份自删并广播结果。
// 返回 consumed=true 表示已应答（含取消）；其余键一律消费（模态确认防误操作）。
func (c *confirmOverlay) HandleKey(h Host, key string) (consumed bool) {
	var yes bool
	switch key {
	case "y", "enter":
		yes = true
	case "n", "esc":
		yes = false
	default:
		return true
	}
	c.k.stack.remove(c) // 按对象身份精确自删，防应答期 Reactor 压栈导致 LIFO 误弹
	h.Send(state.ConfirmResult{ID: c.req.ID, Action: c.req.Action, Data: c.req.Data, Yes: yes})
	return true
}

// View 渲染确认浮层：唯一允许使用边框的场景之一（危险操作确认，防误操作）。
func (c *confirmOverlay) View(h Host, width int) string {
	inner := width - 4
	if inner < 20 {
		inner = 20
	}
	bar := strings.Repeat("─", inner)
	return fmt.Sprintf("┌─ %s %s\n│ %s\n│\n│ [y] 确认   [n] 取消\n└%s", c.req.Title, bar, c.req.Message, bar)
}
