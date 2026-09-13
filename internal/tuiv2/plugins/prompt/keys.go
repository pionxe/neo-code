package prompt

import (
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// 本文件声明 prompt 插件的键位（issue #25 修订 v2 / 审计 P0-2 裁决）：
//   - Input 模式仅两条：esc 精确键（→Normal）+ 单条通配 OnKeyMsg
//     （原始 KeyMsg 全权委托 CommandPrompt.Update——组件内按 Input.Mode
//     三分支状态机处理编辑键/任意字符/权限 y/n/a/问答文本）；
//   - Normal 模式：i 精确键（→Input）。
//
// 否决方案留档（审计第 1 轮）：权限 y/n/a 精确 Binding 会劫持 Input 模式
// 打字（Input.Mode 与 Binding.Mode 正交）；KeyClass 枚举随插件膨胀；
// "未匹配键兜底交输入插件"通道破坏独占语义。

// Bindings 声明 prompt 键位。
func (p *Plugin) Bindings() []kernel.Binding {
	return []kernel.Binding{
		{
			Mode:        state.InputModeInput,
			Key:         "esc",
			Description: "退出输入模式（回到 Normal）",
			OnKey: func(h kernel.Host) {
				h.SetMode(state.NormalMode)
			},
		},
		{
			Mode:        state.InputModeInput,
			Description: "输入编辑（字符插入/删除/光标/提交/权限/问答）",
			OnKeyMsg: func(h kernel.Host, msg tea.KeyMsg) {
				p.delegateUpdate(h, msg)
			},
		},
		{
			Mode:        state.NormalMode,
			Key:         "i",
			Description: "进入输入模式",
			// When 守卫：搜索/Ex 激活期 "i" 是 cmdline 通配绑定的查询字符
			//（issue #41 PR 审计 P1-1——无守卫时输入 "i" 经 SetMode 直接
			// 切走模式、杀掉搜索）。
			When: func(s *state.ViewState) bool { return !s.Search.Active && !s.Ex.Active },
			OnKey: func(h kernel.Host) {
				h.SetMode(state.InputModeInput)
			},
		},
	}
}

// delegateUpdate 将原始按键透传给组件状态机；组件产出的业务消息
// （SubmitMessageMsg 等）作为命令回流 kernel Update 后广播回本插件。
func (p *Plugin) delegateUpdate(h kernel.Host, msg tea.KeyMsg) {
	if p.prompt == nil {
		return
	}
	if cmd := p.prompt.Update(msg); cmd != nil {
		h.GoCmd(cmd)
	}
}
