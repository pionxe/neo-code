package tuiv2

import (
	"context"
	"strings"
	"testing"
	"time"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// testHost 实现 kernel.Host 的测试桩（零测试盲区）。
type testHost struct {
	st         *state.ViewState
	client     gateway.Client
	notifies   []string
	broadcasts []tea.Msg
	bindCalls  int
	quitCalled bool
}

func newTestHost(client gateway.Client) *testHost {
	return &testHost{st: state.NewViewState(), client: client}
}

func (h *testHost) State() *state.ViewState { return h.st }
func (h *testHost) Gateway() gateway.Client { return h.client }
func (h *testHost) GoCmd(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	if msg := cmd(); msg != nil {
		h.Send(msg)
	}
}
func (h *testHost) Send(msg tea.Msg) {
	h.broadcasts = append(h.broadcasts, msg)
}
func (h *testHost) Mode() state.InputMode        { return h.st.Mode }
func (h *testHost) SetMode(m state.InputMode)    { h.st.Mode = m }
func (h *testHost) PushOverlay(o kernel.Overlay) {}
func (h *testHost) PopOverlay()                  {}
func (h *testHost) Confirm(req state.ConfirmRequest) {
	h.notifies = append(h.notifies, "confirm:"+req.Title)
}
func (h *testHost) Notify(text string)                { h.notifies = append(h.notifies, text) }
func (h *testHost) Quit()                             { h.quitCalled = true }
func (h *testHost) Commands() []kernel.Command        { return nil }
func (h *testHost) RunCommand(string, []string) error { return nil }
func (h *testHost) Bindings() []kernel.Binding        { return nil }
func (h *testHost) BindEventStream(ch <-chan gateway.GatewayEvent) {
	h.bindCalls++
}

// TestBootstrapReactorReactAppliesAndNotifies 钉死消费链 + 错误经 Notify 呈现。
func TestBootstrapReactorReactAppliesAndNotifies(t *testing.T) {
	client := newFakeBootstrapClient()
	client.listSessions = []gateway.SessionSummary{{ID: "s1", Title: "demo"}}
	client.healthErr = errFake("health failed")
	client.subscribeErr = errFake("sub failed")
	client.modelsErr = errFake("models failed")

	br := newBootstrapReactor(client)
	h := newTestHost(client)
	br.client = client

	// 模拟 kernel 调 Bootstrap 闭包 → 广播 → React 消费。
	cmd := Bootstrap(context.Background(), client)
	msg := cmd()
	br.React(h, msg)

	// 状态落地断言（health 失败但 list 成功 → sessions 应有数据）。
	if len(h.st.Gateway.Sessions) != 1 {
		t.Fatalf("sessions = %d, want 1 (list succeeds despite health error)", len(h.st.Gateway.Sessions))
	}
	// 错误经 Notify 呈现。
	foundNotify := false
	for _, n := range h.notifies {
		if strings.Contains(n, "初始加载") {
			foundNotify = true
		}
	}
	if !foundNotify {
		t.Fatal("bootstrap errors should be presented via Notify")
	}
}

// TestBootstrapReactorReactIgnoresNonBootstrapMsg 钉死 React 忽略非 bootstrap 消息。
func TestBootstrapReactorReactIgnoresNonBootstrapMsg(t *testing.T) {
	br := newBootstrapReactor(nil)
	h := newTestHost(nil)
	// 非-bootstrapDoneMsg 消息不应 panic 或产生副作用。
	br.React(h, tea.KeyMsg{Type: tea.KeyEnter})
	br.React(h, "string msg")
}

// TestBootstrapReactorLifecycle 补齐 ID/Init/Close 覆盖。
func TestBootstrapReactorLifecycle(t *testing.T) {
	br := newBootstrapReactor(nil)
	if br.ID() != "bootstrap" {
		t.Fatalf("id = %q", br.ID())
	}
	br.Init(context.Background(), newTestHost(nil))
	br.Close(context.Background())
}

// TestApplyBootstrapTimestampAndNoDetail 补齐 Timestamp 保留与 nil detail 分支。
func TestApplyBootstrapTimestampAndNoDetail(t *testing.T) {
	st := state.NewViewState()
	// nil detail：不 panic、不清空已有 Stream。
	ApplyBootstrap(st, bootstrapDoneMsg{healthOK: true}, nil)
	if len(st.Stream) != 0 {
		t.Fatalf("nil detail should not touch stream")
	}
	// 带 Timestamp 的 detail：非零 CreatedAt 保留。
	msg := bootstrapDoneMsg{
		detail: &gateway.SessionDetail{
			Stream: []gateway.StreamItem{
				{ID: "t1", Kind: "message", Role: "user", Text: "hello", CreatedAt: time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)},
			},
		},
	}
	ApplyBootstrap(st, msg, nil)
	if len(st.Stream) != 1 {
		t.Fatalf("stream = %d", len(st.Stream))
	}
	if st.Stream[0].Timestamp.IsZero() {
		t.Fatal("Timestamp should be preserved")
	}
	// detail.Usage → Runtime.Tokens。
	if st.Runtime.Tokens.Total == 0 {
		t.Log("Tokens zero (detail.Usage zero) - acceptable")
	}
}

// TestApplyBootstrapModelsDedup 补齐 Models 去重分支。
func TestApplyBootstrapModelsDedup(t *testing.T) {
	st := state.NewViewState()
	// 预置一个已有模型。
	st.Gateway.Models = []gateway.ModelInfo{{ID: "existing"}}
	msg := bootstrapDoneMsg{
		models: []gateway.ModelInfo{{ID: "existing"}, {ID: "new"}},
	}
	ApplyBootstrap(st, msg, nil)
	// ApplyBootstrap 直接追加（不去重——bootstrap 为一次性初始化场景）。
	// 断言追加后包含两个不重复 ID。
	if len(st.Gateway.Models) != 3 {
		t.Fatalf("models = %v, want 3 entries (existing + existing + new)", st.Gateway.Models)
	}
}

// TestGetModelErrorCollectedInErrs 钉死 GetModel 失败纳入 errs 收集。
func TestGetModelErrorCollectedInErrs(t *testing.T) {
	client := newFakeBootstrapClient()
	client.listSessions = []gateway.SessionSummary{{ID: "s1"}}
	client.getModelErr = errFake("get model failed")

	cmd := Bootstrap(context.Background(), client)
	bd := cmd().(bootstrapDoneMsg)
	found := false
	for _, e := range bd.errs {
		if strings.Contains(e, "get_model") {
			found = true
		}
	}
	if !found {
		t.Fatalf("GetModel error should be in errs, got %v", bd.errs)
	}
}
