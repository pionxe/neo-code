package kernel

import (
	"context"
	"errors"
	"strings"
	"testing"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// testFakeHost 是 Host 接口的测试第二实现（ADR-008 预期声明）：
// 编译期断言证明 Host 可在内核之外被实现——这是 Host 接口合法性的依据。
var _ Host = (*testFakeHost)(nil)

// testFakeHost 是最小 fake：记录调用轨迹，不依赖真终端与 Gateway。
type testFakeHost struct {
	notifies []string
	quitted  bool
}

func (f *testFakeHost) State() *state.ViewState                        { return state.NewViewState() }
func (f *testFakeHost) Gateway() gateway.Client                        { return nil }
func (f *testFakeHost) BindEventStream(ch <-chan gateway.GatewayEvent) {}
func (f *testFakeHost) Commands() []Command                            { return nil }
func (f *testFakeHost) RunCommand(string, []string) error              { return nil }
func (f *testFakeHost) Bindings() []Binding                            { return nil }
func (f *testFakeHost) GoCmd(cmd tea.Cmd)                              {}
func (f *testFakeHost) Send(msg tea.Msg)                               {}
func (f *testFakeHost) Mode() state.InputMode                          { return state.NormalMode }
func (f *testFakeHost) SetMode(m state.InputMode)                      {}
func (f *testFakeHost) PushOverlay(o Overlay)                          {}
func (f *testFakeHost) PopOverlay()                                    {}
func (f *testFakeHost) Confirm(req state.ConfirmRequest)               { f.notifies = append(f.notifies, req.Title) }
func (f *testFakeHost) Notify(text string)                             { f.notifies = append(f.notifies, text) }
func (f *testFakeHost) Quit()                                          { f.quitted = true }

// keyRunes 构造一个 runes 按键消息（如 "y"、"abc"）。
func keyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// mustOK 断言无错误。
func mustOK(t *testing.T, err error, what string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: unexpected error: %v", what, err)
	}
}

func TestRegisterRejectsEmptyID(t *testing.T) {
	k := NewKernel(Config{})
	err := k.Register(barePlugin{id: ""})
	if err == nil || !strings.Contains(err.Error(), "must not be empty") {
		t.Fatalf("err = %v, want empty id error", err)
	}
}

func TestRegisterRejectsDuplicateID(t *testing.T) {
	k := NewKernel(Config{})
	mustOK(t, k.Register(barePlugin{id: "a"}), "first register")
	err := k.Register(barePlugin{id: "a"})
	if !errors.Is(err, ErrDuplicatePlugin) {
		t.Fatalf("err = %v, want ErrDuplicatePlugin", err)
	}
}

func TestRegisterRejectsDuplicateRegion(t *testing.T) {
	k := NewKernel(Config{})
	mustOK(t, k.Register(regionPlugin{id: "r1", region: RegionStatusBar}), "first")
	err := k.Register(regionPlugin{id: "r2", region: RegionStatusBar})
	if !errors.Is(err, ErrDuplicateRegion) {
		t.Fatalf("err = %v, want ErrDuplicateRegion", err)
	}
}

func TestRegisterRejectsBindingWithoutAction(t *testing.T) {
	k := NewKernel(Config{})
	err := k.Register(keyPlugin{id: "b", bindings: []Binding{{Mode: state.NormalMode, Key: "j"}}})
	if err == nil || !strings.Contains(err.Error(), "has no OnKey") {
		t.Fatalf("err = %v, want missing OnKey error", err)
	}
}

func TestCapabilityDiscovery(t *testing.T) {
	k := NewKernel(Config{})
	// 内核自身种子 1 条保留绑定（Normal 空格→Leader，issue #41）。
	seeded := k.bindings.count
	if seeded != 1 {
		t.Fatalf("seeded bindings = %d, want 1", seeded)
	}
	// barePlugin 只实现 Plugin 最小面：不得进入任何可选能力集合。
	mustOK(t, k.Register(barePlugin{id: "bare"}), "register bare")
	if len(k.reactors) != 0 || k.bindings.count != seeded || len(k.commands.list) != 0 || len(k.renderers) != 0 {
		t.Fatalf("bare plugin should not join any capability set")
	}
	// 实现了 Reactor 的插件进入广播集合。
	mustOK(t, k.Register(&recordingReactor{id: "reactive"}), "register reactor")
	if len(k.reactors) != 1 {
		t.Fatalf("reactors = %d, want 1", len(k.reactors))
	}
}

func TestInitRunsInOrderAndCloseReverses(t *testing.T) {
	var events []string
	p1 := lifecyclePlugin{id: "p1", log: func(s string) { events = append(events, s) }}
	p2 := lifecyclePlugin{id: "p2", log: func(s string) { events = append(events, s) }}
	k := NewKernel(Config{})
	mustOK(t, k.Register(&p1), "p1")
	mustOK(t, k.Register(&p2), "p2")

	k.Init()                      // Init 顺序：注册序
	k.Close(context.Background()) // Close 逆序：注册倒序

	want := "init:p1,init:p2,close:p2,close:p1"
	if got := strings.Join(events, ","); got != want {
		t.Fatalf("lifecycle = %q, want %q", got, want)
	}
}

// ---------- 满足契约的最小插件桩 ----------

// barePlugin 只实现 Plugin 最小面。
type barePlugin struct{ id string }

func (p barePlugin) ID() string                       { return p.id }
func (p barePlugin) Init(ctx context.Context, h Host) {}
func (p barePlugin) Close(ctx context.Context)        {}

// recordingReactor 记录收到的广播消息。
type recordingReactor struct {
	id   string
	seen []tea.Msg
}

func (p *recordingReactor) ID() string                       { return p.id }
func (p *recordingReactor) Init(ctx context.Context, h Host) {}
func (p *recordingReactor) Close(ctx context.Context)        {}
func (p *recordingReactor) React(h Host, msg tea.Msg)        { p.seen = append(p.seen, msg) }

// regionPlugin 只实现区域渲染（空渲染，用于所有权冲突测试）。
type regionPlugin struct {
	id     string
	region RegionID
}

func (p regionPlugin) ID() string                       { return p.id }
func (p regionPlugin) Init(ctx context.Context, h Host) {}
func (p regionPlugin) Close(ctx context.Context)        {}
func (p regionPlugin) Region() RegionID                 { return p.region }
func (p regionPlugin) Render(h Host, width int) string  { return "" }

// keyPlugin 只贡献键位绑定。
type keyPlugin struct {
	id       string
	bindings []Binding
}

func (p keyPlugin) ID() string                       { return p.id }
func (p keyPlugin) Init(ctx context.Context, h Host) {}
func (p keyPlugin) Close(ctx context.Context)        {}
func (p keyPlugin) Bindings() []Binding              { return p.bindings }

// lifecyclePlugin 记录生命周期调用轨迹。
type lifecyclePlugin struct {
	id  string
	log func(string)
}

func (p *lifecyclePlugin) ID() string                       { return p.id }
func (p *lifecyclePlugin) Init(ctx context.Context, h Host) { p.log("init:" + p.id) }
func (p *lifecyclePlugin) Close(ctx context.Context)        { p.log("close:" + p.id) }
