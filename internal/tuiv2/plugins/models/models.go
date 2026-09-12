// Package models 是 TUI v2 的模型管理插件（issue #27 S3-2）：
// 拥有 Gateway.Models/ActiveModel 子槽，提供模型切换与模型选择器浮层。
package models

import (
	"context"
	"fmt"
	"time"

	"neo-code/internal/tuiv2/components"
	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// Plugin 是模型管理插件。
type Plugin struct {
	st     *state.ViewState        // 全局唯一状态（Init 时固定指针，ADR-001）
	client gateway.Client          // Gateway 契约
	picker *components.ModelPicker // 模型选择器委托
}

// New 创建模型插件实例。
func New() *Plugin { return &Plugin{} }

// ID 返回插件标识。
func (p *Plugin) ID() string { return "models" }

// Init 固定状态指针、构造选择器委托并记录客户端。
func (p *Plugin) Init(ctx context.Context, h kernel.Host) {
	p.st = h.State()
	p.client = h.Gateway()
	p.picker = components.NewModelPicker(p.st)
}

// Close 释放资源（无外部资源，生命周期对称性）。
func (p *Plugin) Close(ctx context.Context) {}

// React 订阅广播：model_changed 迁移 ActiveModel 子槽；模型选择产出
// 消息（经选择器浮层）→ SetModel RPC → 成功后合成 model_changed 回流
// （写权归本插件，对账表见 issue #27）。
func (p *Plugin) React(h kernel.Host, msg tea.Msg) {
	switch m := msg.(type) {
	case gateway.GatewayEvent:
		// 精确化（审计 P2-①）：models 仅写 model_changed（其余 Gateway
		// 域事件归 sessions，避免全量转发稀释"写者唯一"）。
		if m.Type == gateway.EventModelChanged {
			state.ApplyGatewayForEvent(p.st, m)
		}
	case components.ModelSelectMsg:
		p.handleSelect(h, m)
	}
}

// handleSelect 处理模型切换：SetModel RPC；成功后合成 model_changed
// 事件回流（本插件 React 迁移 ActiveModel），失败追加 Notify。
func (p *Plugin) handleSelect(h kernel.Host, msg components.ModelSelectMsg) {
	if p.client == nil {
		h.Notify("无可用后端，切换失败")
		return
	}
	sessionID := ""
	if p.st.Gateway.ActiveSess != nil {
		sessionID = p.st.Gateway.ActiveSess.ID
	}
	client := p.client
	h.GoCmd(func() tea.Msg {
		if err := client.SetModel(context.Background(), sessionID, msg.ModelID); err != nil {
			return gateway.GatewayEvent{
				Type:    gateway.EventError,
				Payload: map[string]any{"message": fmt.Sprintf("Failed to switch model: %s", err)},
				At:      time.Now(),
			}
		}
		return gateway.GatewayEvent{
			Type:    gateway.EventModelChanged,
			Payload: map[string]any{"model_id": msg.ModelID},
			At:      time.Now(),
		}
	})
}

// pickerOverlay 是模型选择器浮层（委托 components.ModelPicker，ADR-011
// 修订登记同 sessions）。
type pickerOverlay struct {
	p *Plugin
}

// ID 返回浮层标识。
func (o *pickerOverlay) ID() string { return "models.picker" }

// HandleKey 全权委托组件状态机（模态消费）。
func (o *pickerOverlay) HandleKey(h kernel.Host, msg tea.KeyMsg) (consumed bool) {
	if _, cmd := o.p.picker.Update(msg); cmd != nil {
		h.GoCmd(cmd)
	}
	return true
}

// View 委托组件渲染。
func (o *pickerOverlay) View(h kernel.Host, width int) string {
	return o.p.picker.View()
}

// Bindings 声明 Leader 键位：Space m 打开模型选择器。
func (p *Plugin) Bindings() []kernel.Binding {
	return []kernel.Binding{
		{
			Mode:        state.LeaderMode,
			Key:         "m",
			Description: "打开模型选择器",
			OnKey: func(h kernel.Host) {
				h.PushOverlay(&pickerOverlay{p: p})
			},
		},
	}
}

// Commands 声明模型域命令。
func (p *Plugin) Commands() []kernel.Command {
	return []kernel.Command{
		{
			Name:        "/model",
			Aliases:     []string{"model"},
			Description: "打开模型选择器",
			Category:    "model",
			Run: func(h kernel.Host, args []string) {
				h.PushOverlay(&pickerOverlay{p: p})
			},
		},
	}
}
