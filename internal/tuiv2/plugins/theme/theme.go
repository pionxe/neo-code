// Package theme 是 TUI v2 的主题插件（issue #27 S3-3 / ADR-013）：
// 主题 = 插件文件 + 命令。内置 Tokyo Night 两套（真彩/ASCII 符号），
// 自定义主题 = 新增一个插件文件（提供 /theme xxx 命令）+ 装配处注册。
//
// 纪律（审计裁定）：SetActive 仅限 Update/React 同步路径调用
// （bubbletea View 与 Update 同 goroutine，tea.Cmd goroutine 内调用即竞态）；
// 全组件参数化（state.Theme）登记 S11。
package theme

import (
	"context"

	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/theme"
)

// Plugin 是主题插件。
type Plugin struct{}

// New 创建主题插件实例。
func New() *Plugin { return &Plugin{} }

// ID 返回插件标识。
func (p *Plugin) ID() string { return "theme" }

// Init 生命周期占位。
func (p *Plugin) Init(ctx context.Context, h kernel.Host) {}

// Close 释放资源。
func (p *Plugin) Close(ctx context.Context) {}

// Commands 声明主题切换命令：每个命令把对应色板/符号集写入 theme 包
// 活动配置（包级原子替换，渲染器下一帧生效）。
func (p *Plugin) Commands() []kernel.Command {
	return []kernel.Command{
		{
			Name:        "/theme tokyo-night",
			Aliases:     []string{"theme tokyo-night"},
			Description: "切换到 Tokyo Night 主题（Unicode 符号）",
			Category:    "theme",
			Run: func(h kernel.Host, args []string) {
				theme.SetSymbolMode("unicode")
				h.Notify("主题已切换：Tokyo Night")
			},
		},
		{
			Name:        "/theme tokyo-night-ascii",
			Aliases:     []string{"theme tokyo-night-ascii"},
			Description: "切换到 Tokyo Night 主题（ASCII 符号降级）",
			Category:    "theme",
			Run: func(h kernel.Host, args []string) {
				theme.SetSymbolMode("ascii")
				h.Notify("主题已切换：Tokyo Night ASCII")
			},
		},
	}
}
