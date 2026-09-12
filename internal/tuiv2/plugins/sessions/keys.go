package sessions

import (
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// pickerOverlay 是会话选择器浮层（Overlay 能力，ADR-011）：
// 交互委托 components.SessionPicker（组件按 Overlay.Query/Selected 维护
// 过滤与选中——该全局态的 ADR-011 修订已显式登记，S11 参数化）。
// 组件产出的 SessionSelectMsg/SessionDeleteMsg 经 GoCmd 回流广播，
// 由插件 React 消费（与 prompt 插件的委托同模式）。
type pickerOverlay struct {
	p *Plugin
}

// ID 返回浮层标识。
func (o *pickerOverlay) ID() string { return "sessions.picker" }

// HandleKey 全权委托组件状态机（模态：全部按键消费）；
// 组件产出的业务消息经组件 Update 返回命令回流广播。
func (o *pickerOverlay) HandleKey(h kernel.Host, msg tea.KeyMsg) (consumed bool) {
	if _, cmd := o.p.picker.Update(msg); cmd != nil {
		h.GoCmd(cmd)
	}
	return true
}

// View 委托组件渲染。
func (o *pickerOverlay) View(h kernel.Host, width int) string {
	return o.p.picker.View()
}

// openPicker 压入会话选择器浮层（Space s / /session 入口）。
func (p *Plugin) openPicker(h kernel.Host) {
	h.PushOverlay(&pickerOverlay{p: p})
}

// Bindings 声明 Leader 键位：Space s 打开会话选择器。
func (p *Plugin) Bindings() []kernel.Binding {
	return []kernel.Binding{
		{
			Mode:        state.LeaderMode,
			Key:         "s",
			Description: "打开会话选择器",
			OnKey:       func(h kernel.Host) { p.openPicker(h) },
		},
	}
}
