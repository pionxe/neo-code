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
//
// 弹栈语义（P0-2 修复）：esc/enter/ctrl+d 三键由本浮层关闭自身
// （h.PopOverlay）；其余键委托组件且模态消费。
type pickerOverlay struct {
	p *Plugin
}

// ID 返回浮层标识。
func (o *pickerOverlay) ID() string { return "sessions.picker" }

// HandleKey 全权委托组件状态机（模态：全部按键消费）；
// 组件产出的业务消息经组件 Update 返回命令回流广播。
func (o *pickerOverlay) HandleKey(h kernel.Host, msg tea.KeyMsg) (consumed bool) {
	switch msg.String() {
	case "esc":
		h.PopOverlay()
		return true
	case "enter", "ctrl+d":
		if _, cmd := o.p.picker.Update(msg); cmd != nil {
			h.GoCmd(cmd) // 选择/删除消息回流广播；浮层随选择关闭
		}
		h.PopOverlay()
		return true
	}
	// 其余键委托组件（过滤/导航——组件对这些键不产出命令）。
	_, _ = o.p.picker.Update(msg)
	return true
}

// View 委托组件渲染。
func (o *pickerOverlay) View(h kernel.Host, width int) string {
	return o.p.picker.View()
}

// HandleMouse 委托 picker.Update 处理滚轮滚动（仅 WheelUp/WheelDown；
// Left/Motion 吞掉——picker Align(Center) 垂直居中 Y 映射在 kernel
// 路径必错，S8 审计实例2 实测确认）。
func (o *pickerOverlay) HandleMouse(h kernel.Host, msg tea.MouseMsg) (consumed bool) {
	switch msg.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		o.p.picker.Update(msg)
		return true
	default:
		return true // 模态消费：面板开启期间所有鼠标事件不穿透
	}
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
