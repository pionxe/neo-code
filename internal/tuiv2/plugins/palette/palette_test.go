package palette

import (
	"strings"
	"testing"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// recordingHost 是 fake Host：Commands 返回预置快照；PopOverlay 记录次数。
var _ kernel.Host = (*recordingHost)(nil)

type recordingHost struct {
	st       *state.ViewState
	cmds     []kernel.Command
	popCount int
	executed []string
	overlays []kernel.Overlay
}

func newRecordingHost() *recordingHost { return &recordingHost{st: state.NewViewState()} }

func (h *recordingHost) State() *state.ViewState                        { return h.st }
func (h *recordingHost) Gateway() gateway.Client                        { return nil }
func (h *recordingHost) BindEventStream(ch <-chan gateway.GatewayEvent) {}
func (h *recordingHost) GoCmd(cmd tea.Cmd)                              {}
func (h *recordingHost) Send(msg tea.Msg)                               {}
func (h *recordingHost) Mode() state.InputMode                          { return h.st.Mode }
func (h *recordingHost) SetMode(m state.InputMode)                      {}
func (h *recordingHost) PushOverlay(o kernel.Overlay)                   { h.overlays = append(h.overlays, o) }
func (h *recordingHost) PopOverlay()                                    { h.popCount++ }
func (h *recordingHost) Confirm(req state.ConfirmRequest)               {}
func (h *recordingHost) Notify(text string)                             {}
func (h *recordingHost) Quit()                                          {}
func (h *recordingHost) Commands() []kernel.Command                     { return h.cmds }
func (h *recordingHost) RunCommand(name string, args []string) error {
	h.executed = append(h.executed, name)
	return nil
}
func (h *recordingHost) Bindings() []kernel.Binding { return nil }

func TestPluginIdentityAndBindings(t *testing.T) {
	p := New()
	if p.ID() != "palette" {
		t.Fatalf("id = %q", p.ID())
	}
	bs := p.Bindings()
	if len(bs) != 1 || bs[0].Key != "p" || bs[0].Mode != state.LeaderMode {
		t.Fatalf("bindings = %+v", bs)
	}
}

func TestOpenSnapshotsCommandsAndFilter(t *testing.T) {
	p := New()
	h := newRecordingHost()
	h.cmds = []kernel.Command{
		{Name: "/new", Description: "新建会话"},
		{Name: "/model", Description: "切换模型"},
	}
	// 打开：经 Leader p 绑定。
	for _, b := range p.Bindings() {
		b.OnKey(h)
	}
	if len(h.overlays) != 1 {
		t.Fatal("open should push overlay")
	}
	o := h.overlays[0]
	// 过滤 "mo"：/model 前缀命中。
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("mo")})
	out := o.View(h, 60)
	if !strings.Contains(out, "/model") || strings.Contains(out, "/new  ") {
		t.Fatalf("filtered view = %q", out)
	}
	// backspace 回删恢复（经具体类型访问未导出的过滤方法——同包测试）。
	specific := o.(*overlay)
	specific.HandleKey(h, tea.KeyMsg{Type: tea.KeyBackspace})
	_ = specific.View(h, 60)
	specific.HandleKey(h, tea.KeyMsg{Type: tea.KeyBackspace})
	if len(specific.filtered()) != 2 {
		t.Fatalf("after clearing query, matched = %d", len(specific.filtered()))
	}
}

func TestOverlayExecuteRunsCommand(t *testing.T) {
	p := New()
	h := newRecordingHost()
	ran := false
	h.cmds = []kernel.Command{{Name: "/do", Description: "d", Run: func(h kernel.Host, args []string) { ran = true }}}
	p.open(h)
	o := h.overlays[0]
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyEnter})
	if !ran {
		t.Fatal("enter should execute selected command")
	}
	if h.popCount != 1 {
		t.Fatalf("popCount = %d", h.popCount)
	}
}

func TestOverlayEmptyQueryRendersNoMatchMessage(t *testing.T) {
	p := New()
	h := newRecordingHost()
	p.open(h)
	o := h.overlays[0]
	// 过滤到零结果。
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zz")})
	if !strings.Contains(o.View(h, 60), "无匹配命令") {
		t.Fatalf("view = %q", o.View(h, 60))
	}
}

func TestFilterDedup(t *testing.T) {
	// 空查询返回全部（引用透传亦可——只验证长度）。
	o := &overlay{cmds: []kernel.Command{{Name: "/a"}, {Name: "/b"}}}
	if len(o.filtered()) != 2 {
		t.Fatalf("filtered = %d", len(o.filtered()))
	}
}
