package chat

import (
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// 本文件声明 chat 插件的滚动键位（issue #23 修订 v2 / 审计 P1-2）：
// 全部委托 AgentStream.Update——与旧 routeStreamKey 精确等价、零语义漂移。
// 键形状（keyRunes / tea.KeyMsg 构造）经 chat_test.go 的合成锁定测试约束。

// Bindings 声明 Normal 模式滚动键位。
func (p *Plugin) Bindings() []kernel.Binding {
	scroll := func(key tea.KeyMsg) func(h kernel.Host) {
		return func(h kernel.Host) { p.stream.Update(key) }
	}
	return []kernel.Binding{
		{Mode: state.NormalMode, Key: "j", Description: "流下滚一行", OnKey: scroll(keyRunes("j"))},
		{Mode: state.NormalMode, Key: "k", Description: "流上滚一行", OnKey: scroll(keyRunes("k"))},
		{Mode: state.NormalMode, Key: "g", Description: "流滚到顶部", OnKey: scroll(keyRunes("g"))},
		{Mode: state.NormalMode, Key: "G", Description: "流滚到底部", OnKey: scroll(keyRunes("G"))},
		{Mode: state.NormalMode, Key: "ctrl+d", Description: "流下翻半页", OnKey: scroll(tea.KeyMsg{Type: tea.KeyCtrlD})},
		{Mode: state.NormalMode, Key: "ctrl+u", Description: "流上翻半页", OnKey: scroll(tea.KeyMsg{Type: tea.KeyCtrlU})},
		{Mode: state.NormalMode, Key: "ctrl+f", Description: "流下翻整页", OnKey: scroll(tea.KeyMsg{Type: tea.KeyCtrlF})},
		{Mode: state.NormalMode, Key: "ctrl+b", Description: "流上翻整页", OnKey: scroll(tea.KeyMsg{Type: tea.KeyCtrlB})},
	}
}

// keyRunes 合成 runes 按键（与真实终端按键的 String() 等价，测试锁定）。
func keyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}
