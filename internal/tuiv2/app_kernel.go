// Package tuiv2 提供 TUI v2 的内核装配根。
// 本文件是 S3-4 内核接线的产物（issue #31 装配、issue #41 接线切换）：
// 装配 kernel + 11 插件 + bootstrapReactor 替代旧 app 层路由，
// 并承载入口启动参数类型 StartupConfig（自 app.go 迁入）。
//
// 职责边界：本文件仅做装配（创建 + 注册 + 接线），不承载业务规则。
package tuiv2

import (
	"context"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/plugins/chat"
	"neo-code/internal/tuiv2/plugins/cmdline"
	"neo-code/internal/tuiv2/plugins/debug"
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

// StartupConfig 承载 TUI v2 独立入口解析出的启动参数和 Gateway 客户端。
// 自 app.go 迁入（issue #41 S3-4 删码前置步骤）：kernel 装配路径与
// cmd 入口均引用本类型，旧 app 层删除后由本文件作为唯一声明处。
type StartupConfig struct {
	Backend  string
	Scenario string
	Debug    bool
	Client   gateway.Client
}

// NewKernelApp 创建基于内核 + 插件的 TUI v2 应用。
// 返回值满足 tea.Model 契约，可直接传给 tea.NewProgram。
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
		// 调试行独立插件（单区域契约：不能并入 statusbar）；
		// 初值显隐与场景名对齐旧 app 层（debug: cfg.Debug，debugLine.scenario）。
		debug.New(cfg.Debug, cfg.Scenario),
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
