package prompt

import (
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"
)

// 本文件声明 prompt 插件的命令（统一注册表，ADR-010）。
//
// 范围沿革：issue #25 修订 v2 / 审计 P1-4 选 (b) 当时仅 /exit 内联，
// 其余 slash 登记为"内核接线 PR 核对项"；issue #41（内核接线）落地时按
// 归宿迁移——/help 归 help 插件、/debug 归 debug 插件、/session /model
// 归 sessions/models 插件（均已注册），/mode 于本文件注册（S3-4 接线）。

// Commands 声明输入域命令。
func (p *Plugin) Commands() []kernel.Command {
	return []kernel.Command{
		{
			Name:        "/exit",
			Aliases:     []string{"exit", "q", "quit", "/quit"},
			Description: "退出 TUI v2",
			Category:    "prompt",
			Run: func(h kernel.Host, args []string) {
				h.Quit()
			},
		},
		{
			Name:        "/mode",
			Aliases:     []string{"mode"},
			Description: "切换 Agent 模式（build/plan）",
			Category:    "prompt",
			Run:         func(h kernel.Host, args []string) { p.toggleAgentMode(h) },
		},
	}
}

// toggleAgentMode 切换 Agent 模式（build↔plan）并经弱提示反馈。
// Runtime.AgentMode 子槽写权自本命令起归 prompt（issue #41；chat 包头
// 槽纪律同步登记）。语义对齐旧 app 层 toggleAgentMode：空值或 plan →
// build，build → plan。
func (p *Plugin) toggleAgentMode(h kernel.Host) {
	mode := state.AgentModePlan
	if p.st.Runtime.AgentMode == state.AgentModePlan || p.st.Runtime.AgentMode == "" {
		mode = state.AgentModeBuild
	}
	p.st.Runtime.AgentMode = mode
	h.Notify("Agent mode: " + mode)
}
