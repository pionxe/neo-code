package theme

import (
	"context"
	"strings"
	"testing"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"
	themedep "neo-code/internal/tuiv2/theme"

	tea "github.com/charmbracelet/bubbletea"
)

// fakeHost 记录 Notify 调用。
type fakeHost struct {
	notifies []string
}

func (h *fakeHost) State() *state.ViewState                        { return nil }
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

func TestPluginIdentityAndCommands(t *testing.T) {
	p := New()
	if p.ID() != "theme" {
		t.Fatalf("id = %q", p.ID())
	}
	cmds := p.Commands()
	if len(cmds) != 2 {
		t.Fatalf("commands = %d, want 2", len(cmds))
	}
	if cmds[0].Name != "/theme tokyo-night" || cmds[1].Name != "/theme tokyo-night-ascii" {
		t.Fatalf("command names = %v", cmds)
	}
}

func TestThemeCommandsSwitchPaletteAndSymbols(t *testing.T) {
	p, h := New(), &fakeHost{}
	p.Init(context.Background(), h)
	byName := map[string]kernel.Command{}
	for _, c := range p.Commands() {
		byName[c.Name] = c
	}
	// ASCII：符号集偏好切换 + 通知（Success 符号 [OK] 为 ASCII 形态标志）。
	byName["/theme tokyo-night-ascii"].Run(h, nil)
	if got := themedep.Symbols().Success; got != "[OK]" {
		t.Fatalf("success symbol = %q, want ASCII form", got)
	}
	if len(h.notifies) == 0 || !strings.Contains(h.notifies[0], "Tokyo Night ASCII") {
		t.Fatalf("notifies = %v", h.notifies)
	}
	// Unicode：恢复 Unicode 符号（Success ✓）。
	byName["/theme tokyo-night"].Run(h, nil)
	if got := themedep.Symbols().Success; got == "[OK]" {
		t.Fatalf("success symbol = %q, want unicode form", got)
	}
	if !strings.Contains(h.notifies[len(h.notifies)-1], "Tokyo Night") {
		t.Fatalf("notifies = %v", h.notifies)
	}
	p.Close(context.Background()) // 对称生命周期
}
