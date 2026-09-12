// Package cmdline 是 TUI v2 的命令行/搜索插件（issue #27 S3-3）：
// 拥有 Search/Ex 槽，提供 "/" 搜索与 ":" Ex 命令行（RegionCmdLine 内联行，
// 渲染空串即折叠）；Ex 执行经 Host.RunCommand（别名解析单一出处）。
package cmdline

import (
	"context"
	"strings"

	"neo-code/internal/tuiv2/components"
	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// Plugin 是命令行/搜索插件。
type Plugin struct {
	st      *state.ViewState    // 全局唯一状态（Init 时固定指针，ADR-001）
	cmdline *components.CmdLine // Ex/搜索输入渲染委托
}

// New 创建插件实例。
func New() *Plugin { return &Plugin{} }

// ID 返回插件标识。
func (p *Plugin) ID() string { return "cmdline" }

// Init 固定状态指针并构造委托。
func (p *Plugin) Init(ctx context.Context, h kernel.Host) {
	p.st = h.State()
	p.cmdline = components.NewCmdLine(p.st)
}

// Close 释放资源。
func (p *Plugin) Close(ctx context.Context) {}

// React 订阅广播：
//   - ModeChanged：切出 Normal 时清理 Search/Ex（承接旧路径
//     "切出 Normal 清理"——issue #25 核对项 4 的载体）；
//   - run_started 事件同清（旧路径 app.go:126 行为）。
func (p *Plugin) React(h kernel.Host, msg tea.Msg) {
	switch m := msg.(type) {
	case state.ModeChanged:
		if m.To != state.NormalMode {
			p.st.Search = state.SearchState{}
			p.st.Ex = state.ExState{}
		}
	case gateway.GatewayEvent:
		if m.Type == gateway.EventRunStarted {
			p.st.Search = state.SearchState{}
			p.st.Ex = state.ExState{}
		}
	}
}

// Region 返回命令行区域；Search/Ex 非激活时渲染空串 → 行折叠。
func (p *Plugin) Region() kernel.RegionID { return kernel.RegionCmdLine }

// Render 渲染激活的命令行（搜索输入或 Ex 输入）；均未激活返回空串。
func (p *Plugin) Render(h kernel.Host, width int) string {
	if p.st.Search.Active {
		return "查找: " + p.st.Search.Query + "▏（回车搜索，Esc 取消）"
	}
	if p.st.Ex.Active {
		return ":" + p.st.Ex.Input + "▏"
	}
	return ""
}

// Bindings 声明命令行键位（Normal 模式）：
//   - "/" 打开搜索、":" 打开 Ex（各自带 When 守卫：未激活时才打开）；
//   - 搜索/Ex 激活期间的按键捕获（When 守卫：激活时接管全部未精确命中键）；
//   - esc/enter 的关闭与提交。
//
// 守卫保证与 chat 滚动键（j/k/g/G）共存：未激活时这些键归 chat。
func (p *Plugin) Bindings() []kernel.Binding {
	return []kernel.Binding{
		{
			Mode:        state.NormalMode,
			Key:         "/",
			Description: "打开搜索",
			When:        func(s *state.ViewState) bool { return !s.Search.Active },
			OnKey: func(h kernel.Host) {
				p.st.Search = state.SearchState{Active: true}
			},
		},
		{
			Mode:        state.NormalMode,
			Key:         ":",
			Description: "打开 Ex 命令行",
			When:        func(s *state.ViewState) bool { return !s.Ex.Active },
			OnKey: func(h kernel.Host) {
				p.st.Ex = state.ExState{Active: true}
			},
		},
		{
			Mode:        state.NormalMode,
			Key:         "esc",
			Description: "关闭搜索/命令行",
			When:        func(s *state.ViewState) bool { return s.Search.Active || s.Ex.Active },
			OnKey: func(h kernel.Host) {
				p.st.Search = state.SearchState{}
				p.st.Ex = state.ExState{}
			},
		},
		{
			Mode:        state.NormalMode,
			Key:         "enter",
			Description: "提交搜索 / 执行 Ex 命令",
			When:        func(s *state.ViewState) bool { return s.Search.Active || s.Ex.Active },
			OnKey: func(h kernel.Host) {
				if p.st.Search.Active {
					p.executeSearch(h)
					return
				}
				if p.st.Ex.Active {
					cmd := strings.TrimSpace(p.st.Ex.Input)
					p.st.Ex = state.ExState{}
					if cmd == "" {
						return
					}
					if err := h.RunCommand(cmd, nil); err != nil {
						h.Notify("未知命令：" + cmd)
					}
				}
			},
		},
		{
			Mode:        state.NormalMode,
			Key:         "n",
			Description: "搜索结果下一个",
			When:        func(s *state.ViewState) bool { return len(p.st.Search.Matches) > 0 },
			OnKey:       func(h kernel.Host) { p.nextMatch(h, 1) },
		},
		{
			Mode:        state.NormalMode,
			Key:         "N",
			Description: "搜索结果上一个",
			When:        func(s *state.ViewState) bool { return len(p.st.Search.Matches) > 0 },
			OnKey:       func(h kernel.Host) { p.nextMatch(h, -1) },
		},
		{
			// 搜索/Ex 激活期间的字符与编辑键捕获（守卫保证仅在激活时生效，
			// chat 滚动键 j/k/g/G 在未激活时不受影响）。
			Mode:        state.NormalMode,
			Description: "搜索/命令行输入",
			When:        func(s *state.ViewState) bool { return s.Search.Active || s.Ex.Active },
			OnKeyMsg: func(h kernel.Host, msg tea.KeyMsg) {
				switch msg.String() {
				case "backspace", "ctrl+h":
					if p.st.Search.Active {
						if r := []rune(p.st.Search.Query); len(r) > 0 {
							p.st.Search.Query = string(r[:len(r)-1])
						}
					} else if p.st.Ex.Active {
						if r := []rune(p.st.Ex.Input); len(r) > 0 {
							p.st.Ex.Input = string(r[:len(r)-1])
						}
					}
				default:
					if len(msg.Runes) > 0 {
						insert := string(msg.Runes)
						if p.st.Search.Active {
							p.st.Search.Query += insert
						} else if p.st.Ex.Active {
							p.st.Ex.Input += insert
						}
					}
				}
			},
		},
	}
}

// executeSearch 全量扫描 Stream（只读 chat 槽）并记录匹配索引。
// 空查询为 no-op；无匹配提示。
func (p *Plugin) executeSearch(h kernel.Host) {
	query := strings.TrimSpace(p.st.Search.Query)
	if query == "" {
		return
	}
	lq := strings.ToLower(query)
	var matches []int
	for i, entry := range p.st.Stream {
		if strings.Contains(strings.ToLower(entry.Content), lq) {
			matches = append(matches, i)
		}
	}
	p.st.Search.Matches = matches
	p.st.Search.MatchIndex = 0
	p.st.Search.Stale = false
	p.st.Search.Active = false
	if len(matches) == 0 {
		h.Notify("无匹配结果")
		return
	}
	p.jumpTo(matches[0])
}

// nextMatch 在匹配间循环跳转（n/N）。
func (p *Plugin) nextMatch(h kernel.Host, delta int) {
	matches := p.st.Search.Matches
	if len(matches) == 0 {
		return
	}
	p.st.Search.MatchIndex = (p.st.Search.MatchIndex + delta + len(matches)) % len(matches)
	p.jumpTo(matches[p.st.Search.MatchIndex])
}

// jumpTo 滚动到指定条目（offset = 总条目 - 索引 - 1，与流渲染坐标一致）。
func (p *Plugin) jumpTo(index int) {
	total := len(p.st.Stream)
	p.st.Layout.ScrollOffset = total - index - 1
	p.st.Layout.AutoScroll = index == total-1
}
