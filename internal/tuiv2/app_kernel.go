// Package tuiv2 提供 TUI v2 的内核装配根。
// 本文件是 S3-4 内核接线的产物（issue #31）：
// 装配 kernel + 10 插件替代旧 app 层路由。
//
// 职责边界：本文件仅做装配（创建 + 注册 + 接线），不承载业务规则。
package tuiv2

import (
	"context"

	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/plugins/chat"
	"neo-code/internal/tuiv2/plugins/cmdline"
	"neo-code/internal/tuiv2/plugins/help"
	"neo-code/internal/tuiv2/plugins/inspector"
	"neo-code/internal/tuiv2/plugins/models"
	"neo-code/internal/tuiv2/plugins/palette"
	"neo-code/internal/tuiv2/plugins/prompt"
	"neo-code/internal/tuiv2/plugins/sessions"
	"neo-code/internal/tuiv2/plugins/statusbar"
	"neo-code/internal/tuiv2/plugins/theme"
	"neo-code/internal/tuiv2/state"
)

// NewKernelApp 创建基于内核 + 插件的 TUI v2 应用。
// 返回值满足 tea.Model 契约，可直接传给 tea.NewProgram。
// 复用旧 StartupConfig 类型（由 app.go 声明）以保持参数兼容。
func NewKernelApp(ctx context.Context, cfg StartupConfig) *kernel.Kernel {
	_ = ctx // 预留：插件 Init 可能需要请求作用域取消
	st := state.NewViewState()
	k := kernel.NewKernel(kernel.Config{
		Client: cfg.Client,
		State:  st,
		Debug:  cfg.Debug,
	})

	// 注册顺序决定插件 Init 的调用序（kernel 区域拼装由 kernel.regionOrder
	// 决定，与注册顺序无关）；Register 对 ID/区域/绑定/命令冲突 fail-fast。
	plugins := []kernel.Plugin{
		statusbar.New(),
		inspector.New(),
		chat.New(),
		prompt.New(),
		cmdline.New(),
		sessions.New(),
		models.New(),
		palette.New(),
		help.New(),
		theme.New(),
	}
	for _, p := range plugins {
		if err := k.Register(p); err != nil {
			// 装配阶段 fail-fast：非法插件尽早暴露。
			panic(err)
		}
	}

	// bootstrapReactor 消费 bootstrapDoneMsg 并在主 goroutine 落地状态。
	if err := k.Register(newBootstrapReactor(cfg.Client)); err != nil {
		panic(err) // bootstrapReactor ID 冲突 fail-fast
	}

	return k
}
