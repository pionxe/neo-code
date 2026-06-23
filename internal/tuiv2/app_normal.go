// app_normal.go 承载 Normal Mode 的按键路由与命令行/搜索辅助函数，
// 从 app.go 拆分以控制单文件行数（plan-v4 Step 8）。
package tuiv2

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"neo-code/internal/tuiv2/components"
	"neo-code/internal/tuiv2/keymap"
	"neo-code/internal/tuiv2/state"
)

// handleNormalModeKey 处理 Normal Mode 下的键盘输入。
//
// 分层约定（plan-v4）：模式切换键（i/Enter→Input、Space→Leader、:→Ex、
// /→Search）优先拦截；n/N 在搜索 Matches 非空时跳转；Ctrl+D 半页下翻；
// 其余导航键交给 stream。
func (a *App) handleNormalModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	action := keymap.MatchNormalKey(msg.String())
	switch action {
	case keymap.ActionCtrlC:
		return a.handleCtrlC()
	case keymap.ActionEnterInput:
		a.enterInputFromNormal()
		return a, nil
	case keymap.ActionScrollDown, keymap.ActionScrollUp,
		keymap.ActionScrollTop, keymap.ActionScrollBottom:
		_, cmd := a.agentStream.Update(msg)
		return a, cmd
	case keymap.ActionHalfPageDown, keymap.ActionHalfPageUp:
		_, cmd := a.agentStream.Update(msg)
		return a, cmd
	case keymap.ActionFullPageDown, keymap.ActionFullPageUp:
		_, cmd := a.agentStream.Update(msg)
		return a, cmd
	case keymap.ActionLeader:
		a.state.Mode = state.LeaderMode
		return a, leaderTimeoutCmd()
	case keymap.ActionQuit:
		return a, tea.Quit
	case keymap.ActionSearchForward:
		// / 进入搜索输入 overlay
		a.openSearch()
		return a, nil
	case keymap.ActionSearchNext:
		a.jumpSearchMatch(1)
		return a, nil
	case keymap.ActionSearchPrev:
		a.jumpSearchMatch(-1)
		return a, nil
	case keymap.ActionExCommand:
		// : 进入 Ex 命令行输入 overlay
		a.openEx()
		return a, nil
	default:
		_, promptCmd := a.commandPrompt.Update(msg)
		return a, promptCmd
	}
}

// handleLeaderKey 处理 Leader Key 后缀。
//
// 行为约定（plan-v4）：Leader 是独占捕获，非后缀键或超时(1s)时立即静默回到
// Normal（不泄漏给 Normal handler）。后缀键执行动作后回到 Normal，除非打开了
// 需要保持的面板（palette/session_picker/help/model_picker）。

// enterInputFromNormal 从 Normal 进入 Input Mode，并清除 Normal 专属子状态（搜索）。
func (a *App) enterInputFromNormal() {
	a.state.Mode = state.InputModeInput
	a.clearSearchAndEx()
}

// openEx 打开 : 命令行输入 overlay。

// openEx 打开 : 命令行输入 overlay。
func (a *App) openEx() {
	a.state.Ex.Active = true
	a.state.Ex.Input = ""
	a.openOverlay(state.OverlayEx)
}

// openSearch 打开 / 搜索输入 overlay。

// openSearch 打开 / 搜索输入 overlay。
func (a *App) openSearch() {
	a.state.Search.Active = true
	a.state.Search.Query = ""
	a.openOverlay(state.OverlaySearch)
}

// clearSearchAndEx 清除搜索与 Ex 输入状态（切出 Normal 或事件触发时调用）。

// clearSearchAndEx 清除搜索与 Ex 输入状态（切出 Normal 或事件触发时调用）。
func (a *App) clearSearchAndEx() {
	a.state.Search = state.SearchState{}
	a.state.Ex = state.ExState{}
}

// executeExCommand 解释并执行 : 命令（已去除前缀 ":"），返回副作用 cmd。
//
// 支持命令：q/quit/exit=退出、debug=切调试、help=开帮助、compact=触发压缩、
// mode=切换 Agent 模式。空或未知命令给出提示。

// executeExCommand 解释并执行 : 命令（已去除前缀 ":"），返回副作用 cmd。
//
// 支持命令：q/quit/exit=退出、debug=切调试、help=开帮助、compact=触发压缩、
// mode=切换 Agent 模式。空或未知命令给出提示。
func (a *App) executeExCommand(command string) tea.Cmd {
	switch command {
	case "q", "quit", "exit":
		return tea.Quit
	case "debug":
		a.debug = !a.debug
		a.appendStream(state.StreamEntry{
			ID:        fmt.Sprintf("debug-toggle-%d", time.Now().UnixNano()),
			Type:      "status",
			Timestamp: time.Now(),
			Content:   fmt.Sprintf("Debug: %v", a.debug),
			Metadata:  map[string]any{"done": true},
		})
		return nil
	case "help":
		a.openOverlay(state.OverlayHelp)
		return nil
	case "compact":
		return a.triggerCompact()
	case "mode":
		return a.toggleAgentMode()
	default:
		a.appendStream(state.StreamEntry{
			ID:        fmt.Sprintf("ex-unknown-%d", time.Now().UnixNano()),
			Type:      "status",
			Timestamp: time.Now(),
			Content:   fmt.Sprintf("Unknown ex command: %s", emptyDash(command)),
			Metadata:  map[string]any{"done": true},
		})
		return nil
	}
}

// executeSearch 执行全量扫描并记录匹配索引到 Search.Matches，滚动到首个匹配。
//
// 空 query 为 no-op（关闭搜索 overlay）；无匹配给出提示。

// executeSearch 执行全量扫描并记录匹配索引到 Search.Matches，滚动到首个匹配。
//
// 空 query 为 no-op（关闭搜索 overlay）；无匹配给出提示。
func (a *App) executeSearch(query string) tea.Cmd {
	if strings.TrimSpace(query) == "" {
		return nil
	}
	matches := components.RunSearch(a.state.Stream, query)
	a.state.Search.Matches = matches
	a.state.Search.MatchIndex = 0
	a.state.Search.Stale = false
	if len(matches) == 0 {
		a.appendStream(state.StreamEntry{
			ID:        fmt.Sprintf("search-empty-%d", time.Now().UnixNano()),
			Type:      "status",
			Timestamp: time.Now(),
			Content:   fmt.Sprintf("No matches: %s", query),
			Metadata:  map[string]any{"done": true},
		})
		return nil
	}
	a.scrollToStreamIndex(matches[0])
	return nil
}

// jumpSearchMatch 在搜索匹配间循环跳转（direction=1 下一个，-1 上一个）。
//
// 无匹配时静默 no-op；到末尾/首位循环折返。

// jumpSearchMatch 在搜索匹配间循环跳转（direction=1 下一个，-1 上一个）。
//
// 无匹配时静默 no-op；到末尾/首位循环折返。
func (a *App) jumpSearchMatch(direction int) {
	matches := a.state.Search.Matches
	if len(matches) == 0 {
		return
	}
	a.state.Search.MatchIndex = (a.state.Search.MatchIndex + direction + len(matches)) % len(matches)
	a.scrollToStreamIndex(matches[a.state.Search.MatchIndex])
}

// scrollToStreamIndex 滚动 stream 使指定全局 entry 索引尽量可见。
//
// 由于 state.Stream 是 append-only 且全量在内存，这里基于目标索引估算
// 滚动偏移（粗略：将目标定位到视口中部），足够满足跳转可见需求。

// scrollToStreamIndex 滚动 stream 使指定全局 entry 索引尽量可见。
//
// 由于 state.Stream 是 append-only 且全量在内存，这里基于目标索引估算
// 滚动偏移（粗略：将目标定位到视口中部），足够满足跳转可见需求。
func (a *App) scrollToStreamIndex(targetIndex int) {
	if targetIndex < 0 || targetIndex >= len(a.state.Stream) {
		return
	}
	// 粗略估计：stream 行数约为 entry 数的倍数，这里直接用 entry 索引作为
	// 偏移参考，关闭自动滚动并尝试把目标带到视口。精确视口定位由 stream
	// 渲染时的 visibleLines 兜底（超出范围会被 clamp）。
	a.state.Layout.AutoScroll = false
	// 反向估算：偏移越大表示越靠顶部。目标越靠后(索引大)越接近底部，偏移越小。
	estimated := len(a.state.Stream) - targetIndex
	if estimated < 0 {
		estimated = 0
	}
	a.state.Layout.ScrollOffset = estimated
}

// handlePaletteCommand 处理命令面板选择的命令。
