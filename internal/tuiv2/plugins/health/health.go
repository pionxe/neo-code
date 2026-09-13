// Package health 是 TUI v2 的健康探针插件（issue #48，S6 / ADR-005）：
// 周期 Health 探针 + 指数退避 + 断连/恢复视觉（Notify）+
// Gateway.Connected 槽写权承接（自 sessions 临时承接移交）。
//
// 架构 go/no-go 门：本插件全程不触 internal/tuiv2/kernel/——
// 探针调度=GoCmd 自续订（CursorBlink 先例）、退避=New 注入参数、
// 视觉=Notify/槽写、恢复协作=Send(GatewayRecovered{}) 广播。
package health

import (
	"context"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"
)

// Config 是探针节奏参数（具名常量默认值 + New 可注入，S5 审计 P2-3）。
type Config struct {
	// HealthyInterval 是健康状态下的固定探针间隔（30s > v1 心跳 10s：
	// 探针是传输自愈之上的二阶检查，慢于心跳是设计意图）。
	HealthyInterval time.Duration
	// InitialBackoff 是失败后的起始退避。
	InitialBackoff time.Duration
	// MaxBackoff 是退避上限（恢复通知最差延迟上界）。
	MaxBackoff time.Duration
	// ProbeTimeout 是单次探针的独立超时（≪ HealthyInterval：防悬挂探针
	// 拉伸健康节奏；失败期每级至多叠加 ProbeTimeout，意在放慢可接受）。
	ProbeTimeout time.Duration
}

// DefaultConfig 返回默认节奏：30s 固定 / 1s×2 退避至 60s / 探针超时 5s。
func DefaultConfig() Config {
	return Config{
		HealthyInterval: 30 * time.Second,
		InitialBackoff:  1 * time.Second,
		MaxBackoff:      60 * time.Second,
		ProbeTimeout:    5 * time.Second,
	}
}

// 探针状态三态（unknown=启动初值：首探成败都视为状态迁移而非"恢复"）。
const (
	probeStateUnknown = iota
	probeStateHealthy
	probeStateUnhealthy
)

// nextInterval 计算下一探针间隔：attempt=1 为健康固定档；失败从
// Initial 起逐次倍增封顶 Max（attempt=2 → Initial）。
func nextInterval(cfg Config, attempt int) time.Duration {
	if attempt <= 1 {
		return cfg.HealthyInterval
	}
	interval := cfg.InitialBackoff
	for i := 2; i < attempt; i++ {
		interval *= 2
		if interval >= cfg.MaxBackoff {
			return cfg.MaxBackoff
		}
	}
	return interval
}

// ProbeResultMsg 是单次探针的结果（kernel 广播——当前仅本插件消费；
// 导出因跨插件集成测试需构造，S5 审计先例：TestFlattenCoverageMatrix）。
type ProbeResultMsg struct {
	// Err 是探针错误：nil=健康，非 nil=不健康（健康判据 err!=nil，
	// HealthResult.OK 是死词表——S6 审计 P2-1）。
	Err error
}

// probeTickMsg 是下一次探针的续订信号。
type probeTickMsg struct{}

// Plugin 是健康探针插件。
type Plugin struct {
	st     *state.ViewState // 全局唯一状态（Init 时固定指针，ADR-001）
	client gateway.Client   // Gateway 契约（Host.Gateway 透传）
	cfg    Config

	mu           sync.Mutex // 保护 probeState/attemptCount/stopped（React 与 Close 跨 goroutine）
	probeState   int        // 探针三态（边沿触发判据；初值 unknown）
	attemptCount int        // 失败退避计数（健康归零）
	stopped      bool       // Close 停止位：防在队 tick 回流续订（S5 审计 P1-5）
}

// New 创建健康探针插件。
func New(cfg Config) *Plugin { return &Plugin{cfg: cfg} }

// ID 返回插件标识。
func (p *Plugin) ID() string { return "health" }

// Init 固定状态指针与客户端，client==nil 守卫（prompt 先例——
// 无后端环境不启动探针），并启动首轮探针。
func (p *Plugin) Init(ctx context.Context, h kernel.Host) {
	p.st = h.State()
	p.client = h.Gateway()
	if p.client == nil {
		return
	}
	p.startProbe(h)
}

// Close 置停止位：在队的 tick/probe 回流后不再续订。
func (p *Plugin) Close(ctx context.Context) {
	p.mu.Lock()
	p.stopped = true
	p.mu.Unlock()
}

// startProbe 经 GoCmd 发起一次探针（两相形态之一：探针 cmd 只做
// Health，独立超时；间隔调度在 React 侧——S5 审计 P1-3 两相形态）。
func (p *Plugin) startProbe(h kernel.Host) {
	if p.stoppedByLock() {
		return // Close 停止位：在队 tick 回流后不再探针（S5 审计 P1-5）
	}
	client := p.client
	if client == nil {
		return // client==nil 双保险：Init 守卫外的防御（React tick 路径可达）
	}
	timeout := p.cfg.ProbeTimeout
	h.GoCmd(func() tea.Msg {
		probeCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		_, err := client.Health(probeCtx)
		return ProbeResultMsg{Err: err}
	})
}

// scheduleNext 经 GoCmd 定时续订下一轮探针（两相形态之二：调度与
// 探针执行分离）。stopped 停止位双查：React 与 tick 回流各自拦截。
func (p *Plugin) scheduleNext(h kernel.Host, interval time.Duration) {
	if p.stoppedByLock() {
		return
	}
	h.GoCmd(func() tea.Msg {
		if p.stoppedByLock() {
			return nil
		}
		time.Sleep(interval)
		// 睡眠期间的 Close 由 React(probeTickMsg)→startProbe 的停止位
		// 拦截（单一出处——闭包内二次检查与其重复，审计后移除）。
		return probeTickMsg{}
	})
}

// stoppedByLock 读取停止位。
func (p *Plugin) stoppedByLock() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stopped
}

// React 消费探针结果与健康事件：
//   - ProbeResultMsg：写 Connected 槽（err==nil 判据——HealthResult.OK
//     是死词表：fake 错误先返 error、real 恒 true，S6 审计 P2-1）+
//     边沿触发 Notify + 恢复广播 GatewayRecovered；无条件重武装下一轮
//     （全分支续订——循环静默死亡防线，S5 审计 P1-b）；
//   - probeTickMsg：发起下一轮探针；
//   - EventHealthChanged / EventGatewayOffline：Connected 槽直写
//     （health_changed 自 sessions 移交；gateway_offline 不走
//     ApplyGatewayForEvent——S6 审计实例2 注记）。
func (p *Plugin) React(h kernel.Host, msg tea.Msg) {
	switch m := msg.(type) {
	case ProbeResultMsg:
		healthy := m.Err == nil
		p.mu.Lock()
		previous := p.probeState
		if healthy {
			p.probeState = probeStateHealthy
			p.attemptCount = 0 // 成功归零回固定档（S5 审计注记）
			attempt := 1
			p.mu.Unlock()

			p.st.Gateway.Connected = true // Connected 槽写权归 health（S6 移交）
			if previous == probeStateUnhealthy {
				// 恢复边沿（unhealthy→healthy）：弱提示 + 广播恢复意图
				//（sessions 重绑事件流）；unknown→healthy 为启动首探，
				// 无断连可恢复，不广播。
				h.Notify("网关连接已恢复")
				h.Send(state.GatewayRecovered{})
			}
			p.scheduleNext(h, nextInterval(p.cfg, attempt))
			return
		}

		// 失败路径：进入/保持 unhealthy；unknown→unhealthy 也提示
		//（启动即连不上是用户需要知道的第一断连信号）。
		p.probeState = probeStateUnhealthy
		p.attemptCount++
		attempt := p.attemptCount + 1
		p.mu.Unlock()

		p.st.Gateway.Connected = false
		if previous != probeStateUnhealthy {
			h.Notify("网关连接断开：" + m.Err.Error())
		}
		p.scheduleNext(h, nextInterval(p.cfg, attempt))
	case probeTickMsg:
		p.startProbe(h)
	case gateway.GatewayEvent:
		switch m.Type {
		case gateway.EventHealthChanged:
			// 收敛到 ApplyGatewayForEvent（S6 审计实例1 P2-a：键集对齐
			// payloadBool 的 connected/ok，避免直取分叉）。
			state.ApplyGatewayForEvent(p.st, m)
		case gateway.EventGatewayOffline:
			// 直接槽写（ApplyGatewayForEvent 不加分支——S6 审计实例2 注记）。
			p.st.Gateway.Connected = false
		}
	}
}
