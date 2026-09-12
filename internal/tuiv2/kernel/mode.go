package kernel

import (
	"time"

	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// Options 是内核机制的可注入参数（AGENTS.md 配置先行：不硬编码时序）。
type Options struct {
	// LeaderTimeout 是 Leader 模式无后续按键时自动回落 Normal 的等待时长。
	LeaderTimeout time.Duration
	// NotifyExpiry 是弱提示在状态栏的停留时长，到期由内核清除 Notify 槽。
	NotifyExpiry time.Duration
	// MaxQueueDepth 兼任两职：广播队列深度上限 + 每轮 drain 的派发预算。
	// 自激回声（插件处理时再 Send）超过预算即清空余量 + debug 日志，
	// 与 §4.1 规则 1 的"拒绝语义"同风格。
	MaxQueueDepth int
	// Debug 开启调试行与内核 debug 日志。
	Debug bool
}

// setDefaults 补齐零值参数的机制级默认值。
func (o Options) setDefaults() Options {
	if o.LeaderTimeout <= 0 {
		o.LeaderTimeout = 1 * time.Second
	}
	if o.NotifyExpiry <= 0 {
		o.NotifyExpiry = 4 * time.Second
	}
	if o.MaxQueueDepth <= 0 {
		o.MaxQueueDepth = 64
	}
	return o
}

// leaderTimeoutMsg 是 Leader 超时的内核私有消息：永不入广播（§4.1 契约）。
// gen 携带进入 Leader 模式时的代际号，旧定时器到期即失效。
type leaderTimeoutMsg struct{ gen int }

// notifyExpiryMsg 是弱提示到期的内核私有消息：永不入广播，按代际对号清除。
type notifyExpiryMsg struct{ gen int }

// modeMachine 是键位模式机（Input/Normal/Leader 三态）。
// 模式归属权唯一归内核：插件只经 Host.SetMode 切换（如绑定 space→Leader），
// 进入 Leader 时内核武装超时（代际号递增，旧定时器到期失效）。
// 按键路由三级优先级（§4.1，裁决在 kernel.go 的 dispatchKey）：
//  1. 内核保留键：ctrl+c 请求退出；esc 在浮层栈非空时先问栈顶；
//  2. 浮层栈顶（独占，未消费即丢弃）；
//  3. 当前模式的注册绑定；未命中即丢弃（Leader 未命中额外静默回落 Normal）。
type modeMachine struct {
	mode      state.InputMode
	leaderGen int           // Leader 代际号：每次进入递增
	timeout   time.Duration // Leader 超时时长（Options 注入）
	debugf    func(format string, args ...any)
}

// setMode 切换键位模式；进入 Leader 时返回武装超时的定时器命令（交还 Update 回流）。
func (m *modeMachine) setMode(next state.InputMode) tea.Cmd {
	if next == m.mode {
		return nil
	}
	m.mode = next
	if next != state.LeaderMode {
		return nil
	}
	m.leaderGen++
	gen := m.leaderGen
	timeout := m.timeout
	return tea.Tick(timeout, func(time.Time) tea.Msg {
		return leaderTimeoutMsg{gen: gen}
	})
}

// onLeaderTimeout 处理超时：仅当代际号匹配且仍处 Leader 模式时静默回落 Normal。
func (m *modeMachine) onLeaderTimeout(msg leaderTimeoutMsg) {
	if m.mode != state.LeaderMode || msg.gen != m.leaderGen {
		return
	}
	if m.debugf != nil {
		m.debugf("leader timeout, back to normal")
	}
	m.mode = state.NormalMode
}
