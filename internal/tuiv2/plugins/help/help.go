// Package help 是 TUI v2 的帮助面板插件（issue #27 S3-3）：
// 视图型 + Overlay——内容从内核绑定注册表快照自动生成（ADR-010），
// 不再手工维护键位文档（消灭 keep-in-sync）。
package help

import (
	"context"
	"sort"
	"strings"

	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// Plugin 是帮助面板插件。
type Plugin struct{}

// New 创建帮助插件实例。
func New() *Plugin { return &Plugin{} }

// ID 返回插件标识。
func (p *Plugin) ID() string { return "help" }

// Init 生命周期占位。
func (p *Plugin) Init(ctx context.Context, h kernel.Host) {
	_ = ctx
}

// Close 释放资源（无外部资源，生命周期对称性）。
func (p *Plugin) Close(ctx context.Context) {
	_ = ctx
}

// open 构造并压入帮助浮层：内容 = 内核保留键 + 全部注册绑定（按 Group/Mode 分组）。
func (p *Plugin) open(h kernel.Host) {
	h.PushOverlay(&overlay{h: h})
}

// overlay 是帮助浮层（内容即注册表快照的渲染）。
type overlay struct{ h kernel.Host }

// ID 返回浮层标识。
func (o *overlay) ID() string { return "help" }

// HandleKey 任意键关闭（帮助为只读浮层）。
func (o *overlay) HandleKey(h kernel.Host, msg tea.KeyMsg) (consumed bool) {
	h.PopOverlay()
	return true
}

// modeName 返回键位模式的展示名。
func modeName(m state.InputMode) string {
	switch m {
	case state.InputModeInput:
		return "输入模式"
	case state.LeaderMode:
		return "Leader 模式"
	default:
		return "Normal 模式"
	}
}

// View 渲染帮助：按 Mode 分组列出全部绑定（Group 字段为分组标题）。
func (o *overlay) View(h kernel.Host, width int) string {
	var b strings.Builder
	b.WriteString("┌─ ⌨ 帮助（任意键关闭）─────────────────────┐\n")
	b.WriteString("│ 内核：ctrl+c 退出程序\n")
	for _, bd := range h.Bindings() {
		group := bd.Group
		if group == "" {
			group = modeName(bd.Mode)
		}
		b.WriteString("│ " + group + "  " + bd.Key + "  " + bd.Description + "\n")
	}
	b.WriteString("└──────────────────────────────────────────┘")
	return b.String()
}

// Bindings 声明帮助打开键：Leader h。
func (p *Plugin) Bindings() []kernel.Binding {
	return []kernel.Binding{
		{
			Mode:        state.LeaderMode,
			Key:         "h",
			Description: "打开帮助面板",
			OnKey: func(h kernel.Host) {
				p.open(h)
			},
		},
	}
}

// sortedKeys 供测试断言分组稳定性（按组名排序的键列表）。
func sortedKeys(bindings []kernel.Binding) []string {
	out := make([]string, 0, len(bindings))
	for _, b := range bindings {
		out = append(out, b.Key)
	}
	sort.Strings(out)
	return out
}
