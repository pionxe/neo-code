package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type mockRuntimePort struct {
	mu sync.Mutex

	events chan RuntimeEvent

	runCalls int
	runInput RunInput
	runErr   error

	compactResult CompactResult
	compactErr    error

	cancelResult bool

	listResult []SessionSummary
	listErr    error

	loadResult Session
	loadErr    error

	setResult Session
	setErr    error
}

func newMockRuntimePort() *mockRuntimePort {
	return &mockRuntimePort{
		events: make(chan RuntimeEvent, 16),
	}
}

func (m *mockRuntimePort) Run(_ context.Context, input RunInput) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runCalls++
	m.runInput = input
	return m.runErr
}

func (m *mockRuntimePort) Compact(_ context.Context, _ CompactInput) (CompactResult, error) {
	return m.compactResult, m.compactErr
}

func (m *mockRuntimePort) CancelActiveRun() bool {
	return m.cancelResult
}

func (m *mockRuntimePort) Events() <-chan RuntimeEvent {
	return m.events
}

func (m *mockRuntimePort) ListSessions(_ context.Context) ([]SessionSummary, error) {
	return m.listResult, m.listErr
}

func (m *mockRuntimePort) LoadSession(_ context.Context, _ string) (Session, error) {
	return m.loadResult, m.loadErr
}

func (m *mockRuntimePort) SetSessionWorkdir(_ context.Context, _, _ string) (Session, error) {
	return m.setResult, m.setErr
}

func (m *mockRuntimePort) runCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runCalls
}

type runningGatewayServer struct {
	server   *Server
	baseURL  string
	wsURL    string
	cancel   context.CancelFunc
	errCh    chan error
	closed   bool
	closeMux sync.Mutex
}

func startGatewayServer(t *testing.T, runtimePort RuntimePort) *runningGatewayServer {
	t.Helper()

	gateway, ok := NewServer(ServerConfig{Address: "127.0.0.1:0"}).(*Server)
	if !ok {
		t.Fatalf("NewServer should return *Server")
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- gateway.Serve(ctx, runtimePort)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if addr := gateway.Address(); addr != "" {
			return &runningGatewayServer{
				server:  gateway,
				baseURL: "http://" + addr,
				wsURL:   "ws://" + addr + "/v1/gateway/ws",
				cancel:  cancel,
				errCh:   errCh,
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	t.Fatalf("gateway address was not ready within timeout")
	return nil
}

func (r *runningGatewayServer) close(t *testing.T) {
	t.Helper()

	r.closeMux.Lock()
	if r.closed {
		r.closeMux.Unlock()
		return
	}
	r.closed = true
	r.closeMux.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := r.server.Close(ctx); err != nil {
		t.Fatalf("close gateway failed: %v", err)
	}
	r.cancel()

	select {
	case err := <-r.errCh:
		if err != nil {
			t.Fatalf("serve returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("gateway serve did not exit in time")
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, check func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("condition not satisfied: %s", message)
}

func decodeHTTPFrame(t *testing.T, resp *http.Response) MessageFrame {
	t.Helper()
	defer resp.Body.Close()

	var frame MessageFrame
	if err := json.NewDecoder(resp.Body).Decode(&frame); err != nil {
		t.Fatalf("decode frame failed: %v", err)
	}
	return frame
}

func readWSFrame(t *testing.T, conn *websocket.Conn) MessageFrame {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	var frame MessageFrame
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatalf("read websocket frame failed: %v", err)
	}
	return frame
}

func dialWebSocket(t *testing.T, wsURL string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket failed: %v", err)
	}
	return conn
}

func TestServerHealthz(t *testing.T) {
	runner := startGatewayServer(t, nil)
	defer runner.close(t)

	resp, err := http.Get(runner.baseURL + "/healthz")
	if err != nil {
		t.Fatalf("healthz request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", resp.StatusCode, http.StatusOK)
	}

	var payload map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode healthz response failed: %v", err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("unexpected healthz payload: %#v", payload)
	}
}

func TestHTTPFrameInvalidJSON(t *testing.T) {
	runner := startGatewayServer(t, newMockRuntimePort())
	defer runner.close(t)

	resp, err := http.Post(runner.baseURL+"/v1/gateway/frame", "application/json", bytes.NewBufferString("{"))
	if err != nil {
		t.Fatalf("post frame failed: %v", err)
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected status: got %d want %d", resp.StatusCode, http.StatusBadRequest)
	}

	frame := decodeHTTPFrame(t, resp)
	if frame.Type != FrameTypeError {
		t.Fatalf("unexpected frame type: got %q want %q", frame.Type, FrameTypeError)
	}
	if frame.Error == nil || frame.Error.Code != ErrorCodeInvalidFrame.String() {
		t.Fatalf("unexpected error frame: %#v", frame.Error)
	}
}

func TestHTTPRunRequestReturnsAck(t *testing.T) {
	mockRuntime := newMockRuntimePort()
	runner := startGatewayServer(t, mockRuntime)
	defer runner.close(t)

	requestBody, err := json.Marshal(MessageFrame{
		Type:      FrameTypeRequest,
		Action:    FrameActionRun,
		RequestID: "req_001",
		RunID:     "run_001",
		SessionID: "sess_001",
		InputText: "hello",
	})
	if err != nil {
		t.Fatalf("marshal request failed: %v", err)
	}

	resp, err := http.Post(runner.baseURL+"/v1/gateway/frame", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("post frame failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", resp.StatusCode, http.StatusOK)
	}

	frame := decodeHTTPFrame(t, resp)
	if frame.Type != FrameTypeAck {
		t.Fatalf("unexpected frame type: got %q want %q", frame.Type, FrameTypeAck)
	}
	if frame.Action != FrameActionRun {
		t.Fatalf("unexpected ack action: got %q want %q", frame.Action, FrameActionRun)
	}
	if frame.RequestID != "req_001" {
		t.Fatalf("unexpected ack request_id: got %q", frame.RequestID)
	}

	waitForCondition(t, 2*time.Second, func() bool {
		return mockRuntime.runCallCount() == 1
	}, "run should be forwarded to runtime")
}

func TestHTTPRunRequestRuntimeUnavailable(t *testing.T) {
	runner := startGatewayServer(t, nil)
	defer runner.close(t)

	requestBody, err := json.Marshal(MessageFrame{
		Type:      FrameTypeRequest,
		Action:    FrameActionRun,
		RequestID: "req_001",
		InputText: "hello",
	})
	if err != nil {
		t.Fatalf("marshal request failed: %v", err)
	}

	resp, err := http.Post(runner.baseURL+"/v1/gateway/frame", "application/json", bytes.NewReader(requestBody))
	if err != nil {
		t.Fatalf("post frame failed: %v", err)
	}

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status: got %d want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}

	frame := decodeHTTPFrame(t, resp)
	if frame.Type != FrameTypeError {
		t.Fatalf("unexpected frame type: got %q want %q", frame.Type, FrameTypeError)
	}
	if frame.Error == nil || frame.Error.Code != ErrorCodeRuntimeUnavailable.String() {
		t.Fatalf("unexpected error frame: %#v", frame.Error)
	}
}

func TestWebSocketRunRequestReturnsAck(t *testing.T) {
	mockRuntime := newMockRuntimePort()
	runner := startGatewayServer(t, mockRuntime)
	defer runner.close(t)

	conn := dialWebSocket(t, runner.wsURL)
	defer conn.Close()

	request := MessageFrame{
		Type:      FrameTypeRequest,
		Action:    FrameActionRun,
		RequestID: "req_ws_01",
		RunID:     "run_ws_01",
		SessionID: "sess_ws_01",
		InputText: "hello",
	}
	if err := conn.WriteJSON(request); err != nil {
		t.Fatalf("write websocket frame failed: %v", err)
	}

	ack := readWSFrame(t, conn)
	if ack.Type != FrameTypeAck {
		t.Fatalf("unexpected websocket ack type: got %q want %q", ack.Type, FrameTypeAck)
	}
	if ack.Action != FrameActionRun {
		t.Fatalf("unexpected websocket ack action: got %q", ack.Action)
	}

	waitForCondition(t, 2*time.Second, func() bool {
		return mockRuntime.runCallCount() == 1
	}, "run should be forwarded through websocket path")
}

func TestWebSocketInvalidFrameReturnsError(t *testing.T) {
	runner := startGatewayServer(t, newMockRuntimePort())
	defer runner.close(t)

	conn := dialWebSocket(t, runner.wsURL)
	defer conn.Close()

	invalidRequest := MessageFrame{Type: FrameTypeEvent}
	if err := conn.WriteJSON(invalidRequest); err != nil {
		t.Fatalf("write websocket frame failed: %v", err)
	}

	errFrame := readWSFrame(t, conn)
	if errFrame.Type != FrameTypeError {
		t.Fatalf("unexpected frame type: got %q want %q", errFrame.Type, FrameTypeError)
	}
	if errFrame.Error == nil || errFrame.Error.Code != ErrorCodeInvalidFrame.String() {
		t.Fatalf("unexpected websocket error frame: %#v", errFrame.Error)
	}
}

func TestWebSocketBroadcastRuntimeEventsToAllClients(t *testing.T) {
	mockRuntime := newMockRuntimePort()
	runner := startGatewayServer(t, mockRuntime)
	defer runner.close(t)

	conn1 := dialWebSocket(t, runner.wsURL)
	defer conn1.Close()
	conn2 := dialWebSocket(t, runner.wsURL)
	defer conn2.Close()

	mockRuntime.events <- RuntimeEvent{
		Type:      RuntimeEventTypeRunProgress,
		RunID:     "run_100",
		SessionID: "sess_100",
		Payload:   map[string]any{"message": "delta"},
	}

	frame1 := readWSFrame(t, conn1)
	frame2 := readWSFrame(t, conn2)

	if frame1.Type != FrameTypeEvent || frame2.Type != FrameTypeEvent {
		t.Fatalf("expected event frames, got %#v and %#v", frame1.Type, frame2.Type)
	}
	if frame1.RunID != "run_100" || frame2.RunID != "run_100" {
		t.Fatalf("unexpected run_id values: %q / %q", frame1.RunID, frame2.RunID)
	}
}

func TestServerCloseIsIdempotent(t *testing.T) {
	runner := startGatewayServer(t, newMockRuntimePort())

	ctx1, cancel1 := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel1()
	if err := runner.server.Close(ctx1); err != nil {
		t.Fatalf("first close failed: %v", err)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel2()
	if err := runner.server.Close(ctx2); err != nil {
		t.Fatalf("second close failed: %v", err)
	}

	runner.cancel()
	select {
	case err := <-runner.errCh:
		if err != nil {
			t.Fatalf("serve returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("serve did not exit after close")
	}

	_, err := http.Get(runner.baseURL + "/healthz")
	if err == nil {
		t.Fatalf("expected request failure after close")
	}
}
