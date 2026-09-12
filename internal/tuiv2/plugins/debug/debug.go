// Package debug 是 TUI v2 的调试行插件（issue #41 内核接线补齐）：
// 认领 RegionDebug 渲染最小运行信息，并提供 /debug 命令切换显隐。
// 独立插件实例的缘由（审计裁定）：kernel.Register 每实例仅认领一个
// Region（单区域契约），statusbar 已占 RegionStatusBar，不能复用。
// 初值与场景名由装配处注入（对齐旧 app 层 debug: cfg.Debug 与
// debugLine 的 scenario 字段），插件自身不读启动配置。
package debug

import (
	"context"
	"fmt"

	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"
)

// Plugin 是调试行插件：显隐开关为插件自持交互状态，不进全局 ViewState。
type Plugin struct {
	enabled  bool   // 调试行显隐开关（/debug 切换；初值由装配注入）
	scenario string // 场景名（渲染字段；对齐旧 debugLine）
}

// New 创建调试行插件：enabled 为初值显隐，scenario 为渲染的场景名。
func New(enabled bool, scenario string) *Plugin {
	return &Plugin{enabled: enabled, scenario: scenario}
}

// ID 返回插件标识。
func (p *Plugin) ID() string { return "debug" }

// Init 生命周期占位。
func (p *Plugin) Init(ctx context.Context, h kernel.Host) {}

// Close 释放资源（无外部资源，生命周期对称性）。
func (p *Plugin) Close(ctx context.Context) {}

// Region 返回调试行区域。
func (p *Plugin) Region() kernel.RegionID { return kernel.RegionDebug }

// Render 渲染调试行：关闭时返回空串（compose 折叠该行）；
// 字段对齐旧 app 层 debugLine（mode=按键模式/scenario/events/size）。
// Render 为纯读（RegionRenderer 契约），不变更任何状态。
func (p *Plugin) Render(h kernel.Host, width int) string {
	if !p.enabled || h.State() == nil {
		return ""
	}
	st := h.State()
	// 尺寸缺省展示 "0x0"（对齐旧 defaultTerminal 常量语义），
	// 任一维 >0 时展示实际 WxH。
	size := "0x0"
	if st.Layout.Width > 0 || st.Layout.Height > 0 {
		size = fmt.Sprintf("%dx%d", st.Layout.Width, st.Layout.Height)
	}
	return fmt.Sprintf("[debug] mode:%s  scenario:%s  events:%d  size:%s",
		modeName(st.Mode), p.scenario, len(st.Stream), size)
}

// Commands 声明 /debug 命令：切换显隐并经弱提示反馈（文案对齐旧 toggleDebug）。
// Aliases 含无斜杠 "debug"：Ex 行（:debug）与 slash（/debug）双入口可达。
func (p *Plugin) Commands() []kernel.Command {
	return []kernel.Command{
		{
			Name:        "/debug",
			Aliases:     []string{"debug"},
			Description: "切换调试行显隐",
			Category:    "debug",
			Run: func(h kernel.Host, args []string) {
				p.enabled = !p.enabled
				h.Notify(fmt.Sprintf("Debug: %v", p.enabled))
			},
		},
	}
}

// modeName 将键位模式映射为调试行短名（对齐旧 app 层 inputModeName）。
func modeName(mode state.InputMode) string {
	switch mode {
	case state.NormalMode:
		return "normal"
	case state.LeaderMode:
		return "leader"
	default:
		return "input"
	}
}
