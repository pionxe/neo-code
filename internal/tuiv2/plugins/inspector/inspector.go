// Package inspector 是 TUI v2 的右侧软检查器插件（issue #27 S3-1）：
// 视图型插件（issue #23 分类）——仅实现 RegionRenderer，渲染委托
// components.SoftInspector，遵守与 chat 的数据边界（View 期只读，
// 禁写、不走广播）。
package inspector

import (
	"context"

	"neo-code/internal/tuiv2/components"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"
)

// Plugin 是软检查器插件。
type Plugin struct {
	st        *state.ViewState          // 全局唯一状态（Init 时固定指针，ADR-001）
	inspector *components.SoftInspector // 渲染委托（构造一次终身绑定）
}

// New 创建检查器插件实例。
func New() *Plugin { return &Plugin{} }

// ID 返回插件标识。
func (p *Plugin) ID() string { return "inspector" }

// Init 固定状态指针并构造渲染委托。
func (p *Plugin) Init(ctx context.Context, h kernel.Host) {
	p.st = h.State()
	p.inspector = components.NewSoftInspector(p.st)
}

// Close 释放资源（无外部资源，生命周期对称性）。
func (p *Plugin) Close(ctx context.Context) {}

// Region 返回右侧检查器区域。
func (p *Plugin) Region() kernel.RegionID { return kernel.RegionInspector }

// Render 委托 SoftInspector 渲染；ShowInspector=false 时组件输出空串，
// compose 按空渲染折叠该行（窄屏隐藏语义与旧布局一致）。
func (p *Plugin) Render(h kernel.Host, width int) string {
	if p.inspector == nil {
		return ""
	}
	return p.inspector.View()
}
