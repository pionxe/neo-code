package health

import (
	"context"
	"errors"
	"testing"
	"time"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// mockClient 是 gateway.Client 的测试实现：只实现 Health（其余 panic 防
// 意外调用），Health 错误可注入并记录调用。
type mockClient struct {
	healthErr error
	calls     int
}

func (m *mockClient) Health(ctx context.Context) (*gateway.HealthResult, error) {
	m.calls++
	return &gateway.HealthResult{OK: m.healthErr == nil}, m.healthErr
}

func (m *mockClient) ListSessions(ctx context.Context) ([]gateway.SessionSummary, error) {
	panic("unexpected")
}

func (m *mockClient) LoadSession(ctx context.Context, id string) (*gateway.SessionDetail, error) {
	panic("unexpected")
}

func (m *mockClient) CreateSession(ctx context.Context) (*gateway.SessionSummary, error) {
	panic("unexpected")
}

func (m *mockClient) SendMessage(ctx context.Context, sessionID string, text string) (*gateway.RunAck, error) {
	panic("unexpected")
}

func (m *mockClient) CancelRun(ctx context.Context, sessionID string, runID string) error {
	panic("unexpected")
}

func (m *mockClient) SubscribeEvents(ctx context.Context, sessionID string) (<-chan gateway.GatewayEvent, error) {
	panic("unexpected")
}

func (m *mockClient) ResolvePermission(ctx context.Context, decision gateway.PermissionDecision) error {
	panic("unexpected")
}

func (m *mockClient) AnswerUserQuestion(ctx context.Context, answer gateway.UserQuestionAnswer) error {
	panic("unexpected")
}

func (m *mockClient) ListModels(ctx context.Context) ([]gateway.ModelInfo, error) {
	panic("unexpected")
}

func (m *mockClient) SetModel(ctx context.Context, sessionID string, modelID string) error {
	panic("unexpected")
}

func (m *mockClient) GetModel(ctx context.Context, sessionID string) (string, error) {
	panic("unexpected")
}

func (m *mockClient) Close() error { return nil }

// recordingHost 是 kernel.Host 的测试实现：同步执行 GoCmd 闭包并回流
// 消息（模拟"命令执行 → 消息回流 Update → 广播"链路）。
type recordingHost struct {
	st         *state.ViewState
	client     gateway.Client
	notifies   []string
	broadcasts []tea.Msg
	goCmds     int
	deferred   []tea.Cmd // 延迟执行的命令（deferMode=true 时存而不跑）
	deferMode  bool
}

func (h *recordingHost) State() *state.ViewState { return h.st }
func (h *recordingHost) Gateway() gateway.Client { return h.client }

// GoCmd 计数；deferMode 下仅存储命令（供测试手动延迟执行），
// 否则同步执行并把回流消息追加 broadcast（模拟内核链路）。
func (h *recordingHost) GoCmd(cmd tea.Cmd) {
	h.goCmds++
	if h.deferMode {
		h.deferred = append(h.deferred, cmd)
		return
	}
	if msg := cmd(); msg != nil {
		h.broadcasts = append(h.broadcasts, msg)
	}
}
func (h *recordingHost) Send(msg tea.Msg)                               { h.broadcasts = append(h.broadcasts, msg) }
func (h *recordingHost) Mode() state.InputMode                          { return 0 }
func (h *recordingHost) SetMode(m state.InputMode)                      {}
func (h *recordingHost) PushOverlay(o kernel.Overlay)                   {}
func (h *recordingHost) PopOverlay()                                    {}
func (h *recordingHost) Confirm(req state.ConfirmRequest)               {}
func (h *recordingHost) Notify(text string)                             { h.notifies = append(h.notifies, text) }
func (h *recordingHost) Quit()                                          {}
func (h *recordingHost) Commands() []kernel.Command                     { return nil }
func (h *recordingHost) RunCommand(string, []string) error              { return nil }
func (h *recordingHost) Bindings() []kernel.Binding                     { return nil }
func (h *recordingHost) BindEventStream(ch <-chan gateway.GatewayEvent) {}

// testConfig 是测试用微缩节奏（真实默认 30s/1s/60s/5s 会让同步
// GoCmd 的 tick 闭包睡眠真实时长——测试不必等待真实节奏）。
func testConfig() Config {
	return Config{
		HealthyInterval: 10 * time.Millisecond,
		InitialBackoff:  10 * time.Millisecond,
		MaxBackoff:      50 * time.Millisecond,
		ProbeTimeout:    5 * time.Millisecond,
	}
}

// TestPluginIdentity 验证身份与健康判据（err!=nil——HealthResult.OK 是
// 死词表，S6 审计 P2-1）。
func TestPluginIdentity(t *testing.T) {
	p := New(DefaultConfig())
	if p.ID() != "health" {
		t.Fatalf("id = %q", p.ID())
	}
}

// TestProbeWritesConnectedAndEdgeNotify 验证探针结果双写与边沿触发：
// 失败→Connected=false+断开 Notify；恢复→Connected=true+恢复 Notify+
// GatewayRecovered 广播；持续健康无重复广播。
func TestProbeWritesConnectedAndEdgeNotify(t *testing.T) {
	client := &mockClient{}
	p, h := New(testConfig()), &recordingHost{st: state.NewViewState(), client: client}
	p.Init(context.Background(), h)

	// 首轮探针失败（GoCmd 同步执行 → probeResultMsg 回流 React）。
	client.healthErr = errors.New("connection refused")
	_, healthErr := client.Health(context.Background())
	p.React(h, probeResultMsg{err: healthErr})
	if p.st.Gateway.Connected {
		t.Fatal("failed probe should write Connected=false")
	}
	if len(h.notifies) != 1 || h.notifies[0] != "网关连接断开：connection refused" {
		t.Fatalf("notifies = %v", h.notifies)
	}
	// 失败不广播恢复（计数只看 GatewayRecovered 类型——broadcasts 里
	// 还有 Init 同步探针的 probeResultMsg 与续订 tick 的 probeTickMsg）。
	if recoveryCount(h) != 0 {
		t.Fatalf("failure must not broadcast recovery, got %d", recoveryCount(h))
	}

	// 探针恢复：Connected=true + 恢复 Notify + GatewayRecovered 广播。
	client.healthErr = nil
	_, healthErr = client.Health(context.Background())
	p.React(h, probeResultMsg{err: healthErr})
	if !p.st.Gateway.Connected {
		t.Fatal("recovered probe should write Connected=true")
	}
	if len(h.notifies) != 2 || h.notifies[1] != "网关连接已恢复" {
		t.Fatalf("notifies = %v", h.notifies)
	}
	if recoveryCount(h) != 1 {
		t.Fatalf("recovery must broadcast once, got %d", recoveryCount(h))
	}

	// 持续健康：无重复广播（边沿触发）。
	p.React(h, probeResultMsg{})
	if recoveryCount(h) != 1 {
		t.Fatalf("steady-healthy must not re-broadcast, got %d", recoveryCount(h))
	}
}

// TestHealthChangedAndOfflineEvents 验证 B 组事件直写 Connected 槽
// （health_changed 自 sessions 移交；gateway_offline 直写）。
func TestHealthChangedAndOfflineEvents(t *testing.T) {
	p, h := New(testConfig()), &recordingHost{st: state.NewViewState()}
	p.Init(context.Background(), h)

	p.React(h, gateway.GatewayEvent{
		Type:    gateway.EventHealthChanged,
		Payload: map[string]any{"connected": true},
	})
	if !p.st.Gateway.Connected {
		t.Fatal("health_changed should write Connected=true")
	}
	p.React(h, gateway.GatewayEvent{
		Type:    gateway.EventGatewayOffline,
		Payload: map[string]any{"message": "gateway offline"},
	})
	if p.st.Gateway.Connected {
		t.Fatal("gateway_offline should write Connected=false")
	}
}

// TestCloseStopsRescheduling 验证 Close 停止位：置位后 probeResult 不再
// 续订（GoCmd 不再被调用——循环静默死亡防线反向断言）。
func TestCloseStopsRescheduling(t *testing.T) {
	client := &mockClient{}
	p, h := New(testConfig()), &recordingHost{st: state.NewViewState(), client: client}
	p.Init(context.Background(), h)
	p.Close(context.Background())

	before := h.goCmds
	p.React(h, probeResultMsg{err: errors.New("late result")})
	if h.goCmds != before {
		t.Fatal("closed plugin must not reschedule")
	}
}

// TestNextIntervalBackoffSequence 验证退避序列纯函数：
// 健康=固定档；失败 1→2→4→…→Max 封顶（S6 审计 P2-3 具名常量）。
func TestNextIntervalBackoffSequence(t *testing.T) {
	cfg := DefaultConfig()
	if got := nextInterval(cfg, 1); got != cfg.HealthyInterval {
		t.Fatalf("healthy interval = %v", got)
	}
	want := []time.Duration{
		cfg.InitialBackoff,      // attempt=2
		2 * cfg.InitialBackoff,  // attempt=3
		4 * cfg.InitialBackoff,  // attempt=4
		8 * cfg.InitialBackoff,  // attempt=5
		16 * cfg.InitialBackoff, // attempt=6
		32 * cfg.InitialBackoff, // attempt=7
		cfg.MaxBackoff,          // attempt=8（64s ≥ 60s 封顶）
		cfg.MaxBackoff,          // attempt=9
	}
	for i, attempt := range []int{2, 3, 4, 5, 6, 7, 8, 9} {
		if got := nextInterval(cfg, attempt); got != want[i] {
			t.Fatalf("attempt=%d interval = %v, want %v", attempt, got, want[i])
		}
	}
}

// recoveryCount 统计广播中的 GatewayRecovered 数量（边沿断言专用——
// broadcasts 另含探针结果回流与续订 tick，类型过滤后才可断言）。
func recoveryCount(h *recordingHost) int {
	count := 0
	for _, b := range h.broadcasts {
		if _, ok := b.(state.GatewayRecovered); ok {
			count++
		}
	}
	return count
}

// TestInitNilClientGuard 验证 client==nil 守卫：无后端环境不启动探针
// （prompt 先例；Init 后不发探针——goCmds 零增长）。
func TestInitNilClientGuard(t *testing.T) {
	p, h := New(testConfig()), &recordingHost{st: state.NewViewState(), client: nil}
	p.Init(context.Background(), h)
	p.React(h, probeTickMsg{})
	if h.goCmds != 0 {
		t.Fatalf("nil client must not start probe, goCmds = %d", h.goCmds)
	}
}

// TestProbeTickTriggersProbe 验证 probeTickMsg 续订路径：tick → 发起探针
// （GoCmd 执行 → 结果回流 broadcast）。
func TestProbeTickTriggersProbe(t *testing.T) {
	client := &mockClient{}
	p, h := New(testConfig()), &recordingHost{st: state.NewViewState(), client: client}
	p.Init(context.Background(), h)
	p.React(h, probeTickMsg{})
	if client.calls == 0 {
		t.Fatal("tick should trigger a probe")
	}
}

// TestStopBitBlocksTickAndProbe 验证停止位三处拦截：scheduleNext 不发
// GoCmd；tick 闭包回流 nil；探针闭包不发请求（S5 审计 P1-5 收口）。
func TestStopBitBlocksTickAndProbe(t *testing.T) {
	client := &mockClient{}
	p, h := New(testConfig()), &recordingHost{st: state.NewViewState(), client: client}
	p.Init(context.Background(), h)
	p.Close(context.Background())

	before := client.calls
	p.scheduleNext(h, time.Millisecond)
	time.Sleep(30 * time.Millisecond) // 等 tick 闭包（若未被停止位拦截会回流）
	if client.calls != before {
		t.Fatal("stopped probe must not run")
	}
	p.React(h, probeTickMsg{})
	if client.calls != before {
		t.Fatal("stopped tick must not trigger probe")
	}
}

// TestHealthyProbeWritesConnectedTrue 验证健康探针写 Connected=true
// （首轮成功即写——S6 审计 P2-2：离线启动恒 false，首轮探针成功须写 true）。
func TestHealthyProbeWritesConnectedTrue(t *testing.T) {
	client := &mockClient{}
	p, h := New(testConfig()), &recordingHost{st: state.NewViewState(), client: client}
	p.Init(context.Background(), h)
	p.React(h, probeResultMsg{})
	if !p.st.Gateway.Connected {
		t.Fatal("first healthy probe should write Connected=true")
	}
}

// TestStopBitInsideTickClosure 覆盖 scheduleNext 闭包内的两处防御性
// 停止位（S5 审计 P1-5 双保险层）：调度后、执行中 Close，闭包内的
// 停止位拦截回流（时序窗口测试——多次迭代覆盖前后两个检查点）。
func TestStopBitInsideTickClosure(t *testing.T) {
	for i := 0; i < 5; i++ {
		client := &mockClient{}
		p, h := New(testConfig()), &recordingHost{st: state.NewViewState(), client: client}
		p.Init(context.Background(), h)
		p.scheduleNext(h, 5*time.Millisecond)
		p.Close(context.Background()) // 与闭包执行竞态：任一停止位拦截均合法
		time.Sleep(30 * time.Millisecond)
		p.mu.Lock()
		stopped := p.stopped
		p.mu.Unlock()
		if !stopped {
			t.Fatal("plugin should be stopped")
		}
	}
}

// TestScheduleNextClosureStopped 验证 scheduleNext 闭包（138 行）的停止位
// 拦截：deferMode 存闭包 → Close → 手动执行 → 安全返回 nil（不续订）。
func TestScheduleNextClosureStopped(t *testing.T) {
	p, h := New(testConfig()), &recordingHost{st: state.NewViewState(), client: nil}
	p.Init(context.Background(), h)
	h.deferMode = true
	p.scheduleNext(h, time.Millisecond)
	p.Close(context.Background())
	if len(h.deferred) == 0 {
		t.Fatal("closure should be deferred in deferMode")
	}
	for _, cmd := range h.deferred {
		if msg := cmd(); msg != nil {
			t.Fatalf("closed plugin tick must return nil, got %v", msg)
		}
	}
}
