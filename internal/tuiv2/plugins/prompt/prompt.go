// Package prompt 是 TUI v2 的输入区插件（issue #25）：拥有 Input 槽，
// 渲染底部输入区（消息/权限/问答三态），处理输入编辑键位与提交/权限/
// 问答链路。它是 kernel 契约键位通配扩展（OnKeyMsg）的第一个消费者。
//
// 槽纪律（ADR-009 / issue #25 修订 v2）：写 Input 槽（经 state.ApplyInputForEvent
// 的六类事件分支与提交清理）与 Runtime.AgentMode 子槽（/mode 命令，
// issue #41 接线登记；chat 包头纪律同步）；读 Stream/Runtime/Gateway。
package prompt

import (
	"context"
	"strings"
	"time"

	"neo-code/internal/tuiv2/components"
	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// Plugin 是输入区插件。
type Plugin struct {
	st     *state.ViewState          // 全局唯一状态（Init 时固定指针，ADR-001）
	prompt *components.CommandPrompt // 输入区渲染与 Input.Mode 状态机委托
	client gateway.Client            // Gateway 契约（Host.Gateway 透传）
}

// New 创建 prompt 插件实例。
func New() *Plugin { return &Plugin{} }

// ID 返回插件标识。
func (p *Plugin) ID() string { return "prompt" }

// Init 固定状态指针、构造输入区委托、记录 Gateway 客户端，
// 并启动光标闪烁（组件 CursorBlinkMsg 自续订经 React 回流）。
func (p *Plugin) Init(ctx context.Context, h kernel.Host) {
	p.st = h.State()
	p.prompt = components.NewCommandPrompt(p.st)
	p.client = h.Gateway()
	if cmd := p.prompt.Init(); cmd != nil {
		h.GoCmd(cmd)
	}
}

// Close 释放资源（当前无外部资源，保留生命周期对称性）。
func (p *Plugin) Close(ctx context.Context) {
	_ = ctx // 无外部资源：显式忽略参数使生命周期可覆盖
}

// React 订阅广播，承担三类职责：
//  1. 六类对话事件的 Input 槽写入（ApplyInputForEvent——移交自 chat 路径）；
//  2. 组件产出的业务消息（提交/权限/问答）转 Gateway RPC；
//  3. 光标闪烁续订。
func (p *Plugin) React(h kernel.Host, msg tea.Msg) {
	switch m := msg.(type) {
	case gateway.GatewayEvent:
		// 六类事件写 Input；非六类为 no-op（ApplyInputForEvent 内建）。
		state.ApplyInputForEvent(p.st, m)
	case components.SubmitMessageMsg:
		p.handleSubmit(h, m)
	case components.SlashCommandMsg:
		p.handleSlashCommand(h, m)
	case components.PermissionActionMsg:
		p.handlePermission(h, m)
	case components.QuestionAnswerMsg:
		p.handleQuestion(h, m)
	case components.PromptCancelMsg:
		// 旧路径追加流状态条目——该职责属 chat（Stream 槽），经 Notify 降级
		// 并登记接线 PR 核对项（issue #25 P2）。
		h.Notify("已取消当前输入")
	case components.CursorBlinkMsg:
		// 光标闪烁续订：委托组件翻转可见性并取回下一条续订命令。
		if _, cmd := p.prompt.Update(m); cmd != nil {
			h.GoCmd(cmd)
		}
	}
}

// handleSlashCommand 路由 slash 命令到统一命令注册表（RunCommand 是 Ex/slash
// 两入口的解析单一出处，ADR-010）。语义对齐旧 app 层 handleSlashCommand：
// 已知命令执行（含 /quit——/exit 别名表承接）、未知命令弱提示呈现。
// 空命令为 no-op（组件对非 "/" 输入产出 SubmitMessageMsg，此为脏数据防御）。
// React 分支自 issue #41 接线起补齐——此前 kernel 路径对 SlashCommandMsg
// 无任何消费者（审计 P1-2 实测静默丢弃）。
func (p *Plugin) handleSlashCommand(h kernel.Host, msg components.SlashCommandMsg) {
	cmd := strings.TrimSpace(msg.Command)
	if cmd == "" {
		return
	}
	args := []string(nil)
	if fields := strings.Fields(msg.Args); len(fields) > 0 {
		args = fields
	}
	if err := h.RunCommand(cmd, args); err != nil {
		h.Notify("unknown command: " + cmd)
	}
}

// handleSubmit 处理消息提交：非空校验 → 异步 SendMessage RPC →
// 广播 UserSubmitted（chat 据此更新 lastText 并追加 role=user 流条目）。
// RPC 失败经 gateway 回流的 EventError 端到端呈现（Phase=error + 流条目）。
func (p *Plugin) handleSubmit(h kernel.Host, msg components.SubmitMessageMsg) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}
	if p.client == nil {
		h.Notify("无可用后端，发送失败")
		return
	}
	if p.st.Gateway.ActiveSess == nil {
		h.Notify("无活跃会话，请先创建或切换会话")
		return
	}
	sessionID := p.st.Gateway.ActiveSess.ID
	client := p.client
	h.GoCmd(func() tea.Msg {
		// ACK/错误映射对齐旧路径 submitMessageCmd（审计 P1-2）：
		// ACK → EventRunStarted（chat 迁移 run 状态），错误 → EventError。
		ack, err := client.SendMessage(context.Background(), sessionID, text)
		if err != nil {
			return errorEvent(err)
		}
		return gateway.GatewayEvent{
			Type:      gateway.EventRunStarted,
			SessionID: ack.SessionID,
			RunID:     ack.RunID,
			Payload:   map[string]any{"message": ack.Message, "accepted": ack.Accepted},
			At:        time.Now(),
		}
	})
	h.Send(state.UserSubmitted{Text: text})
}

// handlePermission 处理权限快捷键：转 Gateway.ResolvePermission
// （字段填充对齐旧路径 app.go:448-453：Allow=y|a，Reason=决策字符）。
func (p *Plugin) handlePermission(h kernel.Host, msg components.PermissionActionMsg) {
	if p.client == nil {
		return
	}
	sessionID := ""
	if p.st.Gateway.ActiveSess != nil {
		sessionID = p.st.Gateway.ActiveSess.ID
	}
	decision := gateway.PermissionDecision{
		SessionID: sessionID,
		RunID:     p.st.Runtime.RunID,
		Allow:     msg.Decision == "y" || msg.Decision == "a",
		Reason:    msg.Decision,
	}
	client := p.client
	h.GoCmd(func() tea.Msg {
		// 完成映射对齐旧路径 resolvePermissionCmd（审计 P1-2）。
		if err := client.ResolvePermission(context.Background(), decision); err != nil {
			return errorEvent(err)
		}
		text := "permission denied"
		if decision.Allow {
			text = "permission allowed"
		}
		return gateway.GatewayEvent{
			Type:      gateway.EventPermissionResolved,
			SessionID: decision.SessionID,
			RunID:     decision.RunID,
			Payload:   map[string]any{"decision": decision.Reason, "message": text},
			At:        time.Now(),
		}
	})
}

// handleQuestion 处理 ask_user 回答提交：转 Gateway.AnswerUserQuestion
// （Text 字段对齐旧路径 app.go:458-464）。
func (p *Plugin) handleQuestion(h kernel.Host, msg components.QuestionAnswerMsg) {
	if p.client == nil {
		return
	}
	sessionID := ""
	if p.st.Gateway.ActiveSess != nil {
		sessionID = p.st.Gateway.ActiveSess.ID
	}
	answer := gateway.UserQuestionAnswer{
		SessionID: sessionID,
		RunID:     p.st.Runtime.RunID,
		Text:      msg.Text,
	}
	client := p.client
	h.GoCmd(func() tea.Msg {
		// 完成映射对齐旧路径 answerQuestionCmd（审计 P1-2）。
		if err := client.AnswerUserQuestion(context.Background(), answer); err != nil {
			return errorEvent(err)
		}
		return gateway.GatewayEvent{
			Type:      gateway.EventUserQuestionAnswered,
			SessionID: answer.SessionID,
			RunID:     answer.RunID,
			Payload:   map[string]any{"answer": answer.Text, "message": "answer submitted"},
			At:        time.Now(),
		}
	})
}

// errorEvent 将 RPC 错误包装成统一错误事件（对齐旧路径 errorEvent），
// 经 kernel 广播回流 chat → Phase=error + 流条目（端到端失败呈现）。
func errorEvent(err error) gateway.GatewayEvent {
	return gateway.GatewayEvent{
		Type:    gateway.EventError,
		Payload: map[string]any{"message": err.Error()},
		At:      time.Now(),
	}
}

// Region 返回底部输入区。
func (p *Plugin) Region() kernel.RegionID { return kernel.RegionPrompt }

// Render 委托 CommandPrompt 渲染（组件按 Input.Mode 呈现消息/权限/问答
// 三态视图与模式指示条；宽度契约与 chat 一致：组件读 Layout.Width）。
func (p *Plugin) Render(h kernel.Host, width int) string {
	if p.prompt == nil {
		return ""
	}
	return p.prompt.View()
}
