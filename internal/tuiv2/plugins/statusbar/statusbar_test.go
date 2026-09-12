package statusbar

import (
	"context"
	"strings"
	"testing"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// fakeHost 是 Host 接口的测试第二实现（ADR-008 预期声明的插件侧复用）。
var _ kernel.Host = (*fakeHost)(nil)

type fakeHost struct {
	st *state.ViewState
}

func newFakeHost() *fakeHost { return &fakeHost{st: state.NewViewState()} }

func (h *fakeHost) State() *state.ViewState                        { return h.st }
func (h *fakeHost) Gateway() gateway.Client                        { return nil }
func (h *fakeHost) Commands() []kernel.Command                     { return nil }
func (h *fakeHost) RunCommand(string, []string) error              { return nil }
func (h *fakeHost) Bindings() []kernel.Binding                     { return nil }
func (h *fakeHost) BindEventStream(ch <-chan gateway.GatewayEvent) {}
func (h *fakeHost) GoCmd(cmd tea.Cmd)                              {}
func (h *fakeHost) Send(msg tea.Msg)                               {}
func (h *fakeHost) Mode() state.InputMode                          { return h.st.Mode }
func (h *fakeHost) SetMode(m state.InputMode)                      {}
func (h *fakeHost) PushOverlay(o kernel.Overlay)                   {}
func (h *fakeHost) PopOverlay()                                    {}
func (h *fakeHost) Confirm(req state.ConfirmRequest)               {}
func (h *fakeHost) Notify(text string)                             {}
func (h *fakeHost) Quit()                                          {}

func TestPluginIdentity(t *testing.T) {
	p := New()
	if p.ID() != "statusbar" {
		t.Fatalf("id = %q", p.ID())
	}
	if p.Region() != kernel.RegionStatusBar {
		t.Fatalf("region = %q", p.Region())
	}
}

func TestInitFixesStateAndRenderer(t *testing.T) {
	p, h := New(), newFakeHost()
	p.Init(context.Background(), h)
	if p.st != h.st {
		t.Fatal("Init should fix the shared state pointer")
	}
	if p.status == nil {
		t.Fatal("Init should construct renderer")
	}
	p.Close(context.Background()) // 无外部资源，对称生命周期
}

func TestRenderReflectsState(t *testing.T) {
	p, h := New(), newFakeHost()
	p.Init(context.Background(), h)
	h.st.Runtime.Phase = state.RuntimePhaseRunning
	h.st.Gateway.ActiveModel = "test-model"
	out := p.Render(h, 100)
	if out == "" {
		t.Fatal("render should not be empty for status bar")
	}
	if !strings.Contains(out, "test-model") {
		t.Fatalf("render should reflect model, got %q", out)
	}
}

func TestRenderBeforeInitIsSafe(t *testing.T) {
	p := New()
	if out := p.Render(nil, 100); out != "" {
		t.Fatalf("render before init = %q", out)
	}
}
