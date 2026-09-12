package prompt

import "neo-code/internal/tuiv2/kernel"

// 本文件声明 prompt 插件的命令（统一注册表，ADR-010）。
//
// 范围裁决（issue #25 修订 v2 / 审计 P1-4 选 (b)）：本 PR 仅 /exit 内联；
// 全量 slash 路由（/help /session /model /mode /debug）登记为内核接线 PR
// 核对项，按归宿由对应插件迁移（避免 kernel 契约二次变更）。

// Commands 声明输入域命令。
func (p *Plugin) Commands() []kernel.Command {
	return []kernel.Command{
		{
			Name:        "/exit",
			Aliases:     []string{"exit", "q"},
			Description: "退出 TUI v2",
			Category:    "prompt",
			Run: func(h kernel.Host, args []string) {
				h.Quit()
			},
		},
	}
}
