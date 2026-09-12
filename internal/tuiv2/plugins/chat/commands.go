package chat

import (
	"context"

	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// 本文件声明 chat 插件的对话域命令（统一注册表：palette / `:` / `/` 三入口
// 共用同一份数据，ADR-010）。

// Commands 声明对话域命令。
func (p *Plugin) Commands() []kernel.Command {
	return []kernel.Command{
		{
			Name:        "/clear",
			Aliases:     []string{"clear"},
			Description: "清空当前会话消息",
			Category:    "chat",
			Run:         p.runClear,
		},
		{
			Name:        "/cancel",
			Aliases:     []string{"cancel"},
			Description: "取消当前运行",
			Category:    "chat",
			Run:         p.runCancel,
		},
		{
			Name:        "/compact",
			Aliases:     []string{"compact"},
			Description: "压缩会话上下文",
			Category:    "chat",
			Run:         p.runCompact,
		},
		{
			Name:        "/retry",
			Aliases:     []string{"retry"},
			Description: "重试最近一次输入",
			Category:    "chat",
			Run:         p.runRetry,
		},
	}
}

// runClear 清空 Stream 槽（chat 拥有）并以弱提示确认。
func (p *Plugin) runClear(h kernel.Host, args []string) {
	p.st.Stream = nil
	p.st.Layout.AutoScroll = true
	p.st.Layout.ScrollOffset = 0
	h.Notify("已清空当前会话消息")
}

// runCancel 取消当前运行：空闲判定对齐旧路径 app_leader 语义——以
// Phase ∈ {running, waiting_permission, waiting_user} 为准而非 RunID
// （EventRunFinished 不清 RunID，stale RunID 会被误发 CancelRun，审计 P1-b）；
// 运行中经 GoCmd 异步调用 Gateway.CancelRun（RunID 读 Runtime 槽，
// sessionID 读 Gateway 槽活跃会话——均为合法读路径）。
func (p *Plugin) runCancel(h kernel.Host, args []string) {
	switch p.st.Runtime.Phase {
	case state.RuntimePhaseRunning, state.RuntimePhaseWaitingPermission, state.RuntimePhaseWaitingUser:
		// 可取消态，继续。
	default:
		return // 空闲/错误/已取消态静默 no-op
	}
	if p.client == nil {
		h.Notify("无可用后端，取消失败")
		return
	}
	sessionID := ""
	if p.st.Gateway.ActiveSess != nil {
		sessionID = p.st.Gateway.ActiveSess.ID
	}
	runID := p.st.Runtime.RunID
	client := p.client
	h.GoCmd(func() tea.Msg {
		_ = client.CancelRun(context.Background(), sessionID, runID)
		return nil
	})
}

// runCompact 占位：Gateway 契约当前无 compact RPC（ADR-002 ErrUnsupported
// 范畴），S3 契约收敛后接入。
func (p *Plugin) runCompact(h kernel.Host, args []string) {
	h.Notify("compact 尚未接入后端（S3 契约收敛后可用）")
}

// runRetry 重试最近一次输入。lastText 由用户提交路径填充——prompt 插件
// 迁移前恒空，诚实降级为提示（真实提交链路随 prompt PR 经广播接入）。
func (p *Plugin) runRetry(h kernel.Host, args []string) {
	if p.lastText == "" {
		h.Notify("没有可重试的历史输入")
		return
	}
	h.Notify("重试需要输入区接入（prompt 插件迁移后可用）")
}

// recordSubmittedText 供后续 prompt 插件的提交广播更新 lastText
// （本 PR 不接广播源，方法保留以钉住数据流向：仅 chat 私有，不暴露给其他插件）。
func (p *Plugin) recordSubmittedText(text string) {
	p.lastText = text
}
