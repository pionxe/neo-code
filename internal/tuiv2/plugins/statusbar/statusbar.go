// Package statusbar 是 TUI v2 的顶部状态栏插件（issue #27 S3-1）：
// 视图型插件（issue #23 分类）——仅实现 RegionRenderer，渲染委托
// components.AmbientStatus，自身不演化、不持有交互状态。
package statusbar

import (
	"context"

	"neo-code/internal/tuiv2/components"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"
)

// Plugin 是状态栏插件。
type Plugin struct {
	st     *state.ViewState          // 全局唯一状态（Init 时固定指针，ADR-001）
	status *components.AmbientStatus // 渲染委托（构造一次终身绑定）
}

// New 创建状态栏插件实例。
func New() *Plugin { return &Plugin{} }

// ID 返回插件标识。
func (p *Plugin) ID() string { return "statusbar" }

// Init 固定状态指针并构造渲染委托。
func (p *Plugin) Init(ctx context.Context, h kernel.Host) {
	p.st = h.State()
	p.status = components.NewAmbientStatus(p.st)
}

// Close 释放资源（无外部资源，生命周期对称性）。
func (p *Plugin) Close(ctx context.Context) {}

// Region 返回顶部状态栏区域。
func (p *Plugin) Region() kernel.RegionID { return kernel.RegionStatusBar }

// Render 委托 AmbientStatus 渲染：width 参数透传为唯一宽度真相源
// （S7 真相源统一，issue #50——消除 Layout.Width 首帧零值错位）。
func (p *Plugin) Render(h kernel.Host, width int) string {
	if p.status == nil {
		return ""
	}
	return p.status.View(width)
}
