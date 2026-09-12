package debug

import (
	"context"
	"strings"
	"testing"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// fakeHost 记录 Notify 调用并提供可注入的状态指针（Render 需要真实状态）。
type fakeHost struct {
	st       *state.ViewState
	notifies []string
}

func (h *fakeHost) State() *state.ViewState                        { return h.st }
func (h *fakeHost) Gateway() gateway.Client                        { return nil }
func (h *fakeHost) GoCmd(cmd tea.Cmd)                              {}
func (h *fakeHost) Send(msg tea.Msg)                               {}
func (h *fakeHost) Mode() state.InputMode                          { return 0 }
func (h *fakeHost) SetMode(m state.InputMode)                      {}
func (h *fakeHost) PushOverlay(o kernel.Overlay)                   {}
func (h *fakeHost) PopOverlay()                                    {}
func (h *fakeHost) Confirm(req state.ConfirmRequest)               {}
func (h *fakeHost) Notify(text string)                             { h.notifies = append(h.notifies, text) }
func (h *fakeHost) Quit()                                          {}
func (h *fakeHost) Commands() []kernel.Command                     { return nil }
func (h *fakeHost) RunCommand(string, []string) error              { return nil }
func (h *fakeHost) Bindings() []kernel.Binding                     { return nil }
func (h *fakeHost) BindEventStream(ch <-chan gateway.GatewayEvent) {}

// TestPluginIdentityAndRegion 验证身份与区域认领（单区域契约：RegionDebug）。
func TestPluginIdentityAndRegion(t *testing.T) {
	p := New(false, "fake")
	if p.ID() != "debug" {
		t.Fatalf("id = %q", p.ID())
	}
	if p.Region() != kernel.RegionDebug {
		t.Fatalf("region = %v, want RegionDebug", p.Region())
	}
	// Close 无外部资源，调用不 panic 即可（生命周期对称性）。
	p.Close(context.Background())
}

// TestRenderNormalModeName 验证 Normal 模式的调试行短名渲染。
func TestRenderNormalModeName(t *testing.T) {
	st := state.NewViewState()
	st.Mode = state.NormalMode
	p := New(true, "fake")
	if got := p.Render(&fakeHost{st: st}, 80); !strings.Contains(got, "mode:normal") {
		t.Fatalf("render %q missing mode:normal", got)
	}
}

// TestRenderDisabledReturnsEmpty 验证关闭时渲染空串（compose 折叠该行）。
func TestRenderDisabledReturnsEmpty(t *testing.T) {
	p := New(false, "fake")
	if got := p.Render(&fakeHost{st: state.NewViewState()}, 80); got != "" {
		t.Fatalf("disabled render = %q, want empty", got)
	}
}

// TestRenderEnabledFields 验证开启时字段对齐旧 debugLine：
// mode=按键模式/scenario/events/size；尺寸全零时缺省 "0x0"。
func TestRenderEnabledFields(t *testing.T) {
	st := state.NewViewState()
	st.Mode = state.LeaderMode
	st.Layout.Width = 100
	st.Layout.Height = 30
	st.Stream = []state.StreamEntry{{}, {}, {}}
	p := New(true, "streaming_chat")
	got := p.Render(&fakeHost{st: st}, 80)
	for _, want := range []string{
		"mode:leader", "scenario:streaming_chat", "events:3", "size:100x30",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("render %q missing %q", got, want)
		}
	}
	// 尺寸全零 → 缺省 "0x0"（对齐旧 defaultTerminal 常量语义）。
	st.Layout.Width, st.Layout.Height = 0, 0
	if got := p.Render(&fakeHost{st: st}, 80); !strings.Contains(got, "size:0x0") {
		t.Fatalf("render %q missing size:0x0", got)
	}
}

// TestRenderNilStateSafe 验证 Init 前状态未固定的防御路径。
func TestRenderNilStateSafe(t *testing.T) {
	p := New(true, "fake")
	if got := p.Render(&fakeHost{}, 80); got != "" {
		t.Fatalf("nil-state render = %q, want empty", got)
	}
}

// TestDebugCommandTogglesAndNotifies 验证 /debug 命令：切换显隐 +
// 弱提示反馈（文案对齐旧 toggleDebug "Debug: %v"）+ 渲染随之变化。
func TestDebugCommandTogglesAndNotifies(t *testing.T) {
	p, h := New(false, "fake"), &fakeHost{st: state.NewViewState()}
	p.Init(context.Background(), h)
	cmds := p.Commands()
	if len(cmds) != 1 || cmds[0].Name != "/debug" {
		t.Fatalf("commands = %v", cmds)
	}
	// 别名完备：无斜杠 "debug" 通 :debug（issue #41 审计 P2-4）。
	if len(cmds[0].Aliases) != 1 || cmds[0].Aliases[0] != "debug" {
		t.Fatalf("aliases = %v, want [debug]", cmds[0].Aliases)
	}
	if got := p.Render(h, 80); got != "" {
		t.Fatalf("initial render = %q, want empty", got)
	}
	cmds[0].Run(h, nil)
	if len(h.notifies) != 1 || h.notifies[0] != "Debug: true" {
		t.Fatalf("notifies = %v", h.notifies)
	}
	if got := p.Render(h, 80); !strings.Contains(got, "[debug]") {
		t.Fatalf("render after enable = %q", got)
	}
	cmds[0].Run(h, nil)
	if len(h.notifies) != 2 || h.notifies[1] != "Debug: false" {
		t.Fatalf("notifies = %v", h.notifies)
	}
	if got := p.Render(h, 80); got != "" {
		t.Fatalf("render after disable = %q, want empty", got)
	}
}
