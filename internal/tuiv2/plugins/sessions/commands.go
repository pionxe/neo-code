package sessions

import (
	"context"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// 本文件声明会话域命令（统一注册表，ADR-010）：
// /new 新建、/session 选择器、/delete 删除（确认框）。

// Commands 声明会话域命令。
func (p *Plugin) Commands() []kernel.Command {
	return []kernel.Command{
		{
			Name:        "/new",
			Aliases:     []string{"new"},
			Description: "新建会话",
			Category:    "session",
			Run:         p.runNew,
		},
		{
			Name:        "/session",
			Aliases:     []string{"session"},
			Description: "打开会话选择器",
			Category:    "session",
			Run:         p.runSession,
		},
		{
			Name:        "/delete",
			Aliases:     []string{"delete"},
			Description: "删除当前会话",
			Category:    "session",
			Run:         p.runDelete,
		},
	}
}

// runNew 新建会话：闭包只执行 RPC 并返回合成事件（P0-1 并发契约——
// 无 Host 副作用）；槽迁移与通知由 React 完成（session_created 迁移 +
// EventError 由 chat 呈现）。冗余 session_updated 已去除（审计 P2-②）。
func (p *Plugin) runNew(h kernel.Host, args []string) {
	if p.client == nil {
		h.Notify("无可用后端，创建失败")
		return
	}
	client := p.client
	h.GoCmd(func() tea.Msg {
		summary, err := client.CreateSession(context.Background())
		if err != nil {
			return gateway.GatewayEvent{
				Type:    gateway.EventError,
				Payload: map[string]any{"message": "创建会话失败：" + err.Error()},
			}
		}
		return gateway.GatewayEvent{
			Type:      gateway.EventSessionCreated,
			SessionID: summary.ID,
			Payload:   map[string]any{"id": summary.ID, "title": summary.Title, "model": summary.Model},
		}
	})
}

// runSession 打开会话选择器浮层。
func (p *Plugin) runSession(h kernel.Host, args []string) {
	p.openPicker(h)
}

// runDelete 删除当前会话：走内核确认服务；确认后经 SessionDeleted 广播
// 由 React 复用 reducer 既有分支迁移槽（单一出处）。
// 已知差距：本地合成删除在真实后端重启后会话复活（契约无 deleteSession
// RPC，ADR-002 ErrUnsupported 语义；后端补 RPC 后接入）。
func (p *Plugin) runDelete(h kernel.Host, args []string) {
	id := p.activeSessionID()
	if id == "" {
		h.Notify("无活跃会话可删除")
		return
	}
	h.Confirm(state.ConfirmRequest{
		Title:   "⚠ 删除会话",
		Message: "确定删除当前会话？此操作不可撤销。",
		Action:  "delete_session",
		Data:    map[string]any{"id": id},
	})
}
