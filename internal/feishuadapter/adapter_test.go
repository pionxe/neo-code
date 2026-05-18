package feishuadapter

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"neo-code/internal/gateway/protocol"
)

type fakeGatewayClient struct {
	mu            sync.Mutex
	calls         []string
	notifications chan GatewayNotification
	runCount      int
	resolveCount  int
	cancelCount   int
	authCount     int
	pingErr       error
	authErr       error
	bindErr       error
	resolveErr    error
	cancelErr     error
	runErr        error
	runErrOnce    bool
	cancelResult  bool
	resolveHook   func(requestID string, decision string) error
}

func newFakeGatewayClient() *fakeGatewayClient {
	return &fakeGatewayClient{notifications: make(chan GatewayNotification, 16)}
}

func (f *fakeGatewayClient) Authenticate(context.Context) error {
	f.record("authenticate")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.authCount++
	return f.authErr
}
func (f *fakeGatewayClient) BindStream(_ context.Context, sessionID string, runID string) error {
	f.record("bind:" + sessionID + ":" + runID)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bindErr
}
func (f *fakeGatewayClient) Run(_ context.Context, sessionID string, runID string, inputText string) error {
	f.record("run:" + sessionID + ":" + runID + ":" + inputText)
	f.mu.Lock()
	f.runCount++
	runErr := f.runErr
	if f.runErrOnce {
		f.runErr = nil
		f.runErrOnce = false
	}
	f.mu.Unlock()
	return runErr
}
func (f *fakeGatewayClient) CancelRun(_ context.Context, sessionID string, runID string) (bool, error) {
	f.record("cancel:" + sessionID + ":" + runID)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelCount++
	if f.cancelErr != nil {
		return false, f.cancelErr
	}
	return f.cancelResult, nil
}
func (f *fakeGatewayClient) ResolvePermission(_ context.Context, requestID string, decision string) error {
	f.record("resolve:" + requestID + ":" + decision)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolveCount++
	if f.resolveHook != nil {
		return f.resolveHook(requestID, decision)
	}
	return f.resolveErr
}
func (f *fakeGatewayClient) ResolveUserQuestion(
	_ context.Context,
	requestID string,
	status string,
	values []string,
	message string,
) error {
	f.record("resolve_user_question:" + requestID + ":" + status + ":" + strings.Join(values, ",") + ":" + strings.TrimSpace(message))
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolveCount++
	return f.resolveErr
}
func (f *fakeGatewayClient) Ping(context.Context) error {
	f.record("ping")
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pingErr
}
func (f *fakeGatewayClient) Notifications() <-chan GatewayNotification { return f.notifications }
func (f *fakeGatewayClient) Close() error {
	close(f.notifications)
	return nil
}
func (f *fakeGatewayClient) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}
func (f *fakeGatewayClient) snapshotCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	copy(out, f.calls)
	return out
}

type sentMessage struct {
	chatID               string
	kind                 string
	text                 string
	card                 PermissionCardPayload
	updatedPendingCard   *PermissionCardPayload
	userQuestionCard     UserQuestionCardPayload
	runCard              StatusCardPayload
	cardID               string
	resolvedCard         *ResolvedPermissionCardPayload
	resolvedUserQuestion *ResolvedUserQuestionCardPayload
}

type fakeMessenger struct {
	mu                     sync.Mutex
	messages               []sentMessage
	nextID                 int
	sendTextErr            error
	sendCardErr            error
	updateCardErr          error
	deleteCardErr          error
	sendPermissionCardHook func(cardID string, payload PermissionCardPayload)
}

func (m *fakeMessenger) SendText(_ context.Context, chatID string, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, sentMessage{chatID: chatID, kind: "text", text: text})
	return m.sendTextErr
}

func (m *fakeMessenger) SendPermissionCard(_ context.Context, chatID string, payload PermissionCardPayload) (string, error) {
	m.mu.Lock()
	m.nextID++
	cardID := fmt.Sprintf("perm-card-%d", m.nextID)
	m.messages = append(m.messages, sentMessage{chatID: chatID, kind: "card", card: payload, cardID: cardID})
	hook := m.sendPermissionCardHook
	m.mu.Unlock()
	if hook != nil {
		hook(cardID, payload)
	}
	return cardID, nil
}

func (m *fakeMessenger) UpdatePermissionCard(_ context.Context, cardID string, payload ResolvedPermissionCardPayload) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, sentMessage{chatID: cardID, kind: "update_perm_card", resolvedCard: &payload})
	return nil
}

func (m *fakeMessenger) UpdatePendingPermissionCard(_ context.Context, cardID string, payload PermissionCardPayload) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, sentMessage{chatID: cardID, kind: "update_pending_perm_card", updatedPendingCard: &payload})
	return nil
}

func (m *fakeMessenger) DeleteMessage(_ context.Context, messageID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, sentMessage{chatID: messageID, kind: "delete_card"})
	return m.deleteCardErr
}

func (m *fakeMessenger) SendUserQuestionCard(_ context.Context, chatID string, payload UserQuestionCardPayload) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	cardID := fmt.Sprintf("ask-card-%d", m.nextID)
	m.messages = append(m.messages, sentMessage{chatID: chatID, kind: "ask_card", userQuestionCard: payload, cardID: cardID})
	return cardID, nil
}

func (m *fakeMessenger) UpdateUserQuestionCard(_ context.Context, cardID string, payload ResolvedUserQuestionCardPayload) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, sentMessage{chatID: cardID, kind: "update_ask_card", resolvedUserQuestion: &payload})
	return nil
}

func (m *fakeMessenger) SendStatusCard(_ context.Context, chatID string, payload StatusCardPayload) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	cardID := fmt.Sprintf("card-%d", m.nextID)
	m.messages = append(m.messages, sentMessage{chatID: chatID, kind: "status_card", runCard: payload, cardID: cardID})
	return cardID, m.sendCardErr
}

func (m *fakeMessenger) UpdateCard(_ context.Context, cardID string, payload StatusCardPayload) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, sentMessage{kind: "update_card", runCard: payload, cardID: cardID})
	return m.updateCardErr
}

func (m *fakeMessenger) snapshot() []sentMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]sentMessage, len(m.messages))
	copy(out, m.messages)
	return out
}

func TestBuildIDsStable(t *testing.T) {
	sessionA := BuildSessionID("chat-1")
	sessionB := BuildSessionID("chat-1")
	if sessionA == "" || sessionA != sessionB {
		t.Fatalf("expected stable session id, got %q and %q", sessionA, sessionB)
	}
	runA := BuildRunID("msg-1")
	runB := BuildRunID("msg-1")
	if runA == "" || runA != runB {
		t.Fatalf("expected stable run id, got %q and %q", runA, runB)
	}
}

func TestNewRejectsMissingDependencies(t *testing.T) {
	cfg := Config{
		ListenAddress:       "127.0.0.1:18080",
		EventPath:           "/feishu/events",
		CardPath:            "/feishu/cards",
		AppID:               "app",
		AppSecret:           "secret",
		VerifyToken:         "verify",
		SigningSecret:       "sign-secret",
		RequestTimeout:      time.Second,
		IdempotencyTTL:      time.Minute,
		ReconnectBackoffMin: 100 * time.Millisecond,
		ReconnectBackoffMax: time.Second,
		RebindInterval:      time.Second,
	}
	if _, err := New(cfg, nil, &fakeMessenger{}, nil); err == nil {
		t.Fatal("expected missing gateway error")
	}
	if _, err := New(cfg, newFakeGatewayClient(), nil, nil); err == nil {
		t.Fatal("expected missing messenger error")
	}
}

func TestRunReturnsAuthenticateFailure(t *testing.T) {
	adapter := newTestAdapter(t)
	gateway := adapterTestGateway(adapter)
	gateway.mu.Lock()
	gateway.authErr = assertErr("auth failed")
	gateway.mu.Unlock()

	err := adapter.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "authenticate gateway") {
		t.Fatalf("run error = %v, want authenticate failure", err)
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	adapter := newTestAdapter(t)
	adapter.cfg.ListenAddress = "127.0.0.1:0"
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- adapter.Run(ctx)
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Fatalf("run error = %v, want nil or context canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for adapter shutdown")
	}
}

func TestHandleFeishuEventChallenge(t *testing.T) {
	adapter := newTestAdapter(t)
	body := `{"type":"url_verification","challenge":"abc","token":"verify"}`
	request := signedRequest(t, adapter.cfg.SigningSecret, body)
	recorder := httptest.NewRecorder()
	adapter.handleFeishuEvent(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"challenge":"abc"`) {
		t.Fatalf("response = %s, want challenge", recorder.Body.String())
	}
}

func TestHandleFeishuEventRejectsInvalidSignature(t *testing.T) {
	adapter := newTestAdapter(t)
	request := httptest.NewRequest(http.MethodPost, "/feishu/events", strings.NewReader(`{"type":"url_verification","challenge":"abc"}`))
	request.Header.Set(headerLarkTimestamp, strconvTimestamp(time.Now().UTC()))
	request.Header.Set(headerLarkNonce, "nonce")
	request.Header.Set(headerLarkSignature, "invalid")
	recorder := httptest.NewRecorder()
	adapter.handleFeishuEvent(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

func TestHandleFeishuEventCoversValidationFailures(t *testing.T) {
	adapter := newTestAdapter(t)
	testCases := []struct {
		name string
		body string
		want int
	}{
		{name: "invalid json", body: `{`, want: http.StatusBadRequest},
		{name: "ignored event", body: `{"header":{"event_type":"other","token":"verify"}}`, want: http.StatusOK},
		{name: "invalid token", body: `{"header":{"event_type":"im.message.receive_v1","token":"bad"},"event":{}}`, want: http.StatusUnauthorized},
		{name: "invalid event body", body: `{"header":{"event_type":"im.message.receive_v1","token":"verify"},"event":"oops"}`, want: http.StatusBadRequest},
		{name: "missing ids", body: `{"header":{"event_type":"im.message.receive_v1","token":"verify"},"event":{"message":{"message_id":"","chat_id":""}}}`, want: http.StatusBadRequest},
		{name: "invalid content", body: "{\"header\":{\"event_id\":\"evt-invalid-content\",\"event_type\":\"im.message.receive_v1\",\"token\":\"verify\"},\"event\":{\"message\":{\"message_id\":\"msg-invalid-content\",\"chat_id\":\"chat-invalid-content\",\"content\":\"{\"}}}", want: http.StatusBadRequest},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			request := signedRequest(t, adapter.cfg.SigningSecret, testCase.body)
			recorder := httptest.NewRecorder()
			adapter.handleFeishuEvent(recorder, request)
			if recorder.Code != testCase.want {
				t.Fatalf("status = %d, want %d body=%s", recorder.Code, testCase.want, recorder.Body.String())
			}
		})
	}
}

func TestMessageEventDedupeOnlyRunsOnce(t *testing.T) {
	adapter := newTestAdapter(t)
	body := messageEventBody("evt-1", "msg-1", "chat-1", "hello")
	for i := 0; i < 2; i++ {
		request := signedRequest(t, adapter.cfg.SigningSecret, body)
		recorder := httptest.NewRecorder()
		adapter.handleFeishuEvent(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", recorder.Code)
		}
	}
	if adapterTestGateway(adapter).runCount != 1 {
		t.Fatalf("run count = %d, want 1", adapterTestGateway(adapter).runCount)
	}
}

func TestMessageEventRetryAfterRunFailure(t *testing.T) {
	adapter := newTestAdapter(t)
	gateway := adapterTestGateway(adapter)
	gateway.mu.Lock()
	gateway.runErr = assertErr("transient")
	gateway.runErrOnce = true
	gateway.mu.Unlock()

	body := messageEventBody("evt-retry", "msg-retry", "chat-retry", "hello")
	request := signedRequest(t, adapter.cfg.SigningSecret, body)
	recorder := httptest.NewRecorder()
	adapter.handleFeishuEvent(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("first status = %d, want 500", recorder.Code)
	}
	request = signedRequest(t, adapter.cfg.SigningSecret, body)
	recorder = httptest.NewRecorder()
	adapter.handleFeishuEvent(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("second status = %d, want 200", recorder.Code)
	}
	if adapterTestGateway(adapter).runCount != 2 {
		t.Fatalf("run count = %d, want 2", adapterTestGateway(adapter).runCount)
	}
}

func TestRunFailureCleansTrackedRunBinding(t *testing.T) {
	adapter := newTestAdapter(t)
	gateway := adapterTestGateway(adapter)
	gateway.mu.Lock()
	gateway.runErr = assertErr("reject")
	gateway.mu.Unlock()

	err := adapter.bindThenRun(context.Background(), "session-fail", "run-fail", "chat-fail", "hello")
	if err == nil {
		t.Fatal("expected bindThenRun error")
	}
	adapter.mu.RLock()
	_, exists := adapter.activeRuns[runBindingKey("session-fail", "run-fail")]
	adapter.mu.RUnlock()
	if exists {
		t.Fatal("expected failed run binding to be cleaned")
	}
}

func TestGroupMessageWithoutMentionAccepted(t *testing.T) {
	adapter := newTestAdapter(t)
	body := messageEventBodyWithChatType("evt-group", "msg-group", "chat-group", "hello group", "group")
	request := signedRequest(t, adapter.cfg.SigningSecret, body)
	recorder := httptest.NewRecorder()
	adapter.handleFeishuEvent(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if adapterTestGateway(adapter).runCount != 1 {
		t.Fatalf("run count = %d, want 1", adapterTestGateway(adapter).runCount)
	}
}

func TestGroupMessageWithMentionAccepted(t *testing.T) {
	adapter := newTestAdapter(t)
	content, _ := json.Marshal(map[string]string{"text": "<at user_id=\"app\">neo</at> hi"})
	payload := map[string]any{
		"header": map[string]any{
			"event_id":   "evt-group-mention",
			"event_type": "im.message.receive_v1",
			"token":      "verify",
			"app_id":     "app",
		},
		"event": map[string]any{
			"message": map[string]any{
				"message_id": "msg-group-mention",
				"chat_id":    "chat-group-mention",
				"chat_type":  "group",
				"content":    string(content),
				"mentions": []map[string]any{
					{
						"name": "neo",
						"id": map[string]any{
							"user_id": "ou_bot",
						},
					},
				},
			},
		},
	}
	raw, _ := json.Marshal(payload)
	request := signedRequest(t, adapter.cfg.SigningSecret, string(raw))
	recorder := httptest.NewRecorder()
	adapter.handleFeishuEvent(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if adapterTestGateway(adapter).runCount != 1 {
		t.Fatalf("run count = %d, want 1", adapterTestGateway(adapter).runCount)
	}
}

func TestGroupMessageWithNonBotMentionAccepted(t *testing.T) {
	adapter := newTestAdapter(t)
	content, _ := json.Marshal(map[string]string{"text": "<at user_id=\"ou_other\">alice</at> hi"})
	payload := map[string]any{
		"header": map[string]any{
			"event_id":   "evt-group-non-bot-mention",
			"event_type": "im.message.receive_v1",
			"token":      "verify",
			"app_id":     "app",
		},
		"event": map[string]any{
			"message": map[string]any{
				"message_id": "msg-group-non-bot-mention",
				"chat_id":    "chat-group-non-bot-mention",
				"chat_type":  "group",
				"content":    string(content),
				"mentions": []map[string]any{
					{
						"name": "alice",
						"id": map[string]any{
							"user_id": "ou_other",
						},
					},
				},
			},
		},
	}
	raw, _ := json.Marshal(payload)
	request := signedRequest(t, adapter.cfg.SigningSecret, string(raw))
	recorder := httptest.NewRecorder()
	adapter.handleFeishuEvent(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if adapterTestGateway(adapter).runCount != 1 {
		t.Fatalf("run count = %d, want 1", adapterTestGateway(adapter).runCount)
	}
}

func TestCallOrderAuthenticateBindRun(t *testing.T) {
	adapter := newTestAdapter(t)
	body := messageEventBody("evt-2", "msg-2", "chat-2", "run it")
	request := signedRequest(t, adapter.cfg.SigningSecret, body)
	recorder := httptest.NewRecorder()
	adapter.handleFeishuEvent(recorder, request)

	calls := adapterTestGateway(adapter).snapshotCalls()
	joined := strings.Join(calls, "|")
	authIndex := strings.Index(joined, "authenticate")
	bindIndex := strings.Index(joined, "bind:")
	runIndex := strings.Index(joined, "run:")
	if !(authIndex >= 0 && bindIndex > authIndex && runIndex > bindIndex) {
		t.Fatalf("unexpected call order: %v", calls)
	}
}

func TestGatewayEventsIgnoreStalePermissionRequestedAfterTerminal(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.consumeGatewayEvents(ctx)

	adapter.trackSession(BuildSessionID("chat-x"), BuildRunID("msg-x"), "chat-x", "chat-x task")
	pushGatewayEvent(t, adapterTestGateway(adapter), BuildSessionID("chat-x"), BuildRunID("msg-x"), "run_done", map[string]any{
		"runtime_event_type": "agent_done",
	})
	pushGatewayEvent(t, adapterTestGateway(adapter), BuildSessionID("chat-x"), BuildRunID("msg-x"), "run_error", map[string]any{
		"runtime_event_type": "error",
	})
	pushGatewayEvent(t, adapterTestGateway(adapter), BuildSessionID("chat-x"), BuildRunID("msg-x"), "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-1",
			"reason":     "need approval",
		},
	})
	time.Sleep(30 * time.Millisecond)
	msgs := adapterTestMessenger(adapter).snapshot()
	for _, message := range msgs {
		if message.kind == "card" && message.card.RequestID == "perm-1" {
			t.Fatalf("unexpected stale permission card after run terminal: %#v", msgs)
		}
	}
}

func TestGatewayUserQuestionRequestedSingleChoiceSendsCard(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.consumeGatewayEvents(ctx)

	sessionID := BuildSessionID("chat-ask-card")
	runID := BuildRunID("msg-ask-card")
	adapter.trackSession(sessionID, runID, "chat-ask-card", "ask task")

	pushGatewayEvent(t, adapterTestGateway(adapter), sessionID, runID, "run_progress", map[string]any{
		"runtime_event_type": "user_question_requested",
		"payload": map[string]any{
			"request_id":  "ask-card-1",
			"question_id": "q1",
			"title":       "选择部署环境",
			"kind":        "single_choice",
			"allow_skip":  true,
			"options": []any{
				map[string]any{"label": "测试环境"},
				map[string]any{"label": "生产环境"},
			},
		},
	})
	time.Sleep(30 * time.Millisecond)

	msgs := adapterTestMessenger(adapter).snapshot()
	found := false
	for _, message := range msgs {
		if message.kind == "ask_card" && message.userQuestionCard.RequestID == "ask-card-1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected ask_user card message, got %#v", msgs)
	}
}

func TestGatewayUserQuestionRequestedTextFallsBackToInstructionText(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.consumeGatewayEvents(ctx)

	sessionID := BuildSessionID("chat-ask-text")
	runID := BuildRunID("msg-ask-text")
	adapter.trackSession(sessionID, runID, "chat-ask-text", "ask text task")

	pushGatewayEvent(t, adapterTestGateway(adapter), sessionID, runID, "run_progress", map[string]any{
		"runtime_event_type": "user_question_requested",
		"payload": map[string]any{
			"request_id":  "ask-text-1",
			"question_id": "q-text-1",
			"title":       "请输入备注",
			"kind":        "text",
			"allow_skip":  false,
		},
	})
	time.Sleep(30 * time.Millisecond)

	msgs := adapterTestMessenger(adapter).snapshot()
	foundInstruction := false
	for _, message := range msgs {
		if message.kind == "text" && strings.Contains(message.text, "回答 ask-text-1") {
			foundInstruction = true
			break
		}
	}
	if !foundInstruction {
		t.Fatalf("expected ask_user instruction text, got %#v", msgs)
	}
}

func TestBindThenRunCreatesStatusCard(t *testing.T) {
	adapter := newTestAdapter(t)
	if err := adapter.bindThenRun(context.Background(), "session-card", "run-card", "chat-card", "编写发布说明"); err != nil {
		t.Fatalf("bindThenRun: %v", err)
	}
	msgs := adapterTestMessenger(adapter).snapshot()
	found := false
	for _, message := range msgs {
		if message.kind == "status_card" && message.runCard.TaskName == "编写发布说明" && message.runCard.Status == "thinking" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected status card message, got %#v", msgs)
	}
}

func TestGatewayEventsUpdateStatusCard(t *testing.T) {
	adapter := newTestAdapter(t)
	if err := adapter.bindThenRun(context.Background(), "session-progress", "run-progress", "chat-progress", "整理计划"); err != nil {
		t.Fatalf("bindThenRun: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.consumeGatewayEvents(ctx)

	pushGatewayEvent(t, adapterTestGateway(adapter), "session-progress", "run-progress", "run_progress", map[string]any{
		"runtime_event_type": "phase_changed",
		"payload": map[string]any{
			"to": "plan",
		},
	})
	pushGatewayEvent(t, adapterTestGateway(adapter), "session-progress", "run-progress", "run_progress", map[string]any{
		"runtime_event_type": "hook_notification",
		"payload": map[string]any{
			"summary": "已收到异步回灌摘要",
			"reason":  "async_rewake",
		},
	})
	pushGatewayEvent(t, adapterTestGateway(adapter), "session-progress", "run-progress", "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-status",
			"reason":     "需要确认是否执行命令",
		},
	})
	time.Sleep(30 * time.Millisecond)
	if err := adapter.HandleCardAction(context.Background(), FeishuCardActionEvent{
		RequestID: "perm-status",
		Decision:  "allow_once",
	}); err != nil {
		t.Fatalf("handle card action: %v", err)
	}
	pushGatewayEvent(t, adapterTestGateway(adapter), "session-progress", "run-progress", "run_done", map[string]any{
		"runtime_event_type": "agent_done",
		"payload": map[string]any{
			"content": "任务完成",
		},
	})
	time.Sleep(30 * time.Millisecond)

	msgs := adapterTestMessenger(adapter).snapshot()
	foundPlanning := false
	foundApproved := false
	foundSuccess := false
	for _, message := range msgs {
		if message.kind != "update_card" {
			continue
		}
		if message.runCard.Status == "planning" {
			foundPlanning = true
		}
		if message.runCard.ApprovalStatus == "approved" {
			foundApproved = true
		}
		if message.runCard.Result == "success" && strings.Contains(message.runCard.Summary, "任务完成") {
			foundSuccess = true
		}
	}
	if !foundPlanning || !foundApproved || !foundSuccess {
		t.Fatalf("unexpected card updates: %#v", msgs)
	}
}

func TestPermissionApprovalDoesNotRevertToPendingAndKeepsResolvedCard(t *testing.T) {
	adapter := newTestAdapter(t)
	adapter.permissionCardDismissDelay = 5 * time.Millisecond
	if err := adapter.bindThenRun(context.Background(), "session-approval", "run-approval", "chat-approval", "执行审批任务"); err != nil {
		t.Fatalf("bindThenRun: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.consumeGatewayEvents(ctx)

	pushGatewayEvent(t, adapterTestGateway(adapter), "session-approval", "run-approval", "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-revert-1",
			"reason":     "需要审批",
		},
	})
	time.Sleep(20 * time.Millisecond)

	if err := adapter.HandleCardAction(context.Background(), FeishuCardActionEvent{
		RequestID: "perm-revert-1",
		Decision:  "allow_once",
	}); err != nil {
		t.Fatalf("handle card action: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	// 网关重复推送同一 permission_requested 时，不应把已审批状态回滚成 pending，也不应重发审批卡片。
	pushGatewayEvent(t, adapterTestGateway(adapter), "session-approval", "run-approval", "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-revert-1",
			"reason":     "重复事件",
		},
	})
	time.Sleep(30 * time.Millisecond)

	msgs := adapterTestMessenger(adapter).snapshot()
	cardSendCount := 0
	updatedResolved := false
	deletedResolvedCard := false
	lastApprovalStatus := ""
	for _, message := range msgs {
		if message.kind == "card" && message.card.RequestID == "perm-revert-1" {
			cardSendCount++
		}
		if message.kind == "update_perm_card" && message.resolvedCard != nil &&
			message.resolvedCard.RequestID == "perm-revert-1" &&
			message.resolvedCard.Approved {
			updatedResolved = true
		}
		if message.kind == "delete_card" {
			deletedResolvedCard = true
		}
		if message.kind == "update_card" {
			lastApprovalStatus = message.runCard.ApprovalStatus
		}
	}
	if cardSendCount != 1 {
		t.Fatalf("permission card send count = %d, want 1; msgs=%#v", cardSendCount, msgs)
	}
	if !updatedResolved {
		t.Fatalf("expected resolved permission card update, msgs=%#v", msgs)
	}
	if deletedResolvedCard {
		t.Fatalf("resolved permission card should stay visible in queue mode, msgs=%#v", msgs)
	}
	if lastApprovalStatus != "approved" {
		t.Fatalf("last approval status = %q, want approved; msgs=%#v", lastApprovalStatus, msgs)
	}
}

func TestPermissionApprovalsDoNotAutoPushQueuedCardOnResolve(t *testing.T) {
	adapter := newTestAdapter(t)
	if err := adapter.bindThenRun(context.Background(), "session-queue", "run-queue", "chat-queue", "执行审批队列任务"); err != nil {
		t.Fatalf("bindThenRun: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.consumeGatewayEvents(ctx)

	pushGatewayEvent(t, adapterTestGateway(adapter), "session-queue", "run-queue", "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-queue-1",
			"reason":     "审批一",
		},
	})
	pushGatewayEvent(t, adapterTestGateway(adapter), "session-queue", "run-queue", "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-queue-2",
			"reason":     "审批二",
		},
	})
	time.Sleep(40 * time.Millisecond)

	msgs := adapterTestMessenger(adapter).snapshot()
	cardSendBeforeResolve := 0
	for _, message := range msgs {
		if message.kind == "card" {
			cardSendBeforeResolve++
		}
	}
	if cardSendBeforeResolve != 1 {
		t.Fatalf("permission cards before resolve = %d, want 1; msgs=%#v", cardSendBeforeResolve, msgs)
	}

	if err := adapter.HandleCardAction(context.Background(), FeishuCardActionEvent{
		RequestID: "perm-queue-1",
		Decision:  "allow_once",
	}); err != nil {
		t.Fatalf("handle first card action: %v", err)
	}
	time.Sleep(40 * time.Millisecond)

	msgs = adapterTestMessenger(adapter).snapshot()
	totalCardSend := 0
	resolvedUpdates := 0
	pendingUpdates := 0
	for _, message := range msgs {
		if message.kind == "card" {
			totalCardSend++
		}
		if message.kind == "update_perm_card" && message.resolvedCard != nil &&
			message.resolvedCard.RequestID == "perm-queue-1" {
			resolvedUpdates++
		}
		if message.kind == "update_pending_perm_card" && message.updatedPendingCard != nil &&
			message.updatedPendingCard.RequestID == "perm-queue-2" {
			pendingUpdates++
		}
	}
	if totalCardSend != 1 {
		t.Fatalf("permission cards total after first resolve = %d, want 1; msgs=%#v", totalCardSend, msgs)
	}
	if resolvedUpdates == 0 {
		t.Fatalf("expected resolved state visible before switching to next pending, msgs=%#v", msgs)
	}
	if pendingUpdates == 0 {
		t.Fatalf("expected pending card remains focused on newer request, msgs=%#v", msgs)
	}
}

func TestPermissionCardReusedAcrossSequentialRequests(t *testing.T) {
	adapter := newTestAdapter(t)
	if err := adapter.bindThenRun(context.Background(), "session-reuse", "run-reuse", "chat-reuse", "执行审批复用任务"); err != nil {
		t.Fatalf("bindThenRun: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.consumeGatewayEvents(ctx)

	pushGatewayEvent(t, adapterTestGateway(adapter), "session-reuse", "run-reuse", "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-reuse-1",
			"reason":     "审批一",
		},
	})
	time.Sleep(30 * time.Millisecond)

	if err := adapter.HandleCardAction(context.Background(), FeishuCardActionEvent{
		RequestID: "perm-reuse-1",
		Decision:  "allow_once",
	}); err != nil {
		t.Fatalf("handle first card action: %v", err)
	}
	time.Sleep(30 * time.Millisecond)

	pushGatewayEvent(t, adapterTestGateway(adapter), "session-reuse", "run-reuse", "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-reuse-2",
			"reason":     "审批二",
		},
	})
	time.Sleep(40 * time.Millisecond)

	msgs := adapterTestMessenger(adapter).snapshot()
	cardSends := 0
	pendingUpdates := 0
	resolvedUpdates := 0
	lastPendingRequestID := ""
	for _, message := range msgs {
		switch message.kind {
		case "card":
			cardSends++
		case "update_pending_perm_card":
			pendingUpdates++
			if message.updatedPendingCard != nil {
				lastPendingRequestID = message.updatedPendingCard.RequestID
			}
		case "update_perm_card":
			resolvedUpdates++
		}
	}
	if cardSends != 1 {
		t.Fatalf("permission card sends = %d, want 1; msgs=%#v", cardSends, msgs)
	}
	if pendingUpdates < 1 {
		t.Fatalf("expected pending permission card update for second request, msgs=%#v", msgs)
	}
	if resolvedUpdates < 1 {
		t.Fatalf("expected resolved permission card update for first request, msgs=%#v", msgs)
	}
	if lastPendingRequestID != "perm-reuse-2" {
		t.Fatalf("last pending request id = %q, want perm-reuse-2; msgs=%#v", lastPendingRequestID, msgs)
	}
}

func TestPermissionQueuedRequestDoesNotOverrideActiveCardBeforeResolve(t *testing.T) {
	adapter := newTestAdapter(t)
	if err := adapter.bindThenRun(context.Background(), "session-queue-order", "run-queue-order", "chat-queue-order", "执行审批时序任务"); err != nil {
		t.Fatalf("bindThenRun: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.consumeGatewayEvents(ctx)

	pushGatewayEvent(t, adapterTestGateway(adapter), "session-queue-order", "run-queue-order", "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-order-1",
			"reason":     "审批一",
		},
	})
	time.Sleep(30 * time.Millisecond)

	pushGatewayEvent(t, adapterTestGateway(adapter), "session-queue-order", "run-queue-order", "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-order-2",
			"reason":     "审批二",
		},
	})
	time.Sleep(60 * time.Millisecond)

	msgs := adapterTestMessenger(adapter).snapshot()
	for _, message := range msgs {
		if message.kind == "update_pending_perm_card" &&
			message.updatedPendingCard != nil &&
			message.updatedPendingCard.RequestID == "perm-order-2" {
			t.Fatalf("queued pending request should not override active card before resolve, msgs=%#v", msgs)
		}
	}

	if err := adapter.HandleCardAction(context.Background(), FeishuCardActionEvent{
		RequestID: "perm-order-1",
		Decision:  "allow_once",
	}); err != nil {
		t.Fatalf("handle first card action: %v", err)
	}
	time.Sleep(750 * time.Millisecond)

	msgs = adapterTestMessenger(adapter).snapshot()
	resolvedIndex := -1
	pendingIndex := -1
	for i, message := range msgs {
		if message.kind == "update_perm_card" &&
			message.resolvedCard != nil &&
			message.resolvedCard.RequestID == "perm-order-1" {
			resolvedIndex = i
		}
		if message.kind == "update_pending_perm_card" &&
			message.updatedPendingCard != nil &&
			message.updatedPendingCard.RequestID == "perm-order-2" {
			pendingIndex = i
		}
	}
	if resolvedIndex < 0 {
		t.Fatalf("expected resolved card update for first request, msgs=%#v", msgs)
	}
	if pendingIndex < 0 {
		t.Fatalf("expected switch to second pending request after resolve, msgs=%#v", msgs)
	}
	if pendingIndex <= resolvedIndex {
		t.Fatalf("pending switch should happen after resolved card update, msgs=%#v", msgs)
	}
}

func TestPermissionQueueSwitchPrefersNewestPendingAfterResolve(t *testing.T) {
	adapter := newTestAdapter(t)
	if err := adapter.bindThenRun(context.Background(), "session-queue-newest", "run-queue-newest", "chat-queue-newest", "执行审批新队列优先任务"); err != nil {
		t.Fatalf("bindThenRun: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.consumeGatewayEvents(ctx)

	pushGatewayEvent(t, adapterTestGateway(adapter), "session-queue-newest", "run-queue-newest", "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-newest-1",
			"reason":     "审批一",
		},
	})
	pushGatewayEvent(t, adapterTestGateway(adapter), "session-queue-newest", "run-queue-newest", "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-newest-2",
			"reason":     "审批二",
		},
	})
	pushGatewayEvent(t, adapterTestGateway(adapter), "session-queue-newest", "run-queue-newest", "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-newest-3",
			"reason":     "审批三",
		},
	})
	time.Sleep(80 * time.Millisecond)

	if err := adapter.HandleCardAction(context.Background(), FeishuCardActionEvent{
		RequestID: "perm-newest-1",
		Decision:  "allow_once",
	}); err != nil {
		t.Fatalf("handle first card action: %v", err)
	}
	time.Sleep(1500 * time.Millisecond)

	msgs := adapterTestMessenger(adapter).snapshot()
	lastPendingRequestID := ""
	for _, message := range msgs {
		if message.kind == "update_pending_perm_card" && message.updatedPendingCard != nil {
			lastPendingRequestID = message.updatedPendingCard.RequestID
		}
	}
	if lastPendingRequestID == "" {
		t.Fatalf("expected pending switch update after resolving first approval, msgs=%#v", msgs)
	}
	if lastPendingRequestID != "perm-newest-3" {
		t.Fatalf("pending switch should prefer newest request, got %q; msgs=%#v", lastPendingRequestID, msgs)
	}
}

func TestPermissionActionStrictlyIgnoresStaleCallbackWithoutFallback(t *testing.T) {
	adapter := newTestAdapter(t)
	if err := adapter.bindThenRun(context.Background(), "session-stale", "run-stale", "chat-stale", "执行审批回退任务"); err != nil {
		t.Fatalf("bindThenRun: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.consumeGatewayEvents(ctx)

	pushGatewayEvent(t, adapterTestGateway(adapter), "session-stale", "run-stale", "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-stale-1",
			"reason":     "审批一",
		},
	})
	time.Sleep(20 * time.Millisecond)

	if err := adapter.HandleCardAction(context.Background(), FeishuCardActionEvent{
		RequestID: "perm-stale-1",
		Decision:  "allow_once",
	}); err != nil {
		t.Fatalf("resolve first permission: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	pushGatewayEvent(t, adapterTestGateway(adapter), "session-stale", "run-stale", "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-stale-2",
			"reason":     "审批二",
		},
	})
	time.Sleep(30 * time.Millisecond)
	permissionCardID := ""
	for _, message := range adapterTestMessenger(adapter).snapshot() {
		if message.kind == "card" {
			permissionCardID = message.cardID
		}
	}
	if permissionCardID == "" {
		t.Fatal("expected permission card id for stale fallback test")
	}

	gateway := adapterTestGateway(adapter)
	gateway.mu.Lock()
	gateway.resolveHook = func(requestID string, _ string) error {
		if requestID == "perm-stale-1" {
			return assertErr(`runtime: permission request "perm-stale-1" not found`)
		}
		return nil
	}
	gateway.mu.Unlock()

	if err := adapter.HandleCardAction(context.Background(), FeishuCardActionEvent{
		RequestID: "perm-stale-1",
		CardID:    permissionCardID,
		Decision:  "reject",
	}); err != nil {
		t.Fatalf("resolve stale permission should be ignored in strict mode: %v", err)
	}
	time.Sleep(30 * time.Millisecond)

	calls := gateway.snapshotCalls()
	resolveStale := 0
	resolveUnexpectedFallback := 0
	for _, call := range calls {
		if call == "resolve:perm-stale-1:reject" {
			resolveStale++
		}
		if call == "resolve:perm-stale-2:reject" {
			resolveUnexpectedFallback++
		}
	}
	if resolveStale != 0 {
		t.Fatalf("stale callback should not call resolve in strict mode, got %#v", calls)
	}
	if resolveUnexpectedFallback != 0 {
		t.Fatalf("stale callback must not fallback-resolve next pending request, got %#v", calls)
	}
}

func TestPermissionActionIgnoresAlreadyResolvedRequestOnOpaquePrimaryError(t *testing.T) {
	adapter := newTestAdapter(t)
	if err := adapter.bindThenRun(context.Background(), "session-opaque", "run-opaque", "chat-opaque", "执行审批回退任务"); err != nil {
		t.Fatalf("bindThenRun: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.consumeGatewayEvents(ctx)

	pushGatewayEvent(t, adapterTestGateway(adapter), "session-opaque", "run-opaque", "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-opaque-1",
			"reason":     "审批一",
		},
	})
	time.Sleep(20 * time.Millisecond)
	if err := adapter.HandleCardAction(context.Background(), FeishuCardActionEvent{
		RequestID: "perm-opaque-1",
		Decision:  "allow_once",
	}); err != nil {
		t.Fatalf("resolve first permission: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	pushGatewayEvent(t, adapterTestGateway(adapter), "session-opaque", "run-opaque", "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-opaque-2",
			"reason":     "审批二",
		},
	})
	time.Sleep(30 * time.Millisecond)

	permissionCardID := ""
	for _, message := range adapterTestMessenger(adapter).snapshot() {
		if message.kind == "card" {
			permissionCardID = message.cardID
		}
	}
	if permissionCardID == "" {
		t.Fatal("expected permission card id for opaque fallback test")
	}

	gateway := adapterTestGateway(adapter)
	gateway.mu.Lock()
	gateway.resolveHook = func(requestID string, _ string) error {
		if strings.TrimSpace(requestID) == "perm-opaque-1" {
			return assertErr("gateway rpc gateway.resolvePermission failed (internal_error): resolve_permission failed")
		}
		return nil
	}
	gateway.mu.Unlock()

	err := adapter.HandleCardAction(context.Background(), FeishuCardActionEvent{
		EventID:    "evt-opaque-fallback",
		RequestID:  "perm-opaque-1",
		Decision:   "allow_once",
		ActionType: "permission",
		CardID:     permissionCardID,
	})
	if err != nil {
		t.Fatalf("resolve opaque stale permission should be ignored: %v", err)
	}

	time.Sleep(20 * time.Millisecond)
	calls := gateway.snapshotCalls()
	resolvePrimary := 0
	for _, call := range calls {
		if call == "resolve:perm-opaque-1:allow_once" {
			resolvePrimary++
		}
	}
	if resolvePrimary != 1 {
		t.Fatalf("expected only initial resolve call for perm-opaque-1, got %#v", calls)
	}
	for _, call := range calls {
		if call == "resolve:perm-opaque-2:allow_once" {
			t.Fatalf("stale callback must not fallback-resolve next pending request, got %#v", calls)
		}
	}
}

func TestPermissionCallbackStrictRejectsNonActiveRequest(t *testing.T) {
	adapter := newTestAdapter(t)
	if err := adapter.bindThenRun(context.Background(), "session-strict-active", "run-strict-active", "chat-strict-active", "执行严格回调任务"); err != nil {
		t.Fatalf("bindThenRun: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.consumeGatewayEvents(ctx)

	pushGatewayEvent(t, adapterTestGateway(adapter), "session-strict-active", "run-strict-active", "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-strict-active-1",
			"reason":     "审批一",
		},
	})
	pushGatewayEvent(t, adapterTestGateway(adapter), "session-strict-active", "run-strict-active", "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-strict-active-2",
			"reason":     "审批二",
		},
	})
	time.Sleep(60 * time.Millisecond)

	if err := adapter.HandleCardAction(context.Background(), FeishuCardActionEvent{
		RequestID: "perm-strict-active-2",
		Decision:  "reject",
	}); err != nil {
		t.Fatalf("strict callback for non-active request should be ignored: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	calls := adapterTestGateway(adapter).snapshotCalls()
	for _, call := range calls {
		if call == "resolve:perm-strict-active-2:reject" {
			t.Fatalf("non-active request must not be resolved in strict mode, calls=%#v", calls)
		}
	}
}

func TestPermissionCallbackStrictRejectsCardRunMismatch(t *testing.T) {
	adapter := newTestAdapter(t)
	if err := adapter.bindThenRun(context.Background(), "session-strict-card", "run-strict-card", "chat-strict-card", "执行严格回调任务"); err != nil {
		t.Fatalf("bindThenRun: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.consumeGatewayEvents(ctx)

	pushGatewayEvent(t, adapterTestGateway(adapter), "session-strict-card", "run-strict-card", "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-strict-card-1",
			"reason":     "审批一",
		},
	})
	time.Sleep(40 * time.Millisecond)

	if err := adapter.HandleCardAction(context.Background(), FeishuCardActionEvent{
		RequestID: "perm-strict-card-1",
		CardID:    "perm-card-unrelated",
		Decision:  "allow_once",
	}); err != nil {
		t.Fatalf("strict callback with card mismatch should be ignored: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	calls := adapterTestGateway(adapter).snapshotCalls()
	for _, call := range calls {
		if call == "resolve:perm-strict-card-1:allow_once" {
			t.Fatalf("card mismatch must not call resolve, calls=%#v", calls)
		}
	}
}

func TestPermissionCallbackStrictRejectsNonDisplayingPendingState(t *testing.T) {
	adapter := newTestAdapter(t)
	if err := adapter.bindThenRun(context.Background(), "session-strict-state", "run-strict-state", "chat-strict-state", "执行严格回调任务"); err != nil {
		t.Fatalf("bindThenRun: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.consumeGatewayEvents(ctx)

	pushGatewayEvent(t, adapterTestGateway(adapter), "session-strict-state", "run-strict-state", "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-strict-state-1",
			"reason":     "审批一",
		},
	})
	time.Sleep(40 * time.Millisecond)

	runKey := runBindingKey("session-strict-state", "run-strict-state")
	adapter.mu.Lock()
	if fsm := adapter.approvalFSMByRun[runKey]; fsm != nil {
		node := fsm.Requests["perm-strict-state-1"]
		node.State = approvalRequestStateQueued
		fsm.Requests["perm-strict-state-1"] = node
	}
	adapter.mu.Unlock()

	if err := adapter.HandleCardAction(context.Background(), FeishuCardActionEvent{
		RequestID: "perm-strict-state-1",
		Decision:  "allow_once",
	}); err != nil {
		t.Fatalf("strict callback with non-displaying state should be ignored: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	calls := adapterTestGateway(adapter).snapshotCalls()
	for _, call := range calls {
		if call == "resolve:perm-strict-state-1:allow_once" {
			t.Fatalf("non-displaying-pending state must not call resolve, calls=%#v", calls)
		}
	}
}

func TestHandleCardActionAcceptsAllowAliasDecision(t *testing.T) {
	adapter := newTestAdapter(t)
	if err := adapter.bindThenRun(context.Background(), "session-allow-alias", "run-allow-alias", "chat-allow-alias", "执行审批别名任务"); err != nil {
		t.Fatalf("bindThenRun: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.consumeGatewayEvents(ctx)

	pushGatewayEvent(t, adapterTestGateway(adapter), "session-allow-alias", "run-allow-alias", "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-allow-alias-1",
			"reason":     "审批别名",
		},
	})
	time.Sleep(30 * time.Millisecond)

	if err := adapter.HandleCardAction(context.Background(), FeishuCardActionEvent{
		RequestID: "perm-allow-alias-1",
		Decision:  "allow",
	}); err != nil {
		t.Fatalf("handle allow alias decision: %v", err)
	}
	time.Sleep(30 * time.Millisecond)

	msgs := adapterTestMessenger(adapter).snapshot()
	foundResolved := false
	for _, message := range msgs {
		if message.kind == "update_perm_card" && message.resolvedCard != nil &&
			message.resolvedCard.RequestID == "perm-allow-alias-1" && message.resolvedCard.Approved {
			foundResolved = true
			break
		}
	}
	if !foundResolved {
		t.Fatalf("expected resolved permission card update for allow alias, msgs=%#v", msgs)
	}
}

func TestHandleMessageInterruptsPreviousPendingRunInSameChat(t *testing.T) {
	adapter := newTestAdapter(t)
	gateway := adapterTestGateway(adapter)
	gateway.mu.Lock()
	gateway.cancelResult = true
	gateway.mu.Unlock()

	first := FeishuMessageEvent{
		EventID:     "evt-int-1",
		MessageID:   "msg-int-1",
		ChatID:      "chat-interrupt",
		ChatType:    "p2p",
		ContentText: "第一条任务",
	}
	second := FeishuMessageEvent{
		EventID:     "evt-int-2",
		MessageID:   "msg-int-2",
		ChatID:      "chat-interrupt",
		ChatType:    "p2p",
		ContentText: "第二条任务",
	}
	if err := adapter.HandleMessage(context.Background(), first); err != nil {
		t.Fatalf("handle first message: %v", err)
	}
	if err := adapter.HandleMessage(context.Background(), second); err != nil {
		t.Fatalf("handle second message: %v", err)
	}
	time.Sleep(30 * time.Millisecond)

	sessionID := BuildSessionID("chat-interrupt")
	firstRunID := BuildRunID("msg-int-1")
	secondRunID := BuildRunID("msg-int-2")

	calls := gateway.snapshotCalls()
	wantCancel := "cancel:" + sessionID + ":" + firstRunID
	foundCancel := false
	for _, call := range calls {
		if call == wantCancel {
			foundCancel = true
			break
		}
	}
	if !foundCancel {
		t.Fatalf("expected cancel call %q, got %#v", wantCancel, calls)
	}

	msgs := adapterTestMessenger(adapter).snapshot()
	foundInterrupted := false
	for _, message := range msgs {
		if message.kind == "update_card" &&
			message.runCard.Status == "interrupted" &&
			message.runCard.Result == "interrupted" {
			foundInterrupted = true
			break
		}
	}
	if !foundInterrupted {
		t.Fatalf("expected interrupted status card update, msgs=%#v", msgs)
	}

	adapter.mu.RLock()
	_, oldExists := adapter.activeRuns[runBindingKey(sessionID, firstRunID)]
	_, newExists := adapter.activeRuns[runBindingKey(sessionID, secondRunID)]
	adapter.mu.RUnlock()
	if oldExists {
		t.Fatal("expected old run to be untracked after interrupt")
	}
	if !newExists {
		t.Fatal("expected new run to stay active")
	}
}

func TestPermissionResolvedEventUpdatesPermissionCard(t *testing.T) {
	adapter := newTestAdapter(t)
	if err := adapter.bindThenRun(context.Background(), "session-resolve", "run-resolve", "chat-resolve", "执行审批事件任务"); err != nil {
		t.Fatalf("bindThenRun: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.consumeGatewayEvents(ctx)

	pushGatewayEvent(t, adapterTestGateway(adapter), "session-resolve", "run-resolve", "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-resolve-1",
			"tool_name":  "filesystem_write_file",
			"reason":     "需要写文件权限",
		},
	})
	pushGatewayEvent(t, adapterTestGateway(adapter), "session-resolve", "run-resolve", "run_progress", map[string]any{
		"runtime_event_type": "permission_resolved",
		"payload": map[string]any{
			"request_id": "perm-resolve-1",
			"decision":   "allow",
		},
	})
	time.Sleep(40 * time.Millisecond)

	msgs := adapterTestMessenger(adapter).snapshot()
	updatedResolved := false
	lastApprovalStatus := ""
	for _, message := range msgs {
		if message.kind == "update_perm_card" && message.resolvedCard != nil &&
			message.resolvedCard.RequestID == "perm-resolve-1" && message.resolvedCard.Approved {
			updatedResolved = true
		}
		if message.kind == "update_card" {
			lastApprovalStatus = message.runCard.ApprovalStatus
		}
	}
	if !updatedResolved {
		t.Fatalf("expected resolved permission card update from permission_resolved event, msgs=%#v", msgs)
	}
	if lastApprovalStatus != "approved" {
		t.Fatalf("last approval status = %q, want approved; msgs=%#v", lastApprovalStatus, msgs)
	}
}

func TestUpdateApprovalStatusFinalizesHistoricalPermissionCards(t *testing.T) {
	adapter := newTestAdapter(t)
	sessionID := BuildSessionID("chat-approval-history")
	runID := BuildRunID("msg-approval-history")
	chatID := "chat-approval-history"
	adapter.trackSession(sessionID, runID, chatID, "history permission task")
	if err := adapter.ensureRunCard(context.Background(), sessionID, runID); err != nil {
		t.Fatalf("ensureRunCard: %v", err)
	}

	key := runBindingKey(sessionID, runID)
	adapter.mu.Lock()
	binding := adapter.activeRuns[key]
	binding.ApprovalRecords = []approvalEntry{
		{
			RequestID: "perm-history-1",
			ToolName:  "filesystem_write_file",
			Operation: "write_file",
			Target:    "1.txt",
			Reason:    "需要写文件权限",
			Decision:  "pending",
		},
	}
	adapter.activeRuns[key] = binding
	adapter.approvalFSMByRun[key] = &approvalFSMState{
		Generation:      1,
		Version:         1,
		CardID:          "perm-card-current",
		ActiveRequestID: "perm-history-1",
		Requests: map[string]approvalRequestNode{
			"perm-history-1": {
				RequestID:  "perm-history-1",
				ToolName:   "filesystem_write_file",
				Operation:  "write_file",
				Target:     "1.txt",
				Reason:     "需要写文件权限",
				Decision:   "pending",
				State:      approvalRequestStateDisplayingPending,
				UpdatedVer: 1,
			},
		},
	}
	adapter.approvalRequestRunIndex[approvalRequestScopedKey(key, "perm-history-1")] = key
	adapter.approvalRequestIDRunIndex["perm-history-1"] = key
	adapter.approvalCardRunIndex["perm-card-current"] = key
	adapter.approvalCardRunIndex["perm-card-old"] = key
	adapter.runPermissionCardHistory[key] = map[string]struct{}{
		"perm-card-current": {},
		"perm-card-old":     {},
	}
	adapter.mu.Unlock()

	adapter.updateApprovalStatus("perm-history-1", "allow_once")

	msgs := adapterTestMessenger(adapter).snapshot()
	updatedCurrent := false
	updatedOld := false
	for _, message := range msgs {
		if message.kind != "update_perm_card" || message.resolvedCard == nil || !message.resolvedCard.Approved {
			continue
		}
		if message.chatID == "perm-card-current" && message.resolvedCard.RequestID == "perm-history-1" {
			updatedCurrent = true
		}
		if message.chatID == "perm-card-old" && message.resolvedCard.RequestID == "perm-history-1" {
			updatedOld = true
		}
	}
	if !updatedCurrent || !updatedOld {
		t.Fatalf("expected current and historical permission cards finalized, msgs=%#v", msgs)
	}
}

func TestRunTerminalFinalizesPermissionCardUsingRunScopedCardFallback(t *testing.T) {
	adapter := newTestAdapter(t)
	sessionID := BuildSessionID("chat-terminal-perm")
	runID := BuildRunID("msg-terminal-perm")
	chatID := "chat-terminal-perm"
	adapter.trackSession(sessionID, runID, chatID, "terminal permission task")
	if err := adapter.ensureRunCard(context.Background(), sessionID, runID); err != nil {
		t.Fatalf("ensureRunCard: %v", err)
	}

	key := runBindingKey(sessionID, runID)
	adapter.mu.Lock()
	binding := adapter.activeRuns[key]
	binding.ApprovalRecords = []approvalEntry{
		{
			RequestID: "perm-terminal-1",
			ToolName:  "filesystem_write_file",
			Operation: "write_file",
			Target:    "1.txt",
			Reason:    "需要写文件权限",
			Decision:  "allow_once",
		},
	}
	adapter.activeRuns[key] = binding
	adapter.approvalFSMByRun[key] = &approvalFSMState{
		Generation:      1,
		Version:         1,
		CardID:          "perm-card-fallback",
		ActiveRequestID: "",
		Requests: map[string]approvalRequestNode{
			"perm-terminal-1": {
				RequestID:  "perm-terminal-1",
				ToolName:   "filesystem_write_file",
				Operation:  "write_file",
				Target:     "1.txt",
				Reason:     "需要写文件权限",
				Decision:   "allow_once",
				State:      approvalRequestStateResolvedApproved,
				UpdatedVer: 1,
			},
		},
	}
	adapter.approvalRequestRunIndex[approvalRequestScopedKey(key, "perm-terminal-1")] = key
	adapter.approvalRequestIDRunIndex["perm-terminal-1"] = key
	adapter.approvalCardRunIndex["perm-card-fallback"] = key
	adapter.approvalCardRunIndex["perm-card-stale"] = key
	adapter.runPermissionCardHistory[key] = map[string]struct{}{
		"perm-card-fallback": {},
		"perm-card-stale":    {},
	}
	adapter.mu.Unlock()

	adapter.markRunTerminal(sessionID, runID, "success", "done", "")

	msgs := adapterTestMessenger(adapter).snapshot()
	foundFallbackResolved := false
	foundStaleResolved := false
	for _, message := range msgs {
		if message.kind == "update_perm_card" &&
			message.resolvedCard != nil &&
			message.resolvedCard.RequestID == "perm-terminal-1" &&
			message.resolvedCard.Approved {
			if message.chatID == "perm-card-fallback" {
				foundFallbackResolved = true
			}
			if message.chatID == "perm-card-stale" {
				foundStaleResolved = true
			}
		}
	}
	if !foundFallbackResolved || !foundStaleResolved {
		t.Fatalf("expected terminal fallback to finalize current and stale permission cards, msgs=%#v", msgs)
	}
}

func TestHandleCardActionUserQuestionResolvesAndUpdatesCard(t *testing.T) {
	adapter := newTestAdapter(t)
	sessionID := BuildSessionID("chat-ask-resolve")
	runID := BuildRunID("msg-ask-resolve")
	adapter.trackSession(sessionID, runID, "chat-ask-resolve", "ask resolve task")
	adapter.markUserQuestionPending(sessionID, runID, userQuestionEntry{
		RequestID:   "ask-resolve-1",
		QuestionID:  "q-resolve-1",
		Title:       "选择分支",
		Kind:        "single_choice",
		AllowSkip:   true,
		Options:     []UserQuestionCardOption{{Label: "main"}},
		Description: "请选择目标分支",
	})
	adapter.mu.Lock()
	adapter.userQuestionCards["ask-resolve-1"] = "ask-card-1"
	adapter.mu.Unlock()

	if err := adapter.HandleCardAction(context.Background(), FeishuCardActionEvent{
		ActionType: "user_question",
		RequestID:  "ask-resolve-1",
		Status:     "answered",
		Values:     []string{"main"},
	}); err != nil {
		t.Fatalf("handle card action: %v", err)
	}

	if adapterTestGateway(adapter).resolveCount != 1 {
		t.Fatalf("resolve count = %d, want 1", adapterTestGateway(adapter).resolveCount)
	}
	msgs := adapterTestMessenger(adapter).snapshot()
	foundUpdate := false
	for _, message := range msgs {
		if message.kind == "update_ask_card" && message.resolvedUserQuestion != nil &&
			message.resolvedUserQuestion.RequestID == "ask-resolve-1" {
			foundUpdate = true
			break
		}
	}
	if !foundUpdate {
		t.Fatalf("expected ask card update, got %#v", msgs)
	}
}

func TestRunTerminalEventUntracksActiveRun(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.consumeGatewayEvents(ctx)

	sessionID := BuildSessionID("chat-cleanup")
	runID := BuildRunID("msg-cleanup")
	adapter.trackSession(sessionID, runID, "chat-cleanup", "chat-cleanup task")

	pushGatewayEvent(t, adapterTestGateway(adapter), sessionID, runID, "run_done", map[string]any{
		"runtime_event_type": "agent_done",
		"payload": map[string]any{
			"content": "done",
		},
	})
	time.Sleep(30 * time.Millisecond)

	adapter.mu.RLock()
	_, exists := adapter.activeRuns[runBindingKey(sessionID, runID)]
	adapter.mu.RUnlock()
	if exists {
		t.Fatalf("expected run binding cleaned after terminal event")
	}
}

func TestRefreshActiveCardsDoesNotForceFailStalledRun(t *testing.T) {
	adapter := newTestAdapter(t)
	base := time.Now().UTC()
	adapter.nowFn = func() time.Time { return base }

	sessionID := BuildSessionID("chat-stalled")
	runID := BuildRunID("msg-stalled")
	adapter.trackSession(sessionID, runID, "chat-stalled", "stalled task")
	if err := adapter.ensureRunCard(context.Background(), sessionID, runID); err != nil {
		t.Fatalf("ensureRunCard: %v", err)
	}

	adapter.nowFn = func() time.Time { return base.Add(defaultRunStallTimeout + 5*time.Second) }
	adapter.refreshActiveCards(context.Background())

	adapter.mu.RLock()
	binding, exists := adapter.activeRuns[runBindingKey(sessionID, runID)]
	adapter.mu.RUnlock()
	if !exists {
		t.Fatalf("expected stalled run to stay tracked")
	}
	if strings.TrimSpace(strings.ToLower(binding.Result)) != "pending" {
		t.Fatalf("result = %q, want pending", binding.Result)
	}
}

func TestPermissionRequestedAfterRunTerminalDoesNotReopenApprovalCard(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.consumeGatewayEvents(ctx)

	sessionID := BuildSessionID("chat-stale-perm")
	runID := BuildRunID("msg-stale-perm")
	chatID := "chat-stale-perm"
	adapter.trackSession(sessionID, runID, chatID, "stale permission task")

	pushGatewayEvent(t, adapterTestGateway(adapter), sessionID, runID, "run_done", map[string]any{
		"runtime_event_type": "agent_done",
		"payload": map[string]any{
			"content": "done",
		},
	})
	time.Sleep(20 * time.Millisecond)

	// run 已终态且绑定已清理时，乱序/重复 permission_requested 不应重新弹出审批卡片。
	pushGatewayEvent(t, adapterTestGateway(adapter), sessionID, runID, "run_progress", map[string]any{
		"runtime_event_type": "permission_requested",
		"payload": map[string]any{
			"request_id": "perm-stale-1",
			"reason":     "旧事件回放",
		},
	})
	time.Sleep(30 * time.Millisecond)

	msgs := adapterTestMessenger(adapter).snapshot()
	for _, message := range msgs {
		if message.kind == "card" && message.card.RequestID == "perm-stale-1" {
			t.Fatalf("unexpected stale permission card after terminal run: %#v", msgs)
		}
	}
}

func TestRunDonePrefersAssistantTextForUserFacingReply(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.consumeGatewayEvents(ctx)

	sessionID := BuildSessionID("chat-done-text")
	runID := BuildRunID("msg-done-text")
	adapter.trackSession(sessionID, runID, "chat-done-text", "chat-done-text task")
	_ = adapter.ensureRunCard(context.Background(), sessionID, runID)

	pushGatewayEvent(t, adapterTestGateway(adapter), sessionID, runID, "run_done", map[string]any{
		"runtime_event_type": "agent_done",
		"payload": map[string]any{
			"parts": []map[string]any{
				{"type": "text", "text": "这是最终回复"},
			},
		},
	})
	time.Sleep(30 * time.Millisecond)

	msgs := adapterTestMessenger(adapter).snapshot()
	if len(msgs) == 0 {
		t.Fatalf("expected at least one message")
	}
	last := msgs[len(msgs)-1]
	if last.kind != "update_card" || !strings.Contains(last.runCard.Summary, "这是最终回复") {
		t.Fatalf("expected card update with summary text, got %#v", last)
	}
}

func TestRunProgressInternalEventsAreNotUserFacing(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go adapter.consumeGatewayEvents(ctx)

	sessionID := BuildSessionID("chat-throttle")
	runID := BuildRunID("msg-throttle")
	adapter.trackSession(sessionID, runID, "chat-throttle", "chat-throttle task")

	pushGatewayEvent(t, adapterTestGateway(adapter), sessionID, runID, "run_progress", map[string]any{
		"runtime_event_type": "agent_chunk",
	})
	pushGatewayEvent(t, adapterTestGateway(adapter), sessionID, runID, "run_progress", map[string]any{
		"runtime_event_type": "agent_chunk",
	})
	time.Sleep(30 * time.Millisecond)

	textCount := 0
	for _, message := range adapterTestMessenger(adapter).snapshot() {
		if message.kind == "text" && strings.Contains(message.text, "运行进度") {
			textCount++
		}
	}
	if textCount != 0 {
		t.Fatalf("progress message count = %d, want 0", textCount)
	}
}

func TestCardCallbackDedupeResolveOnce(t *testing.T) {
	adapter := newTestAdapter(t)
	adapter.trackSession("session-card-dedupe", "run-card-dedupe", "chat-card-dedupe", "card dedupe task")
	adapter.processPermissionRequested(
		context.Background(),
		"session-card-dedupe",
		"run-card-dedupe",
		"chat-card-dedupe",
		"perm-2",
		"filesystem_write_file",
		"write_file",
		"dedupe.txt",
		"需要审批",
	)
	body := `{"action":{"value":{"request_id":"perm-2","decision":"allow_once"}},"token":"verify"}`
	for i := 0; i < 2; i++ {
		request := signedRequest(t, adapter.cfg.SigningSecret, body)
		recorder := httptest.NewRecorder()
		adapter.handleCardCallback(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", recorder.Code)
		}
	}
	if adapterTestGateway(adapter).resolveCount != 1 {
		t.Fatalf("resolve count = %d, want 1", adapterTestGateway(adapter).resolveCount)
	}
}

func TestCardCallbackResolveFailureReturns500(t *testing.T) {
	adapter := newTestAdapter(t)
	adapter.trackSession("session-card-failure", "run-card-failure", "chat-card-failure", "card failure task")
	adapter.processPermissionRequested(
		context.Background(),
		"session-card-failure",
		"run-card-failure",
		"chat-card-failure",
		"perm-3",
		"filesystem_write_file",
		"write_file",
		"failure.txt",
		"需要审批",
	)
	gateway := adapterTestGateway(adapter)
	gateway.mu.Lock()
	gateway.resolveErr = assertErr("deny")
	gateway.mu.Unlock()

	body := `{"action":{"value":{"request_id":"perm-3","decision":"reject"}},"token":"verify"}`
	request := signedRequest(t, adapter.cfg.SigningSecret, body)
	recorder := httptest.NewRecorder()
	adapter.handleCardCallback(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
}

func TestCardCallbackUserQuestionAnswerAccepted(t *testing.T) {
	adapter := newTestAdapter(t)
	sessionID := BuildSessionID("chat-callback-ask")
	runID := BuildRunID("msg-callback-ask")
	adapter.trackSession(sessionID, runID, "chat-callback-ask", "ask callback task")
	adapter.markUserQuestionPending(sessionID, runID, userQuestionEntry{
		RequestID:  "ask-callback-1",
		QuestionID: "q-callback-1",
		Title:      "选择模式",
		Kind:       "single_choice",
		Options:    []UserQuestionCardOption{{Label: "快速"}},
	})

	body := `{"action":{"value":{"action_type":"user_question","request_id":"ask-callback-1","status":"answered","value":"快速"}},"token":"verify"}`
	request := signedRequest(t, adapter.cfg.SigningSecret, body)
	recorder := httptest.NewRecorder()
	adapter.handleCardCallback(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if adapterTestGateway(adapter).resolveCount != 1 {
		t.Fatalf("resolve count = %d, want 1", adapterTestGateway(adapter).resolveCount)
	}
	if !strings.Contains(recorder.Body.String(), "回答已提交") {
		t.Fatalf("response = %s, want ask toast", recorder.Body.String())
	}
}

func TestCardCallbackUrlVerificationAccepted(t *testing.T) {
	adapter := newTestAdapter(t)
	body := `{"type":"url_verification","challenge":"card-challenge","token":"verify","header":{"token":"verify"}}`
	request := signedRequest(t, adapter.cfg.SigningSecret, body)
	recorder := httptest.NewRecorder()
	adapter.handleCardCallback(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"challenge":"card-challenge"`) {
		t.Fatalf("response = %s, want challenge", recorder.Body.String())
	}
}

func TestHandleCardCallbackValidationFailures(t *testing.T) {
	adapter := newTestAdapter(t)
	testCases := []struct {
		name string
		body string
		want int
	}{
		{name: "invalid token", body: `{"token":"bad","header":{"token":"bad"}}`, want: http.StatusUnauthorized},
		{name: "invalid json", body: `{`, want: http.StatusBadRequest},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			request := signedRequest(t, adapter.cfg.SigningSecret, testCase.body)
			recorder := httptest.NewRecorder()
			adapter.handleCardCallback(recorder, request)
			if recorder.Code != testCase.want {
				t.Fatalf("status = %d, want %d", recorder.Code, testCase.want)
			}
		})
	}
}

func TestCardCallbackProbeWithoutActionReturnsOK(t *testing.T) {
	adapter := newTestAdapter(t)
	body := `{"token":"verify","header":{"token":"verify"}}`
	request := signedRequest(t, adapter.cfg.SigningSecret, body)
	recorder := httptest.NewRecorder()
	adapter.handleCardCallback(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if adapterTestGateway(adapter).resolveCount != 0 {
		t.Fatalf("resolve count = %d, want 0", adapterTestGateway(adapter).resolveCount)
	}
}

func TestCardCallbackInvalidActionPayloadReturnsInfoWithoutResolve(t *testing.T) {
	adapter := newTestAdapter(t)
	body := `{"action":{"value":{"action_type":"permission","request_id":"perm-x","decision":"allow_all"}},"token":"verify","header":{"token":"verify"}}`
	request := signedRequest(t, adapter.cfg.SigningSecret, body)
	recorder := httptest.NewRecorder()
	adapter.handleCardCallback(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "callback ready") {
		t.Fatalf("response = %s, want callback ready", recorder.Body.String())
	}
	if adapterTestGateway(adapter).resolveCount != 0 {
		t.Fatalf("resolve count = %d, want 0", adapterTestGateway(adapter).resolveCount)
	}
}

func TestReconnectRebindActiveSessions(t *testing.T) {
	adapter := newTestAdapter(t)
	gw := adapterTestGateway(adapter)
	gw.pingErr = assertErr("dial failed")
	adapter.trackSession("session-a", "run-a", "chat-a", "task-a")

	ctx, cancel := context.WithCancel(context.Background())
	go adapter.reconnectAndRebindLoop(ctx)
	time.Sleep(30 * time.Millisecond)
	gw.mu.Lock()
	gw.pingErr = nil
	gw.mu.Unlock()
	waitForCalls(t, gw, func(calls string) bool {
		return strings.Contains(calls, "bind:session-a:run-a")
	})
	cancel()
	time.Sleep(20 * time.Millisecond)

	calls := strings.Join(gw.snapshotCalls(), "|")
	if !strings.Contains(calls, "bind:session-a:run-a") {
		t.Fatalf("expected rebind call in %v", calls)
	}
}

func TestReconnectRebindTracksMultipleRunsPerSession(t *testing.T) {
	adapter := newTestAdapter(t)
	gw := adapterTestGateway(adapter)
	gw.pingErr = assertErr("dial failed")
	adapter.trackSession("session-x", "run-a", "chat-x", "task-a")
	adapter.trackSession("session-x", "run-b", "chat-x", "task-b")

	ctx, cancel := context.WithCancel(context.Background())
	go adapter.reconnectAndRebindLoop(ctx)
	time.Sleep(30 * time.Millisecond)
	gw.mu.Lock()
	gw.pingErr = nil
	gw.mu.Unlock()
	waitForCalls(t, gw, func(calls string) bool {
		return strings.Contains(calls, "bind:session-x:run-a") &&
			strings.Contains(calls, "bind:session-x:run-b")
	})
	cancel()
	time.Sleep(20 * time.Millisecond)

	calls := strings.Join(gw.snapshotCalls(), "|")
	if !strings.Contains(calls, "bind:session-x:run-a") {
		t.Fatalf("expected run-a rebind call in %v", calls)
	}
	if !strings.Contains(calls, "bind:session-x:run-b") {
		t.Fatalf("expected run-b rebind call in %v", calls)
	}
}

func TestReconnectHealthyPathDoesNotRebind(t *testing.T) {
	adapter := newTestAdapter(t)
	gw := adapterTestGateway(adapter)
	adapter.trackSession("session-steady", "run-steady", "chat-steady", "steady")

	ctx, cancel := context.WithCancel(context.Background())
	go adapter.reconnectAndRebindLoop(ctx)
	time.Sleep(80 * time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)

	calls := strings.Join(gw.snapshotCalls(), "|")
	if strings.Contains(calls, "bind:session-steady:run-steady") {
		t.Fatalf("did not expect steady-state rebind call in %v", calls)
	}
}

func TestRetryAuthenticateAndRebindHandlesAuthFailure(t *testing.T) {
	adapter := newTestAdapter(t)
	gateway := adapterTestGateway(adapter)
	gateway.mu.Lock()
	gateway.authErr = assertErr("re-auth failed")
	gateway.mu.Unlock()
	adapter.trackSession("session-auth-fail", "run-auth-fail", "chat-auth-fail", "task")

	if ok := adapter.retryAuthenticateAndRebind(context.Background(), time.Millisecond); !ok {
		t.Fatal("expected retry loop to continue after auth failure")
	}
	calls := strings.Join(gateway.snapshotCalls(), "|")
	if strings.Contains(calls, "bind:session-auth-fail:run-auth-fail") {
		t.Fatalf("did not expect rebind after auth failure: %v", calls)
	}
}

func TestRetryAuthenticateAndRebindStopsWhenContextCanceled(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if ok := adapter.retryAuthenticateAndRebind(ctx, time.Hour); ok {
		t.Fatal("expected retry to stop when context is canceled")
	}
}

func TestBuildIngressSelectsConfiguredMode(t *testing.T) {
	adapter := newTestAdapter(t)
	if _, ok := adapter.buildIngress().(*WebhookIngress); !ok {
		t.Fatalf("expected webhook ingress by default, got %T", adapter.buildIngress())
	}

	adapter.cfg.IngressMode = IngressModeSDK
	if _, ok := adapter.buildIngress().(*SDKIngress); !ok {
		t.Fatalf("expected sdk ingress, got %T", adapter.buildIngress())
	}
}

func TestRefreshActiveCardsUpdatesExistingCard(t *testing.T) {
	adapter := newTestAdapter(t)
	sessionID := BuildSessionID("chat-refresh")
	runID := BuildRunID("msg-refresh")
	adapter.trackSession(sessionID, runID, "chat-refresh", "refresh task")

	adapter.mu.Lock()
	binding := adapter.activeRuns[runBindingKey(sessionID, runID)]
	binding.CardID = "card-refresh"
	binding.RunStartTime = time.Now().Add(-3 * time.Second)
	adapter.activeRuns[runBindingKey(sessionID, runID)] = binding
	adapter.mu.Unlock()

	adapter.refreshActiveCards(context.Background())

	msgs := adapterTestMessenger(adapter).snapshot()
	if len(msgs) != 1 || msgs[0].kind != "update_card" || msgs[0].cardID != "card-refresh" {
		t.Fatalf("unexpected refresh updates: %#v", msgs)
	}
	if msgs[0].runCard.Elapsed == "" {
		t.Fatalf("expected elapsed time in payload: %#v", msgs[0].runCard)
	}
}

func TestRefreshActiveCardsSkipsBindingsWithoutCardID(t *testing.T) {
	adapter := newTestAdapter(t)
	sessionID := BuildSessionID("chat-no-card")
	runID := BuildRunID("msg-no-card")
	adapter.trackSession(sessionID, runID, "chat-no-card", "no card task")

	adapter.refreshActiveCards(context.Background())

	if msgs := adapterTestMessenger(adapter).snapshot(); len(msgs) != 0 {
		t.Fatalf("expected no card refresh messages, got %#v", msgs)
	}
}

func TestEnsureRunCardUpdatesExistingCard(t *testing.T) {
	adapter := newTestAdapter(t)
	sessionID := BuildSessionID("chat-existing-card")
	runID := BuildRunID("msg-existing-card")
	adapter.trackSession(sessionID, runID, "chat-existing-card", "existing task")

	adapter.mu.Lock()
	binding := adapter.activeRuns[runBindingKey(sessionID, runID)]
	binding.CardID = "card-existing"
	adapter.activeRuns[runBindingKey(sessionID, runID)] = binding
	adapter.mu.Unlock()

	if err := adapter.ensureRunCard(context.Background(), sessionID, runID); err != nil {
		t.Fatalf("ensure existing run card: %v", err)
	}

	msgs := adapterTestMessenger(adapter).snapshot()
	if len(msgs) != 1 || msgs[0].kind != "update_card" || msgs[0].cardID != "card-existing" {
		t.Fatalf("unexpected card update: %#v", msgs)
	}
}

func TestTryHandleTextPermissionHandlesApprovalCommands(t *testing.T) {
	adapter := newTestAdapter(t)
	adapter.trackSession("session-approve", "run-approve", "chat-approve", "approve task")
	adapter.processPermissionRequested(
		context.Background(),
		"session-approve",
		"run-approve",
		"chat-approve",
		"perm-approve",
		"filesystem_write_file",
		"write_file",
		"approve.txt",
		"需要审批",
	)

	handled, err := adapter.tryHandleTextPermission(context.Background(), "chat-approve", "允许 perm-approve")
	if err != nil || !handled {
		t.Fatalf("allow command = handled:%v err:%v", handled, err)
	}
	msgs := adapterTestMessenger(adapter).snapshot()
	if len(msgs) == 0 || msgs[len(msgs)-1].text != "审批已提交：允许一次。" {
		t.Fatalf("unexpected approval reply: %#v", msgs)
	}
	if adapterTestGateway(adapter).resolveCount != 1 {
		t.Fatalf("resolve count = %d, want 1", adapterTestGateway(adapter).resolveCount)
	}
}

func TestTryHandleTextPermissionHandlesRejectCommand(t *testing.T) {
	adapter := newTestAdapter(t)
	adapter.trackSession("session-reject-ok", "run-reject-ok", "chat-reject-ok", "reject task")
	adapter.processPermissionRequested(
		context.Background(),
		"session-reject-ok",
		"run-reject-ok",
		"chat-reject-ok",
		"perm-reject-ok",
		"filesystem_write_file",
		"write_file",
		"reject-ok.txt",
		"需要审批",
	)

	handled, err := adapter.tryHandleTextPermission(context.Background(), "chat-reject-ok", "拒绝 perm-reject-ok")
	if err != nil || !handled {
		t.Fatalf("reject command = handled:%v err:%v", handled, err)
	}
	msgs := adapterTestMessenger(adapter).snapshot()
	if len(msgs) == 0 || msgs[len(msgs)-1].text != "审批已提交：拒绝。" {
		t.Fatalf("unexpected reject reply: %#v", msgs)
	}
}

func TestTryHandleTextPermissionRepliesIgnoredWhenRequestNotActive(t *testing.T) {
	adapter := newTestAdapter(t)
	adapter.trackSession("session-text-ignored", "run-text-ignored", "chat-text-ignored", "text ignored task")
	adapter.processPermissionRequested(
		context.Background(),
		"session-text-ignored",
		"run-text-ignored",
		"chat-text-ignored",
		"perm-text-ignored",
		"filesystem_write_file",
		"write_file",
		"text-ignored.txt",
		"需要审批",
	)
	if err := adapter.HandleCardAction(context.Background(), FeishuCardActionEvent{
		ActionType: "permission",
		RequestID:  "perm-text-ignored",
		Decision:   "allow_once",
	}); err != nil {
		t.Fatalf("resolve permission before text retry: %v", err)
	}

	handled, err := adapter.tryHandleTextPermission(context.Background(), "chat-text-ignored", "允许 perm-text-ignored")
	if err != nil || !handled {
		t.Fatalf("allow stale command = handled:%v err:%v", handled, err)
	}
	msgs := adapterTestMessenger(adapter).snapshot()
	if len(msgs) == 0 || msgs[len(msgs)-1].text != "审批未命中当前待处理请求，已忽略。" {
		t.Fatalf("unexpected stale allow reply: %#v", msgs)
	}
	if adapterTestGateway(adapter).resolveCount != 1 {
		t.Fatalf("resolve count = %d, want 1", adapterTestGateway(adapter).resolveCount)
	}
}

func TestTryHandleTextPermissionRejectFailureRepliesRetryable(t *testing.T) {
	adapter := newTestAdapter(t)
	adapter.trackSession("session-reject", "run-reject", "chat-reject", "reject task")
	adapter.processPermissionRequested(
		context.Background(),
		"session-reject",
		"run-reject",
		"chat-reject",
		"perm-reject",
		"filesystem_write_file",
		"write_file",
		"reject.txt",
		"需要审批",
	)
	gateway := adapterTestGateway(adapter)
	gateway.mu.Lock()
	gateway.resolveErr = assertErr("boom")
	gateway.mu.Unlock()

	handled, err := adapter.tryHandleTextPermission(context.Background(), "chat-reject", "拒绝 perm-reject")
	if err == nil || !handled {
		t.Fatalf("reject command = handled:%v err:%v", handled, err)
	}
	msgs := adapterTestMessenger(adapter).snapshot()
	if len(msgs) == 0 || msgs[len(msgs)-1].text != "审批提交失败，请稍后重试。" {
		t.Fatalf("unexpected failure reply: %#v", msgs)
	}
}

func TestApprovalOutboxPreflightDropsStaleOperation(t *testing.T) {
	adapter := newTestAdapter(t)
	adapter.trackSession("session-outbox-stale", "run-outbox-stale", "chat-outbox-stale", "outbox stale task")
	adapter.processPermissionRequested(
		context.Background(),
		"session-outbox-stale",
		"run-outbox-stale",
		"chat-outbox-stale",
		"perm-outbox-stale",
		"filesystem_write_file",
		"write_file",
		"outbox-stale.txt",
		"需要审批",
	)

	runKey := runBindingKey("session-outbox-stale", "run-outbox-stale")
	adapter.mu.RLock()
	fsm := adapter.approvalFSMByRun[runKey]
	if fsm == nil {
		adapter.mu.RUnlock()
		t.Fatal("expected approval fsm for stale outbox test")
	}
	cardID := strings.TrimSpace(fsm.CardID)
	generation := fsm.Generation
	staleVersion := fsm.Version - 1
	adapter.mu.RUnlock()
	if cardID == "" {
		t.Fatal("expected permission card id for stale outbox test")
	}

	before := len(adapterTestMessenger(adapter).snapshot())
	adapter.executeApprovalOutbox(context.Background(), []approvalOutboxOperation{
		{
			RunKey:     runKey,
			Generation: generation,
			Version:    staleVersion,
			Kind:       approvalOutboxUpdatePendingCard,
			CardID:     cardID,
			RequestID:  "perm-outbox-stale",
			PendingCard: PermissionCardPayload{
				RequestID: "perm-outbox-stale",
				ToolName:  "filesystem_write_file",
				Operation: "write_file",
				Target:    "outbox-stale.txt",
				Message:   "stale should drop",
			},
		},
	})
	after := len(adapterTestMessenger(adapter).snapshot())
	if after != before {
		t.Fatalf("stale outbox should be dropped before send, before=%d after=%d", before, after)
	}
}

func TestApprovalOutboxSendCardCleanupWhenVersionAdvancedDuringSend(t *testing.T) {
	adapter := newTestAdapter(t)
	sessionID := "session-outbox-race-send"
	runID := "run-outbox-race-send"
	runKey := runBindingKey(sessionID, runID)
	adapter.trackSession(sessionID, runID, "chat-outbox-race-send", "outbox race send task")

	var createdCardID string
	var hookOnce sync.Once
	messenger := adapterTestMessenger(adapter)
	messenger.mu.Lock()
	messenger.sendPermissionCardHook = func(cardID string, _ PermissionCardPayload) {
		createdCardID = strings.TrimSpace(cardID)
		hookOnce.Do(func() {
			adapter.mu.Lock()
			if fsm := adapter.approvalFSMByRun[runKey]; fsm != nil {
				fsm.Version++
			}
			adapter.mu.Unlock()
		})
	}
	messenger.mu.Unlock()

	adapter.processPermissionRequested(
		context.Background(),
		sessionID,
		runID,
		"chat-outbox-race-send",
		"perm-outbox-race-send",
		"filesystem_write_file",
		"write_file",
		"outbox-race-send.txt",
		"需要审批",
	)
	if strings.TrimSpace(createdCardID) == "" {
		t.Fatal("expected permission card to be sent")
	}

	msgs := adapterTestMessenger(adapter).snapshot()
	foundDelete := false
	for _, message := range msgs {
		if message.kind == "delete_card" && message.chatID == createdCardID {
			foundDelete = true
			break
		}
	}
	if !foundDelete {
		t.Fatalf("expected stale sent permission card to be deleted, msgs=%#v", msgs)
	}

	adapter.mu.RLock()
	fsm := adapter.approvalFSMByRun[runKey]
	storedCardID := ""
	if fsm != nil {
		storedCardID = strings.TrimSpace(fsm.CardID)
	}
	_, indexed := adapter.approvalCardRunIndex[createdCardID]
	adapter.mu.RUnlock()
	if storedCardID != "" {
		t.Fatalf("stale card should not be attached to fsm, got %q", storedCardID)
	}
	if indexed {
		t.Fatalf("stale card should not remain in approvalCardRunIndex: %s", createdCardID)
	}
}

func TestTryHandleTextPermissionHandlesAskUserAnswerAndSkip(t *testing.T) {
	adapter := newTestAdapter(t)
	sessionID := BuildSessionID("chat-ask-text-cmd")
	runID := BuildRunID("msg-ask-text-cmd")
	adapter.trackSession(sessionID, runID, "chat-ask-text-cmd", "ask cmd task")
	adapter.markUserQuestionPending(sessionID, runID, userQuestionEntry{
		RequestID:  "ask-cmd-1",
		QuestionID: "q-cmd-1",
		Title:      "选择发布环境",
		Kind:       "single_choice",
		Options: []UserQuestionCardOption{
			{Label: "测试环境"},
			{Label: "生产环境"},
		},
		AllowSkip: true,
	})

	handled, err := adapter.tryHandleTextPermission(context.Background(), "chat-ask-text-cmd", "回答 ask-cmd-1 测试环境")
	if err != nil || !handled {
		t.Fatalf("answer command = handled:%v err:%v", handled, err)
	}
	handled, err = adapter.tryHandleTextPermission(context.Background(), "chat-ask-text-cmd", "跳过 ask-cmd-1")
	if err != nil || !handled {
		t.Fatalf("skip command = handled:%v err:%v", handled, err)
	}
	if adapterTestGateway(adapter).resolveCount < 2 {
		t.Fatalf("resolve count = %d, want >=2", adapterTestGateway(adapter).resolveCount)
	}
}

func TestTryHandleTextPermissionRejectsInvalidChoiceAnswer(t *testing.T) {
	adapter := newTestAdapter(t)
	sessionID := BuildSessionID("chat-ask-invalid-choice")
	runID := BuildRunID("msg-ask-invalid-choice")
	adapter.trackSession(sessionID, runID, "chat-ask-invalid-choice", "ask invalid choice task")
	adapter.markUserQuestionPending(sessionID, runID, userQuestionEntry{
		RequestID:  "ask-invalid-choice-1",
		QuestionID: "q-invalid-choice-1",
		Title:      "选择发布环境",
		Kind:       "single_choice",
		Options: []UserQuestionCardOption{
			{Label: "测试环境"},
			{Label: "生产环境"},
		},
		AllowSkip: true,
	})

	handled, err := adapter.tryHandleTextPermission(context.Background(), "chat-ask-invalid-choice", "回答 ask-invalid-choice-1 staging")
	if err != nil || !handled {
		t.Fatalf("invalid single_choice answer = handled:%v err:%v", handled, err)
	}
	if adapterTestGateway(adapter).resolveCount != 0 {
		t.Fatalf("resolve count = %d, want 0 for invalid answer", adapterTestGateway(adapter).resolveCount)
	}
	msgs := adapterTestMessenger(adapter).snapshot()
	if len(msgs) == 0 || msgs[len(msgs)-1].text != "回答格式无效，请使用：回答 <request_id> <内容>" {
		t.Fatalf("unexpected invalid answer reply: %#v", msgs)
	}
}

func TestTryHandleTextPermissionRejectsMultiChoiceExceedingMaxChoices(t *testing.T) {
	adapter := newTestAdapter(t)
	sessionID := BuildSessionID("chat-ask-max-choice")
	runID := BuildRunID("msg-ask-max-choice")
	adapter.trackSession(sessionID, runID, "chat-ask-max-choice", "ask max choice task")
	adapter.markUserQuestionPending(sessionID, runID, userQuestionEntry{
		RequestID:  "ask-max-choice-1",
		QuestionID: "q-max-choice-1",
		Title:      "选择要发布的区域",
		Kind:       "multi_choice",
		Options: []UserQuestionCardOption{
			{Label: "华北"},
			{Label: "华东"},
			{Label: "华南"},
		},
		MaxChoices: 2,
		AllowSkip:  true,
	})

	handled, err := adapter.tryHandleTextPermission(context.Background(), "chat-ask-max-choice", "回答 ask-max-choice-1 华北,华东,华南")
	if err != nil || !handled {
		t.Fatalf("max_choices exceed answer = handled:%v err:%v", handled, err)
	}
	if adapterTestGateway(adapter).resolveCount != 0 {
		t.Fatalf("resolve count = %d, want 0 when max_choices exceeded", adapterTestGateway(adapter).resolveCount)
	}
	msgs := adapterTestMessenger(adapter).snapshot()
	if len(msgs) == 0 || msgs[len(msgs)-1].text != "回答格式无效，请使用：回答 <request_id> <内容>" {
		t.Fatalf("unexpected max_choices reply: %#v", msgs)
	}
}

func TestTryHandleTextPermissionHandlesEmptyAndUnknownCommands(t *testing.T) {
	adapter := newTestAdapter(t)
	for _, testCase := range []struct {
		text        string
		wantHandled bool
	}{
		{text: "", wantHandled: false},
		{text: "允许", wantHandled: false},
		{text: "拒绝", wantHandled: false},
		{text: "hello", wantHandled: false},
	} {
		handled, err := adapter.tryHandleTextPermission(context.Background(), "chat", testCase.text)
		if err != nil {
			t.Fatalf("text %q error: %v", testCase.text, err)
		}
		if handled != testCase.wantHandled {
			t.Fatalf("text %q handled=%v, want %v", testCase.text, handled, testCase.wantHandled)
		}
	}
}

func TestBindThenRunCoversFailureAndFallbackPaths(t *testing.T) {
	t.Run("authenticate failure", func(t *testing.T) {
		adapter := newTestAdapter(t)
		gateway := adapterTestGateway(adapter)
		gateway.mu.Lock()
		gateway.authErr = assertErr("auth failed")
		gateway.mu.Unlock()
		if err := adapter.bindThenRun(context.Background(), "session-auth", "run-auth", "chat-auth", "task"); err == nil {
			t.Fatal("expected authenticate failure")
		}
	})

	t.Run("bind failure", func(t *testing.T) {
		adapter := newTestAdapter(t)
		gateway := adapterTestGateway(adapter)
		gateway.mu.Lock()
		gateway.bindErr = assertErr("bind failed")
		gateway.mu.Unlock()
		if err := adapter.bindThenRun(context.Background(), "session-bind", "run-bind", "chat-bind", "task"); err == nil {
			t.Fatal("expected bind failure")
		}
	})

	t.Run("status card fallback text", func(t *testing.T) {
		adapter := newTestAdapter(t)
		messenger := adapterTestMessenger(adapter)
		messenger.mu.Lock()
		messenger.sendCardErr = assertErr("card failed")
		messenger.mu.Unlock()
		if err := adapter.bindThenRun(context.Background(), "session-card", "run-card", "chat-card", "task"); err != nil {
			t.Fatalf("bind then run: %v", err)
		}
		msgs := messenger.snapshot()
		if len(msgs) == 0 || msgs[len(msgs)-1].text != "任务已受理，正在执行。" {
			t.Fatalf("unexpected fallback messages: %#v", msgs)
		}
	})
}

func TestReadAndVerifyRequestRejectsNonPost(t *testing.T) {
	ingress := &WebhookIngress{
		cfg: Config{
			SigningSecret:  "sign-secret",
			IngressMode:    IngressModeWebhook,
			RequestTimeout: 200 * time.Millisecond,
			IdempotencyTTL: 2 * time.Minute,
		},
	}
	request := httptest.NewRequest(http.MethodGet, "/feishu/events", nil)
	recorder := httptest.NewRecorder()
	if body, ok := ingress.readAndVerifyRequest(recorder, request); ok || body != nil {
		t.Fatalf("expected non-post request rejection, body=%q ok=%v", string(body), ok)
	}
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
}

func TestNewWebhookIngressProvidesDefaultClockAndTokenValidation(t *testing.T) {
	ingress, ok := NewWebhookIngress(Config{VerifyToken: "verify"}, nil).(*WebhookIngress)
	if !ok {
		t.Fatalf("expected webhook ingress, got %T", ingress)
	}
	if ingress.nowFn == nil {
		t.Fatal("expected default clock")
	}
	if !ingress.verifyCallbackToken("", "verify") {
		t.Fatal("expected header token to be accepted")
	}
	ingress.cfg.VerifyToken = ""
	if ingress.verifyCallbackToken("verify", "verify") {
		t.Fatal("expected empty configured token to reject callback")
	}
}

func TestFormatElapsedCoversRanges(t *testing.T) {
	if got := formatElapsed(time.Time{}); got != "" {
		t.Fatalf("zero elapsed = %q, want empty", got)
	}
	if got := formatElapsed(time.Now().Add(-500 * time.Millisecond)); got != "刚刚开始" {
		t.Fatalf("sub-second elapsed = %q", got)
	}
	if got := formatElapsed(time.Now().Add(-5 * time.Second)); got != "5s" {
		t.Fatalf("seconds elapsed = %q", got)
	}
	if got := formatElapsed(time.Now().Add(-65 * time.Second)); got != "1m 5s" {
		t.Fatalf("minutes elapsed = %q", got)
	}
	if got := formatElapsed(time.Now().Add(-(time.Hour + 2*time.Minute + 3*time.Second))); got != "1h 2m 3s" {
		t.Fatalf("hours elapsed = %q", got)
	}
}

func TestBuildTaskNameTruncatesLongFirstLine(t *testing.T) {
	text := "12345678901234567890123456789012345678901\nsecond line"
	if got := buildTaskName(text); got != "1234567890123456789012345678901234567890..." {
		t.Fatalf("task name = %q", got)
	}
}

func TestExtractHookNotificationSummaryAndHintFallbacks(t *testing.T) {
	if summary := extractHookNotificationSummary(nil); summary != "" {
		t.Fatalf("summary = %q, want empty", summary)
	}
	if summary := extractHookNotificationSummary(map[string]any{
		"payload": map[string]any{"summary": "summary"},
	}); summary != "summary" {
		t.Fatalf("summary = %q, want summary", summary)
	}
	if summary := extractHookNotificationSummary(map[string]any{
		"payload": map[string]any{"notification": "notify"},
	}); summary != "notify" {
		t.Fatalf("summary = %q, want notify", summary)
	}
	if summary := extractHookNotificationSummary(map[string]any{
		"payload": map[string]any{"message": "message"},
	}); summary != "message" {
		t.Fatalf("summary = %q, want message", summary)
	}
	if hint := extractHookNotificationHint(map[string]any{
		"payload": map[string]any{"reason": "retry"},
	}); hint != "retry" {
		t.Fatalf("hint = %q, want retry", hint)
	}
	if hint := extractHookNotificationHint(map[string]any{
		"payload": map[string]any{"status": "async"},
	}); hint != "async" {
		t.Fatalf("hint = %q, want async", hint)
	}
	if hint := extractHookNotificationHint(nil); hint != "" {
		t.Fatalf("hint = %q, want empty", hint)
	}
}

func TestDeriveRunStatusAdditionalBranches(t *testing.T) {
	if status := deriveRunStatus("phase_changed", map[string]any{
		"payload": map[string]any{"to": "execute"},
	}, "thinking"); status != "running" {
		t.Fatalf("status = %q, want running", status)
	}
	if status := deriveRunStatus("hook_notification", map[string]any{}, "planning"); status != "running" {
		t.Fatalf("status = %q, want running", status)
	}
	if status := deriveRunStatus("unknown", map[string]any{}, ""); status != "thinking" {
		t.Fatalf("status = %q, want thinking", status)
	}
}

func TestIsMentionCurrentBotMatchesContentMarkupAndOpenID(t *testing.T) {
	cfg := Config{BotUserID: "ou_bot", BotOpenID: "ou_open_bot"}
	if !isMentionCurrentBot(FeishuMessageEvent{
		ChatType:    "group",
		ContentText: `<at user_id="ou_bot"></at> hi`,
	}, cfg) {
		t.Fatal("expected content markup to match bot user id")
	}
	if !isMentionCurrentBot(FeishuMessageEvent{
		ChatType: "group",
		Mentions: []FeishuMention{{OpenID: "ou_open_bot"}},
	}, cfg) {
		t.Fatal("expected mention open id to match bot")
	}
}

func TestExtractUserVisibleDoneTextHandlesTextFieldAndTypedParts(t *testing.T) {
	if text := extractUserVisibleDoneText(map[string]any{
		"payload": map[string]any{"text": "plain done"},
	}); text != "plain done" {
		t.Fatalf("done text = %q, want plain done", text)
	}
	if text := extractUserVisibleDoneText(map[string]any{
		"payload": map[string]any{
			"parts": []any{
				map[string]any{"type": "image", "text": "ignore"},
				map[string]any{"type": "text", "content": "keep"},
			},
		},
	}); text != "keep" {
		t.Fatalf("parts text = %q, want keep", text)
	}
	if text := extractUserVisibleDoneText(map[string]any{
		"payload": map[string]any{
			"parts": []any{
				map[string]any{"type": "text", "text": "line one"},
				map[string]any{"type": "", "content": "line two"},
				"ignored",
			},
		},
	}); text != "line one\nline two" {
		t.Fatalf("parts text = %q, want joined lines", text)
	}
	if text := extractUserVisibleDoneText(map[string]any{
		"payload": map[string]any{"parts": []any{}},
	}); text != "" {
		t.Fatalf("done text = %q, want empty", text)
	}
}

func TestConsumeGatewayEventsIgnoresNonGatewayNotifications(t *testing.T) {
	adapter := newTestAdapter(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		adapter.consumeGatewayEvents(ctx)
		close(done)
	}()

	adapterTestGateway(adapter).notifications <- GatewayNotification{Method: "other.method", Params: json.RawMessage(`{}`)}
	cancel()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("consumeGatewayEvents did not stop after cancellation")
	}
	if msgs := adapterTestMessenger(adapter).snapshot(); len(msgs) != 0 {
		t.Fatalf("expected no messages for non-gateway event, got %#v", msgs)
	}
}

func TestReadAndVerifyRequestRejectsUnreadableBody(t *testing.T) {
	ingress := &WebhookIngress{
		cfg: Config{
			SigningSecret:  "sign-secret",
			IngressMode:    IngressModeWebhook,
			RequestTimeout: 200 * time.Millisecond,
			IdempotencyTTL: 2 * time.Minute,
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/feishu/events", errReader{})
	request.Header.Set(headerLarkTimestamp, strconvTimestamp(time.Now().UTC()))
	request.Header.Set(headerLarkNonce, "nonce")
	request.Header.Set(headerLarkSignature, "sig")
	recorder := httptest.NewRecorder()
	if _, ok := ingress.readAndVerifyRequest(recorder, request); ok {
		t.Fatal("expected unreadable body to fail")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestShouldEmitProgressThrottlesRapidDuplicates(t *testing.T) {
	adapter := newTestAdapter(t)
	now := time.Now().UTC()
	adapter.nowFn = func() time.Time { return now }
	if !adapter.shouldEmitProgress("session", "run", "agent_chunk") {
		t.Fatal("expected first progress event to emit")
	}
	if adapter.shouldEmitProgress("session", "run", "agent_chunk") {
		t.Fatal("expected duplicate progress event to be throttled")
	}
	adapter.nowFn = func() time.Time { return now.Add(defaultProgressNotifyInterval + time.Millisecond) }
	if !adapter.shouldEmitProgress("session", "run", "agent_chunk") {
		t.Fatal("expected event after interval to emit")
	}
}

func TestHelperFunctionsCoverFallbackBranches(t *testing.T) {
	if text, err := decodeMessageText(""); err != nil || text != "" {
		t.Fatalf("decode empty text = %q, %v", text, err)
	}
	if _, err := decodeMessageText("{"); err == nil {
		t.Fatal("expected invalid message content error")
	}
	requestID, toolName, operation, target, reason := extractPermissionRequest(nil)
	if requestID != "" || toolName != "" || operation != "" || target != "" || reason == "" {
		t.Fatalf("unexpected permission extraction: request=%q tool=%q op=%q target=%q reason=%q", requestID, toolName, operation, target, reason)
	}
	if text := extractUserVisibleDoneText(map[string]any{
		"payload": map[string]any{"content": "done"},
	}); text != "done" {
		t.Fatalf("done text = %q, want direct content", text)
	}
	if text := extractUserVisibleErrorText(map[string]any{
		"payload": map[string]any{"error": "boom"},
	}); text != "任务失败：boom" {
		t.Fatalf("error text = %q, want fallback error", text)
	}
	if text := extractUserVisibleErrorText(map[string]any{
		"payload": map[string]any{"error": "runner_offline"},
	}); text != "本机 Runner 未连接，请在电脑上启动 `neocode runner`" {
		t.Fatalf("runner error text = %q", text)
	}
	if text := extractUserVisibleErrorText(map[string]any{
		"payload": map[string]any{"message": "capability_denied"},
	}); text != "权限不足：当前能力令牌不允许此操作" {
		t.Fatalf("capability error text = %q", text)
	}
	if text := extractUserVisibleErrorText(map[string]any{
		"payload": map[string]any{"message": "tool_execution_failed: bash"},
	}); text != "工具执行失败：tool_execution_failed: bash" {
		t.Fatalf("tool execution error text = %q", text)
	}
	if text := extractUserVisibleErrorText(map[string]any{
		"message": "timed out waiting for runner",
	}); text != "本机 Runner 响应超时，请检查网络连接和 Runner 状态" {
		t.Fatalf("timeout error text = %q", text)
	}
	if text := extractUserVisibleErrorText(nil); text != "" {
		t.Fatalf("error text = %q, want empty", text)
	}
	if delay := nextBackoff(time.Second, 1500*time.Millisecond); delay != 1500*time.Millisecond {
		t.Fatalf("next backoff = %s, want capped max", delay)
	}
	if delay := delayWithJitter(0); delay != 200*time.Millisecond {
		t.Fatalf("jitter delay = %s, want default fallback", delay)
	}
	if taskName := buildTaskName(""); taskName != "未命名任务" {
		t.Fatalf("task name = %q, want unnamed fallback", taskName)
	}
	if status := deriveRunStatus("phase_changed", map[string]any{
		"payload": map[string]any{"to": "plan"},
	}, "thinking"); status != "planning" {
		t.Fatalf("status = %q, want planning", status)
	}
	if stalled := shouldMarkRunStalled(sessionBinding{
		Result:         "pending",
		ApprovalStatus: "none",
		LastEventTime:  time.Now().UTC().Add(-(defaultRunStallTimeout + time.Second)),
	}, time.Now().UTC()); !stalled {
		t.Fatal("expected stalled run to be detected")
	}
	if stalled := shouldMarkRunStalled(sessionBinding{
		Result:         "pending",
		ApprovalStatus: "pending",
		LastEventTime:  time.Now().UTC().Add(-(defaultRunStallTimeout + time.Second)),
	}, time.Now().UTC()); stalled {
		t.Fatal("did not expect approval-pending run to be marked stalled")
	}
	if stalled := shouldMarkRunStalled(sessionBinding{
		Result:         "success",
		ApprovalStatus: "none",
		LastEventTime:  time.Now().UTC().Add(-(defaultRunStallTimeout + time.Second)),
	}, time.Now().UTC()); stalled {
		t.Fatal("did not expect terminal run to be marked stalled")
	}
	if status := terminalStatusFromResult("success"); status != "success" {
		t.Fatalf("terminal status = %q, want success", status)
	}
	if status := terminalStatusFromResult("failure"); status != "failure" {
		t.Fatalf("terminal status = %q, want failure", status)
	}
	if status := terminalStatusFromResult("interrupted"); status != "interrupted" {
		t.Fatalf("terminal status = %q, want interrupted", status)
	}
	if status := terminalStatusFromResult("unknown"); status != "running" {
		t.Fatalf("terminal status = %q, want running fallback", status)
	}
	if text := buildTerminalFallbackText("success", ""); text != "任务已完成。" {
		t.Fatalf("terminal fallback text = %q, want success default", text)
	}
	if text := buildTerminalFallbackText("success", "执行完成"); text != "任务已完成：\n执行完成" {
		t.Fatalf("terminal fallback text = %q, want success summary", text)
	}
	if text := buildTerminalFallbackText("failure", "命令执行失败"); text != "任务执行失败：\n命令执行失败" {
		t.Fatalf("terminal fallback text = %q, want failure summary", text)
	}
	if text := buildTerminalFallbackText("failure", ""); text != "任务执行失败，请稍后重试。" {
		t.Fatalf("terminal fallback text = %q, want failure default", text)
	}
	safeLogAdapter := &Adapter{}
	safeLogAdapter.safeLog("ignored")
}

func TestAskUserHelperFunctionsCoverFallbackBranches(t *testing.T) {
	resolved := extractUserQuestionResolved(map[string]any{
		"payload": map[string]any{
			"request_id": " ask-1 ",
			"status":     " Answered ",
			"message":    " 已确认 ",
			"values":     []any{" 选项A ", "", 123, "选项B"},
		},
	})
	if resolved.RequestID != "ask-1" || resolved.Status != "answered" || resolved.Message != "已确认" {
		t.Fatalf("unexpected resolved payload: %#v", resolved)
	}
	if len(resolved.Values) != 2 || resolved.Values[0] != "选项A" || resolved.Values[1] != "选项B" {
		t.Fatalf("resolved values = %#v, want trimmed string values", resolved.Values)
	}
	if fallback := extractUserQuestionResolved(nil); fallback.RequestID != "" || fallback.Status != "" || fallback.Message != "" ||
		len(fallback.Values) != 0 {
		t.Fatalf("nil payload fallback = %#v", fallback)
	}

	if !shouldSendAskUserCard(userQuestionEntry{Kind: "single_choice", Options: []UserQuestionCardOption{{Label: "A"}}}) {
		t.Fatal("expected single choice question to send card")
	}
	if shouldSendAskUserCard(userQuestionEntry{Kind: "text"}) {
		t.Fatal("expected text question without skip to fall back to plain text")
	}
	if !shouldSendAskUserCard(userQuestionEntry{Kind: "text", AllowSkip: true}) {
		t.Fatal("expected skip-enabled text question to send card")
	}

	if !isUserQuestionResolvedEvent(" user_question_timeout ") {
		t.Fatal("expected timeout runtime type to be resolved event")
	}
	if isUserQuestionResolvedEvent("user_question_requested") {
		t.Fatal("did not expect requested runtime type to be resolved event")
	}
	if status := userQuestionStatusFromRuntimeType(" user_question_skipped "); status != "skipped" {
		t.Fatalf("status = %q, want skipped", status)
	}
	if status := userQuestionStatusFromRuntimeType("user_question_timeout"); status != "timeout" {
		t.Fatalf("status = %q, want timeout", status)
	}
	if status := userQuestionStatusFromRuntimeType("user_question_answered"); status != "answered" {
		t.Fatalf("status = %q, want answered", status)
	}

	prompt := buildAskUserTextPrompt(userQuestionEntry{
		RequestID:   "ask-2",
		Title:       "选择部署环境",
		Description: "请确认本次发布目标",
		Options: []UserQuestionCardOption{
			{Label: "测试"},
			{Label: "生产"},
		},
		AllowSkip: true,
	})
	if !strings.Contains(prompt, "选择部署环境") || !strings.Contains(prompt, "可选项：测试 / 生产") {
		t.Fatalf("prompt = %q, want title and option labels", prompt)
	}
	if !strings.Contains(prompt, "请回复：回答 ask-2 <内容>") || !strings.Contains(prompt, "如需跳过：跳过 ask-2") {
		t.Fatalf("prompt = %q, want answer and skip instructions", prompt)
	}
	if fallbackPrompt := buildAskUserTextPrompt(userQuestionEntry{}); !strings.Contains(fallbackPrompt, "请回答以下问题") {
		t.Fatalf("fallback prompt = %q, want default title", fallbackPrompt)
	}

	if summary := buildUserQuestionResolvedSummary(userQuestionEntry{}, "skipped", nil, ""); summary != "用户已跳过该问题" {
		t.Fatalf("skip summary = %q", summary)
	}
	if summary := buildUserQuestionResolvedSummary(userQuestionEntry{}, "timeout", nil, ""); summary != "问题等待超时" {
		t.Fatalf("timeout summary = %q", summary)
	}
	if summary := buildUserQuestionResolvedSummary(userQuestionEntry{}, "answered", nil, " 已提交 "); summary != "用户回答：已提交" {
		t.Fatalf("message summary = %q", summary)
	}
	if summary := buildUserQuestionResolvedSummary(userQuestionEntry{}, "answered", []string{"A", "B"}, ""); summary != "用户回答：A, B" {
		t.Fatalf("values summary = %q", summary)
	}
	if summary := buildUserQuestionResolvedSummary(userQuestionEntry{Title: "选择模式"}, "answered", nil, ""); summary != "用户已回答：选择模式" {
		t.Fatalf("title summary = %q", summary)
	}
	if summary := buildUserQuestionResolvedSummary(userQuestionEntry{}, "answered", nil, ""); summary != "用户已回答问题" {
		t.Fatalf("fallback summary = %q", summary)
	}

	if value := readInt(nil, "count"); value != 0 {
		t.Fatalf("readInt nil map = %d, want 0", value)
	}
	intCases := []struct {
		name string
		raw  any
		want int
	}{
		{name: "int", raw: int(3), want: 3},
		{name: "int32", raw: int32(4), want: 4},
		{name: "int64", raw: int64(5), want: 5},
		{name: "float64", raw: float64(6), want: 6},
		{name: "json number", raw: json.Number("7"), want: 7},
		{name: "invalid", raw: json.Number("bad"), want: 0},
	}
	for _, testCase := range intCases {
		t.Run(testCase.name, func(t *testing.T) {
			if value := readInt(map[string]any{"count": testCase.raw}, "count"); value != testCase.want {
				t.Fatalf("readInt(%s) = %d, want %d", testCase.name, value, testCase.want)
			}
		})
	}
}

func TestIsMentionCurrentBotMatchesConfiguredBotIDs(t *testing.T) {
	cfg := Config{AppID: "cli_app", BotUserID: "ou_bot", BotOpenID: "ou_open_bot"}
	event := FeishuMessageEvent{
		ChatType: "group",
		Mentions: []FeishuMention{
			{UserID: "ou_bot"},
		},
	}
	if !isMentionCurrentBot(event, cfg) {
		t.Fatal("expected mention match by bot_user_id")
	}
}

func TestIsMentionCurrentBotDoesNotTreatAppIDAsUserID(t *testing.T) {
	cfg := Config{AppID: "cli_app"}
	event := FeishuMessageEvent{
		ChatType: "group",
		Mentions: []FeishuMention{
			{UserID: "cli_app"},
		},
	}
	if isMentionCurrentBot(event, cfg) {
		t.Fatal("expected no match when only user_id equals app_id")
	}
}

func TestIsMentionCurrentBotMatchesMentionAppID(t *testing.T) {
	cfg := Config{AppID: "cli_app"}
	event := FeishuMessageEvent{
		ChatType: "group",
		Mentions: []FeishuMention{
			{AppID: "cli_app"},
		},
	}
	if !isMentionCurrentBot(event, cfg) {
		t.Fatal("expected mention match by mention.app_id")
	}
}

func TestBuildPendingPermissionPayloadAndFindApprovalDecision(t *testing.T) {
	binding := sessionBinding{
		ApprovalRecords: []approvalEntry{
			{
				RequestID: "req-pending",
				ToolName:  "bash",
				Operation: "exec",
				Target:    "pwd",
				Reason:    "need confirm",
				Decision:  "pending",
			},
			{
				RequestID: "req-approved",
				Decision:  "allow_once",
			},
		},
	}

	payload, ok := buildPendingPermissionPayload(binding, " req-pending ")
	if !ok {
		t.Fatal("expected pending payload")
	}
	if payload.RequestID != "req-pending" || payload.ToolName != "bash" || payload.Operation != "exec" ||
		payload.Target != "pwd" || payload.Message != "need confirm" {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	if _, ok := buildPendingPermissionPayload(binding, "req-approved"); ok {
		t.Fatal("expected resolved request to not build pending payload")
	}
	if _, ok := buildPendingPermissionPayload(binding, ""); ok {
		t.Fatal("expected empty request id to fail")
	}

	if got := findApprovalDecision(binding.ApprovalRecords, " req-approved "); got != "allow_once" {
		t.Fatalf("expected approval decision, got %q", got)
	}
	if got := findApprovalDecision(binding.ApprovalRecords, "missing"); got != "" {
		t.Fatalf("expected missing decision to be empty, got %q", got)
	}
}

func TestSyncBindingApprovalsFromFSMLocked(t *testing.T) {
	adapter := newTestAdapter(t)
	binding := sessionBinding{}
	fsm := &approvalFSMState{
		ActiveRequestID: "req-active",
		Requests: map[string]approvalRequestNode{
			"req-active": {
				RequestID: "req-active",
				ToolName:  "bash",
				Operation: "exec",
				Target:    "pwd",
				Reason:    "pending reason",
				Decision:  "pending",
				State:     approvalRequestStateDisplayingPending,
			},
			"req-approved": {
				RequestID: "req-approved",
				ToolName:  "fs",
				Operation: "read",
				Target:    "file",
				Reason:    "approved reason",
				Decision:  "allow",
				State:     approvalRequestStateResolvedApproved,
			},
			"req-rejected": {
				RequestID: "req-rejected",
				ToolName:  "net",
				Operation: "post",
				Target:    "url",
				Reason:    "rejected reason",
				Decision:  "deny",
				State:     approvalRequestStateResolvedRejected,
			},
			"req-archived": {
				RequestID: "req-archived",
				ToolName:  "other",
				Operation: "noop",
				Target:    "x",
				Reason:    "archived reason",
				Decision:  "pending",
				State:     approvalRequestStateArchived,
			},
		},
	}

	adapter.syncBindingApprovalsFromFSMLocked(&binding, fsm)
	if binding.ApprovalStatus != "pending" {
		t.Fatalf("expected pending approval status, got %q", binding.ApprovalStatus)
	}
	if len(binding.ApprovalRecords) != 4 {
		t.Fatalf("expected 4 approval records, got %d", len(binding.ApprovalRecords))
	}
	if binding.ApprovalRecords[0].RequestID != "req-active" {
		t.Fatalf("expected active request first, got %+v", binding.ApprovalRecords)
	}

	statusCases := []struct {
		name string
		fsm  *approvalFSMState
		want string
	}{
		{
			name: "rejected",
			fsm: &approvalFSMState{
				Requests: map[string]approvalRequestNode{
					"req": {RequestID: "req", Decision: "reject", State: approvalRequestStateResolvedRejected},
				},
			},
			want: "rejected",
		},
		{
			name: "approved",
			fsm: &approvalFSMState{
				Requests: map[string]approvalRequestNode{
					"req": {RequestID: "req", Decision: "allow_once", State: approvalRequestStateResolvedApproved},
				},
			},
			want: "approved",
		},
		{
			name: "mixed",
			fsm: &approvalFSMState{
				Requests: map[string]approvalRequestNode{
					"req-a": {RequestID: "req-a", Decision: "allow_once", State: approvalRequestStateResolvedApproved},
					"req-r": {RequestID: "req-r", Decision: "reject", State: approvalRequestStateResolvedRejected},
				},
			},
			want: "mixed",
		},
		{
			name: "none",
			fsm: &approvalFSMState{
				Requests: map[string]approvalRequestNode{
					"req": {RequestID: "req", Decision: "", State: approvalRequestStateArchived},
				},
			},
			want: "none",
		},
	}
	for _, tc := range statusCases {
		t.Run(tc.name, func(t *testing.T) {
			derived := sessionBinding{}
			adapter.syncBindingApprovalsFromFSMLocked(&derived, tc.fsm)
			if derived.ApprovalStatus != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, derived.ApprovalStatus)
			}
		})
	}
}

func TestApprovalHelperLookups(t *testing.T) {
	if containsApprovalRequest([]string{" req-1 ", "req-2"}, "req-1") != true {
		t.Fatal("expected request to be found in pending stack")
	}
	if containsApprovalRequest([]string{"req-1"}, " ") {
		t.Fatal("expected empty request id to be rejected")
	}
	if containsApprovalRequest([]string{"req-1"}, "req-2") {
		t.Fatal("expected missing request to not be found")
	}

	adapter := newTestAdapter(t)
	adapter.mu.Lock()
	adapter.approvalFSMByRun["run-a"] = &approvalFSMState{
		ActiveRequestID: "shared",
		Requests: map[string]approvalRequestNode{
			"shared": {RequestID: "shared"},
		},
	}
	adapter.approvalFSMByRun["run-b"] = &approvalFSMState{
		Requests: map[string]approvalRequestNode{
			"shared": {RequestID: "shared"},
		},
	}
	adapter.approvalRequestRunIndex[approvalRequestScopedKey("run-a", "shared")] = "run-a"
	adapter.approvalRequestRunIndex[approvalRequestScopedKey("run-b", "shared")] = "other-run"
	adapter.approvalRequestIDRunIndex["shared"] = "run-b"
	adapter.mu.Unlock()

	adapter.mu.RLock()
	got := adapter.resolveApprovalRunKeyByRequestLocked("shared")
	adapter.mu.RUnlock()
	if got != "run-a" {
		t.Fatalf("expected active run to win fallback scan, got %q", got)
	}

	adapter.mu.Lock()
	delete(adapter.approvalRequestRunIndex, approvalRequestScopedKey("run-a", "shared"))
	adapter.approvalRequestRunIndex[approvalRequestScopedKey("run-b", "shared")] = "run-b"
	adapter.mu.Unlock()

	adapter.mu.RLock()
	got = adapter.resolveApprovalRunKeyByRequestLocked("shared")
	adapter.mu.RUnlock()
	if got != "run-b" {
		t.Fatalf("expected indexed run to be used after scoped entry removal, got %q", got)
	}

	adapter.mu.RLock()
	if got = adapter.resolveApprovalRunKeyByRequestLocked("missing"); got != "" {
		adapter.mu.RUnlock()
		t.Fatalf("expected missing request id to resolve empty run key, got %q", got)
	}
	adapter.mu.RUnlock()
}

func TestApprovalOutboxVersionGuardsAndCleanup(t *testing.T) {
	adapter := newTestAdapter(t)
	messenger := adapterTestMessenger(adapter)

	if gen, ver, ok := adapter.snapshotApprovalFSMVersion(""); ok || gen != 0 || ver != 0 {
		t.Fatalf("expected empty run key snapshot miss, got %d %d %v", gen, ver, ok)
	}
	if adapter.shouldExecuteApprovalOutbox(approvalOutboxOperation{RunKey: "missing"}) {
		t.Fatal("expected missing run key to fail preflight")
	}

	adapter.mu.Lock()
	adapter.approvalFSMByRun["run-1"] = &approvalFSMState{Generation: 11, Version: 22}
	adapter.mu.Unlock()

	if gen, ver, ok := adapter.snapshotApprovalFSMVersion("run-1"); !ok || gen != 11 || ver != 22 {
		t.Fatalf("unexpected snapshot: %d %d %v", gen, ver, ok)
	}

	match := approvalOutboxOperation{RunKey: "run-1", Generation: 11, Version: 22}
	if !adapter.shouldExecuteApprovalOutbox(match) {
		t.Fatal("expected matching outbox to pass preflight")
	}
	if adapter.shouldExecuteApprovalOutbox(approvalOutboxOperation{RunKey: "run-1", Generation: 11, Version: 21}) {
		t.Fatal("expected stale outbox to fail preflight")
	}

	adapter.confirmApprovalOutbox(match)
	adapter.confirmApprovalOutbox(approvalOutboxOperation{RunKey: "run-1", Generation: 10, Version: 22})
	adapter.confirmApprovalOutbox(approvalOutboxOperation{RunKey: "missing", Generation: 1, Version: 1})

	adapter.cleanupStalePermissionCard(match, "")
	adapter.cleanupStalePermissionCard(match, "card-cleanup")
	messages := messenger.snapshot()
	if len(messages) == 0 || messages[len(messages)-1].kind != "delete_card" || messages[len(messages)-1].chatID != "card-cleanup" {
		t.Fatalf("expected cleanup to delete stale card, got %+v", messages)
	}
}

func TestSchedulePermissionCardDismiss(t *testing.T) {
	adapter := newTestAdapter(t)
	adapter.permissionCardDismissDelay = 5 * time.Millisecond
	messenger := adapterTestMessenger(adapter)

	adapter.mu.Lock()
	adapter.approvalFSMByRun["run-1"] = &approvalFSMState{CardID: "card-1"}
	adapter.approvalCardRunIndex["card-1"] = "run-1"
	adapter.mu.Unlock()

	adapter.schedulePermissionCardDismiss("req-1", "card-1")
	time.Sleep(30 * time.Millisecond)

	messages := messenger.snapshot()
	if len(messages) == 0 || messages[len(messages)-1].kind != "delete_card" || messages[len(messages)-1].chatID != "card-1" {
		t.Fatalf("expected permission card delete, got %+v", messages)
	}

	adapter.mu.RLock()
	defer adapter.mu.RUnlock()
	if _, ok := adapter.approvalCardRunIndex["card-1"]; ok {
		t.Fatal("expected approval card index removed after dismiss")
	}
	if got := adapter.approvalFSMByRun["run-1"].CardID; got != "" {
		t.Fatalf("expected fsm card id cleared, got %q", got)
	}
}

func TestMarkRunTerminalFallbackPaths(t *testing.T) {
	t.Run("empty card id sends terminal text", func(t *testing.T) {
		adapter := newTestAdapter(t)
		sessionID := BuildSessionID("chat-terminal-empty-card")
		runID := BuildRunID("run-terminal-empty-card")
		key := runBindingKey(sessionID, runID)

		adapter.mu.Lock()
		adapter.activeRuns[key] = sessionBinding{
			SessionID: sessionID,
			RunID:     runID,
			ChatID:    "chat-terminal-empty-card",
			Status:    "running",
			Result:    "pending",
		}
		adapter.mu.Unlock()

		adapter.markRunTerminal(sessionID, runID, "success", "", "fallback summary")

		messages := adapterTestMessenger(adapter).snapshot()
		if len(messages) == 0 || messages[len(messages)-1].kind != "text" {
			t.Fatalf("expected fallback terminal text, got %+v", messages)
		}
		if !strings.Contains(messages[len(messages)-1].text, "fallback summary") {
			t.Fatalf("expected fallback summary in terminal text, got %+v", messages[len(messages)-1])
		}

		adapter.mu.RLock()
		binding := adapter.activeRuns[key]
		adapter.mu.RUnlock()
		if binding.LastSummary != "fallback summary" || binding.Status != "success" || binding.Result != "success" {
			t.Fatalf("unexpected terminal binding: %+v", binding)
		}
	})

	t.Run("card update failure falls back to text", func(t *testing.T) {
		adapter := newTestAdapter(t)
		messenger := adapterTestMessenger(adapter)
		messenger.updateCardErr = fmt.Errorf("update failed")
		sessionID := BuildSessionID("chat-terminal-update-fail")
		runID := BuildRunID("run-terminal-update-fail")
		key := runBindingKey(sessionID, runID)

		adapter.mu.Lock()
		adapter.activeRuns[key] = sessionBinding{
			SessionID:   sessionID,
			RunID:       runID,
			ChatID:      "chat-terminal-update-fail",
			CardID:      "status-card-1",
			Status:      "running",
			Result:      "pending",
			LastSummary: "old summary",
		}
		adapter.mu.Unlock()

		adapter.markRunTerminal(sessionID, runID, "failure", "new summary", "")

		messages := messenger.snapshot()
		if len(messages) < 2 {
			t.Fatalf("expected update failure followed by fallback text, got %+v", messages)
		}
		last := messages[len(messages)-1]
		if last.kind != "text" || !strings.Contains(last.text, "new summary") {
			t.Fatalf("expected fallback text with new summary, got %+v", last)
		}
	})
}

func TestHandleMessageBranches(t *testing.T) {
	t.Run("rejects missing identifiers", func(t *testing.T) {
		adapter := newTestAdapter(t)
		err := adapter.HandleMessage(context.Background(), FeishuMessageEvent{
			MessageID:   "",
			ChatID:      "chat",
			ContentText: "run",
		})
		if err == nil || !strings.Contains(err.Error(), "missing message_id or chat_id") {
			t.Fatalf("expected missing identifier error, got %v", err)
		}
	})

	t.Run("ignores empty text and duplicate event", func(t *testing.T) {
		adapter := newTestAdapter(t)
		event := FeishuMessageEvent{
			EventID:     "evt-empty",
			MessageID:   "msg-empty",
			ChatID:      "chat-empty",
			ContentText: "   ",
		}
		if err := adapter.HandleMessage(context.Background(), event); err != nil {
			t.Fatalf("handle empty text: %v", err)
		}
		if err := adapter.HandleMessage(context.Background(), event); err != nil {
			t.Fatalf("handle duplicate empty text: %v", err)
		}
		if calls := adapterTestGateway(adapter).snapshotCalls(); len(calls) != 0 {
			t.Fatalf("expected no gateway calls for empty text, got %v", calls)
		}
	})

	t.Run("run failure sends fallback text", func(t *testing.T) {
		adapter := newTestAdapter(t)
		gateway := adapterTestGateway(adapter)
		gateway.runErr = fmt.Errorf("run failed")

		err := adapter.HandleMessage(context.Background(), FeishuMessageEvent{
			EventID:     "evt-run-fail",
			MessageID:   "msg-run-fail",
			ChatID:      "chat-run-fail",
			ChatType:    "group",
			ContentText: "执行失败分支",
		})
		if err == nil || !strings.Contains(err.Error(), "run failed") {
			t.Fatalf("expected run failure, got %v", err)
		}

		msgs := adapterTestMessenger(adapter).snapshot()
		if len(msgs) == 0 || msgs[len(msgs)-1].kind != "text" || msgs[len(msgs)-1].text != "任务受理失败，请稍后重试。" {
			t.Fatalf("expected fallback text after run failure, got %+v", msgs)
		}

		sessionID := BuildSessionID("chat-run-fail")
		runID := BuildRunID("msg-run-fail")
		adapter.mu.RLock()
		_, exists := adapter.activeRuns[runBindingKey(sessionID, runID)]
		adapter.mu.RUnlock()
		if exists {
			t.Fatal("expected failed run to be untracked")
		}
	})
}

func TestParseUserQuestionTextAnswerAndHelpers(t *testing.T) {
	adapter := newTestAdapter(t)

	if values, message, ok := adapter.parseUserQuestionTextAnswer("missing", " free text "); !ok || len(values) != 1 ||
		values[0] != "free text" || message != "free text" {
		t.Fatalf("expected fallback free text answer, got values=%v message=%q ok=%v", values, message, ok)
	}
	if _, _, ok := adapter.parseUserQuestionTextAnswer("missing", " "); ok {
		t.Fatal("expected empty fallback answer to fail")
	}

	adapter.mu.Lock()
	adapter.pendingQuestions["text"] = userQuestionEntry{RequestID: "text", Kind: "text"}
	adapter.pendingQuestions["single-empty-options"] = userQuestionEntry{RequestID: "single-empty-options", Kind: "single_choice"}
	adapter.pendingQuestions["single"] = userQuestionEntry{
		RequestID: "single",
		Kind:      "single_choice",
		Options: []UserQuestionCardOption{
			{Label: "Alpha"},
			{Label: "Beta Option"},
		},
	}
	adapter.pendingQuestions["multi"] = userQuestionEntry{
		RequestID:  "multi",
		Kind:       "multi_choice",
		MaxChoices: 2,
		Options: []UserQuestionCardOption{
			{Label: "One"},
			{Label: "Two"},
			{Label: "Three"},
		},
	}
	adapter.pendingQuestions["other"] = userQuestionEntry{RequestID: "other", Kind: "other"}
	adapter.mu.Unlock()

	if _, _, ok := adapter.parseUserQuestionTextAnswer("text", " "); ok {
		t.Fatal("expected empty text answer to fail")
	}
	if values, message, ok := adapter.parseUserQuestionTextAnswer("text", "hello"); !ok || message != "hello" || values[0] != "hello" {
		t.Fatalf("unexpected text answer parse result: values=%v message=%q ok=%v", values, message, ok)
	}
	if values, message, ok := adapter.parseUserQuestionTextAnswer("single-empty-options", "custom"); !ok || len(values) != 1 ||
		values[0] != "custom" || message != "" {
		t.Fatalf("unexpected single-choice without options result: values=%v message=%q ok=%v", values, message, ok)
	}
	if values, message, ok := adapter.parseUserQuestionTextAnswer("single", "2"); !ok || len(values) != 1 ||
		values[0] != "Beta Option" || message != "" {
		t.Fatalf("unexpected single-choice index result: values=%v message=%q ok=%v", values, message, ok)
	}
	if _, _, ok := adapter.parseUserQuestionTextAnswer("single", "missing"); ok {
		t.Fatal("expected unknown single-choice label to fail")
	}
	if values, message, ok := adapter.parseUserQuestionTextAnswer("multi", "one, Two，one"); !ok || len(values) != 2 ||
		values[0] != "One" || values[1] != "Two" || message != "" {
		t.Fatalf("unexpected multi-choice result: values=%v message=%q ok=%v", values, message, ok)
	}
	if _, _, ok := adapter.parseUserQuestionTextAnswer("multi", "one two three"); ok {
		t.Fatal("expected max-choices violation to fail")
	}
	if values, message, ok := adapter.parseUserQuestionTextAnswer("other", "free"); !ok || len(values) != 1 ||
		values[0] != "free" || message != "free" {
		t.Fatalf("unexpected default answer parse result: values=%v message=%q ok=%v", values, message, ok)
	}

	if got, ok := resolveChoiceLabel("  beta option ", []UserQuestionCardOption{{Label: "Alpha"}, {Label: "Beta Option"}}); !ok || got != "Beta Option" {
		t.Fatalf("expected normalized label match, got %q %v", got, ok)
	}
	if _, ok := resolveChoiceLabel("9", []UserQuestionCardOption{{Label: "Alpha"}}); ok {
		t.Fatal("expected out-of-range index to fail")
	}

	requestID, body := splitRequestAndBody(" req-1  line one line two ")
	if requestID != "req-1" || body != "line one line two" {
		t.Fatalf("unexpected request/body split: %q %q", requestID, body)
	}
	if requestID, body := splitRequestAndBody(" "); requestID != "" || body != "" {
		t.Fatalf("expected empty split result, got %q %q", requestID, body)
	}

	tokens := splitMultiChoiceTokens(" one，two|three ; two ")
	if len(tokens) != 3 || tokens[0] != "one" || tokens[1] != "two" || tokens[2] != "three" {
		t.Fatalf("unexpected tokens: %v", tokens)
	}
	tokens = splitMultiChoiceTokens(" one   two  one ")
	if len(tokens) != 2 || tokens[0] != "one" || tokens[1] != "two" {
		t.Fatalf("unexpected whitespace tokens: %v", tokens)
	}

	unique := uniqueNonEmptyStrings([]string{" Alpha ", "alpha", "", "Beta"})
	if len(unique) != 2 || unique[0] != "Alpha" || unique[1] != "Beta" {
		t.Fatalf("unexpected unique strings: %v", unique)
	}
}

func TestExtractUserQuestionRequestAndApprovalDecisionHelpers(t *testing.T) {
	entry := extractUserQuestionRequest(map[string]any{
		"payload": map[string]any{
			"request_id":  " req-1 ",
			"question_id": " q-1 ",
			"title":       " 选择环境 ",
			"description": " 请确认发布目标 ",
			"kind":        " Multi_Choice ",
			"allow_skip":  true,
			"max_choices": int32(2),
			"options": []any{
				" 测试 ",
				map[string]any{"label": " 生产 ", "description": " 正式环境 "},
				map[string]any{"label": " "},
				123,
			},
		},
	})
	if entry.RequestID != "req-1" || entry.QuestionID != "q-1" || entry.Title != "选择环境" ||
		entry.Description != "请确认发布目标" || entry.Kind != "multi_choice" ||
		!entry.AllowSkip || entry.MaxChoices != 2 {
		t.Fatalf("unexpected user question entry: %+v", entry)
	}
	if len(entry.Options) != 2 || entry.Options[0].Label != "测试" || entry.Options[1].Label != "生产" ||
		entry.Options[1].Description != "正式环境" {
		t.Fatalf("unexpected user question options: %+v", entry.Options)
	}
	if fallback := extractUserQuestionRequest(nil); fallback.RequestID != "" || len(fallback.Options) != 0 {
		t.Fatalf("expected nil envelope fallback, got %+v", fallback)
	}

	requestID, decision := extractPermissionResolved(map[string]any{
		"payload": map[string]any{
			"request_id": " req-2 ",
			"decision":   " Allow ",
		},
	})
	if requestID != "req-2" || decision != "allow" {
		t.Fatalf("unexpected resolved permission payload: request_id=%q decision=%q", requestID, decision)
	}
	if requestID, decision := extractPermissionResolved(nil); requestID != "" || decision != "" {
		t.Fatalf("expected nil permission resolved payload, got %q %q", requestID, decision)
	}

	if got := normalizeApprovalDecision(" Denied "); got != "reject" {
		t.Fatalf("expected denied alias normalized to reject, got %q", got)
	}
	if got := normalizeApprovalDecision("allow_session"); got != "allow_session" {
		t.Fatalf("expected allow_session to stay stable, got %q", got)
	}
	if !isPermissionRequestNotFoundError(fmt.Errorf("permission request abc not found")) {
		t.Fatal("expected not-found error to be detected")
	}
	if isPermissionRequestNotFoundError(nil) {
		t.Fatal("expected nil error to not match")
	}
	if isPermissionRequestNotFoundError(fmt.Errorf("other error")) {
		t.Fatal("expected unrelated error to not match")
	}
	if readBool(nil, "ok") {
		t.Fatal("expected nil map bool lookup to fall back to false")
	}
	if !readBool(map[string]any{"ok": true}, "ok") {
		t.Fatal("expected bool field to be read")
	}
	if readBool(map[string]any{}, "ok") {
		t.Fatal("expected missing bool field to fall back to false")
	}
	if readBool(map[string]any{"ok": "true"}, "ok") {
		t.Fatal("expected non-bool field to fall back to false")
	}
}

func TestUpdateUserQuestionStatusUpdatesCardsAndState(t *testing.T) {
	adapter := newTestAdapter(t)
	sessionID := BuildSessionID("chat-user-question-status")
	runID := BuildRunID("run-user-question-status")
	key := runBindingKey(sessionID, runID)

	adapter.mu.Lock()
	adapter.activeRuns[key] = sessionBinding{
		SessionID: sessionID,
		RunID:     runID,
		ChatID:    "chat-user-question-status",
		CardID:    "status-card-1",
		Status:    "running",
		Result:    "pending",
	}
	adapter.pendingQuestions["ask-1"] = userQuestionEntry{
		RequestID: "ask-1",
		Title:     "选择环境",
		Kind:      "single_choice",
	}
	adapter.userQuestionCards["ask-1"] = "ask-card-1"
	adapter.requestRuns["ask-1"] = key
	adapter.mu.Unlock()

	adapter.updateUserQuestionStatus("ask-1", "answered", []string{"测试"}, "已确认")

	msgs := adapterTestMessenger(adapter).snapshot()
	if len(msgs) < 2 {
		t.Fatalf("expected status card and ask-user card updates, got %+v", msgs)
	}
	foundStatus := false
	foundAskCard := false
	for _, msg := range msgs {
		if msg.kind == "update_card" && msg.cardID == "status-card-1" {
			foundStatus = true
		}
		if msg.kind == "update_ask_card" && msg.chatID == "ask-card-1" && msg.resolvedUserQuestion != nil &&
			msg.resolvedUserQuestion.RequestID == "ask-1" && msg.resolvedUserQuestion.Status == "answered" {
			foundAskCard = true
		}
	}
	if !foundStatus || !foundAskCard {
		t.Fatalf("expected both card updates, got %+v", msgs)
	}

	adapter.mu.RLock()
	binding := adapter.activeRuns[key]
	_, pendingExists := adapter.pendingQuestions["ask-1"]
	_, askCardExists := adapter.userQuestionCards["ask-1"]
	_, requestExists := adapter.requestRuns["ask-1"]
	adapter.mu.RUnlock()
	if !strings.Contains(binding.LastSummary, "已确认") {
		t.Fatalf("expected resolved summary recorded, got %+v", binding)
	}
	if pendingExists || askCardExists || requestExists {
		t.Fatalf("expected ask-user indexes cleaned, pending=%v card=%v request=%v", pendingExists, askCardExists, requestExists)
	}
}

func TestUpdateApprovalStatusPromotesNextPendingAndUpdatesCards(t *testing.T) {
	adapter := newTestAdapter(t)
	sessionID := BuildSessionID("chat-approval-update")
	runID := BuildRunID("run-approval-update")
	key := runBindingKey(sessionID, runID)

	adapter.mu.Lock()
	adapter.activeRuns[key] = sessionBinding{
		SessionID: sessionID,
		RunID:     runID,
		ChatID:    "chat-approval-update",
		CardID:    "status-card-1",
		Status:    "waiting_approval",
		Result:    "pending",
	}
	adapter.approvalFSMByRun[key] = &approvalFSMState{
		Generation:      7,
		Version:         3,
		CardID:          "perm-card-1",
		ActiveRequestID: "req-1",
		PendingStack:    []string{"req-2"},
		Requests: map[string]approvalRequestNode{
			"req-1": {
				RequestID: "req-1",
				ToolName:  "bash",
				Operation: "exec",
				Target:    "pwd",
				Reason:    "need first approval",
				Decision:  "pending",
				State:     approvalRequestStateDisplayingPending,
			},
			"req-2": {
				RequestID: "req-2",
				ToolName:  "fs",
				Operation: "write",
				Target:    "file.txt",
				Reason:    "need second approval",
				Decision:  "pending",
				State:     approvalRequestStateQueued,
			},
		},
	}
	adapter.approvalRequestRunIndex[approvalRequestScopedKey(key, "req-1")] = key
	adapter.approvalRequestRunIndex[approvalRequestScopedKey(key, "req-2")] = key
	adapter.approvalRequestIDRunIndex["req-1"] = key
	adapter.approvalRequestIDRunIndex["req-2"] = key
	adapter.approvalCardRunIndex["perm-card-1"] = key
	adapter.runPermissionCardHistory[key] = map[string]struct{}{
		"perm-card-1":       {},
		"perm-card-history": {},
	}
	adapter.mu.Unlock()

	adapter.updateApprovalStatus("req-1", "allow")

	msgs := adapterTestMessenger(adapter).snapshot()
	kinds := map[string]int{}
	for _, msg := range msgs {
		kinds[msg.kind]++
	}
	if kinds["update_card"] == 0 || kinds["update_perm_card"] == 0 || kinds["update_pending_perm_card"] == 0 {
		t.Fatalf("expected status/resolved/pending card updates, got %+v", msgs)
	}

	adapter.mu.RLock()
	fsm := adapter.approvalFSMByRun[key]
	binding := adapter.activeRuns[key]
	adapter.mu.RUnlock()
	if fsm.Version != 4 {
		t.Fatalf("expected fsm version incremented, got %d", fsm.Version)
	}
	if fsm.ActiveRequestID != "req-2" {
		t.Fatalf("expected queued request promoted, got %q", fsm.ActiveRequestID)
	}
	if fsm.Requests["req-1"].State != approvalRequestStateResolvedApproved {
		t.Fatalf("expected first request resolved approved, got %+v", fsm.Requests["req-1"])
	}
	if fsm.Requests["req-2"].State != approvalRequestStateDisplayingPending {
		t.Fatalf("expected second request promoted to displaying_pending, got %+v", fsm.Requests["req-2"])
	}
	if binding.ApprovalStatus != "pending" {
		t.Fatalf("expected binding approval status stay pending after promotion, got %+v", binding)
	}
}

func TestUntrackRunCleansDerivedState(t *testing.T) {
	adapter := newTestAdapter(t)
	sessionID := BuildSessionID("chat-untrack")
	runID := BuildRunID("run-untrack")
	key := runBindingKey(sessionID, runID)

	adapter.mu.Lock()
	adapter.activeRuns[key] = sessionBinding{
		SessionID: sessionID,
		RunID:     runID,
		ChatID:    "chat-untrack",
		CardID:    "status-card-1",
	}
	adapter.requestRuns["ask-1"] = key
	adapter.userQuestionCards["ask-1"] = "ask-card-1"
	adapter.pendingQuestions["ask-1"] = userQuestionEntry{RequestID: "ask-1"}
	adapter.approvalRequestRunIndex[approvalRequestScopedKey(key, "perm-1")] = key
	adapter.approvalRequestIDRunIndex["perm-1"] = key
	adapter.approvalCardRunIndex["perm-card-1"] = key
	adapter.approvalFSMByRun[key] = &approvalFSMState{
		Generation: 1,
		Requests: map[string]approvalRequestNode{
			"perm-1": {RequestID: "perm-1"},
		},
	}
	adapter.runPermissionCardHistory[key] = map[string]struct{}{"perm-card-1": {}}
	adapter.lastProgressAt[key] = time.Now().UTC()
	adapter.mu.Unlock()

	adapter.untrackRun(sessionID, runID)

	adapter.mu.RLock()
	defer adapter.mu.RUnlock()
	if _, ok := adapter.activeRuns[key]; ok {
		t.Fatal("expected active run removed")
	}
	if _, ok := adapter.requestRuns["ask-1"]; ok {
		t.Fatal("expected requestRuns cleaned")
	}
	if _, ok := adapter.userQuestionCards["ask-1"]; ok {
		t.Fatal("expected userQuestionCards cleaned")
	}
	if _, ok := adapter.pendingQuestions["ask-1"]; ok {
		t.Fatal("expected pendingQuestions cleaned")
	}
	if _, ok := adapter.approvalRequestRunIndex[approvalRequestScopedKey(key, "perm-1")]; ok {
		t.Fatal("expected approvalRequestRunIndex cleaned")
	}
	if _, ok := adapter.approvalRequestIDRunIndex["perm-1"]; ok {
		t.Fatal("expected approvalRequestIDRunIndex cleaned")
	}
	if _, ok := adapter.approvalCardRunIndex["perm-card-1"]; ok {
		t.Fatal("expected approvalCardRunIndex cleaned")
	}
	if _, ok := adapter.approvalFSMByRun[key]; ok {
		t.Fatal("expected approval FSM cleaned")
	}
	if _, ok := adapter.runPermissionCardHistory[key]; ok {
		t.Fatal("expected permission card history cleaned")
	}
	if _, ok := adapter.lastProgressAt[key]; ok {
		t.Fatal("expected progress throttle state cleaned")
	}
}

func TestExtractProgressLineAndProgressHelpers(t *testing.T) {
	cases := []struct {
		name        string
		runtimeType string
		envelope    map[string]any
		want        string
	}{
		{
			name:        "phase changed",
			runtimeType: "phase_changed",
			envelope:    map[string]any{"payload": map[string]any{"to": "tool_call"}},
			want:        "进入阶段：tool_call",
		},
		{
			name:        "tool start",
			runtimeType: "tool_start",
			envelope:    map[string]any{"payload": map[string]any{"tool_name": "bash", "operation": "exec", "target": "pwd"}},
			want:        "开始工具：bash · exec · pwd",
		},
		{
			name:        "tool result status",
			runtimeType: "tool_result",
			envelope:    map[string]any{"payload": map[string]any{"tool_name": "bash", "status": "ok"}},
			want:        "bash完成：ok",
		},
		{
			name:        "tool result default name",
			runtimeType: "tool_result",
			envelope:    map[string]any{"payload": map[string]any{}},
			want:        "工具完成",
		},
		{
			name:        "permission requested default tool",
			runtimeType: "permission_requested",
			envelope:    map[string]any{"payload": map[string]any{}},
			want:        "等待审批：工具操作",
		},
		{
			name:        "permission rejected",
			runtimeType: "permission_resolved",
			envelope:    map[string]any{"payload": map[string]any{"decision": "reject"}},
			want:        "审批结果：已拒绝",
		},
		{
			name:        "permission approved",
			runtimeType: "permission_resolved",
			envelope:    map[string]any{"payload": map[string]any{"decision": "allow"}},
			want:        "审批结果：已通过",
		},
		{
			name:        "user question requested",
			runtimeType: "user_question_requested",
			envelope:    map[string]any{},
			want:        "等待用户回答问题",
		},
		{
			name:        "user question answered",
			runtimeType: "user_question_answered",
			envelope:    map[string]any{},
			want:        "用户已回答问题",
		},
		{
			name:        "user question skipped",
			runtimeType: "user_question_skipped",
			envelope:    map[string]any{},
			want:        "用户已跳过问题",
		},
		{
			name:        "run error",
			runtimeType: "run_error",
			envelope:    map[string]any{"payload": map[string]any{"message": "boom"}},
			want:        "执行失败：boom",
		},
		{
			name:        "run done",
			runtimeType: "run_done",
			envelope:    map[string]any{},
			want:        "执行完成",
		},
		{
			name:        "unknown",
			runtimeType: "other",
			envelope:    map[string]any{},
			want:        "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractProgressLine(tc.runtimeType, tc.envelope); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}

	trail := appendProgressTrail([]string{"a"}, "a", 3)
	if len(trail) != 1 || trail[0] != "a" {
		t.Fatalf("expected duplicate line ignored, got %v", trail)
	}
	trail = appendProgressTrail(trail, "b", 2)
	trail = appendProgressTrail(trail, "c", 2)
	if len(trail) != 2 || trail[0] != "b" || trail[1] != "c" {
		t.Fatalf("expected tail truncation, got %v", trail)
	}
	trail = appendProgressTrail(trail, " ", 2)
	if len(trail) != 2 {
		t.Fatalf("expected blank line ignored, got %v", trail)
	}

	if !equalStringSlices([]string{"a", "b"}, []string{"a", "b"}) {
		t.Fatal("expected equal slices to match")
	}
	if equalStringSlices([]string{"a"}, []string{"b"}) {
		t.Fatal("expected differing slices to not match")
	}
	if equalStringSlices([]string{"a"}, []string{"a", "b"}) {
		t.Fatal("expected differing lengths to not match")
	}
}

func newTestAdapter(t *testing.T) *Adapter {
	t.Helper()
	gateway := newFakeGatewayClient()
	messenger := &fakeMessenger{}
	adapter, err := New(Config{
		ListenAddress:       "127.0.0.1:18080",
		EventPath:           "/feishu/events",
		CardPath:            "/feishu/cards",
		AppID:               "app",
		AppSecret:           "secret",
		BotUserID:           "ou_bot",
		BotOpenID:           "ou_open_bot",
		VerifyToken:         "verify",
		SigningSecret:       "sign-secret",
		RequestTimeout:      200 * time.Millisecond,
		IdempotencyTTL:      2 * time.Minute,
		ReconnectBackoffMin: 10 * time.Millisecond,
		ReconnectBackoffMax: 20 * time.Millisecond,
		RebindInterval:      20 * time.Millisecond,
	}, gateway, messenger, nil)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	return adapter
}

func adapterTestGateway(adapter *Adapter) *fakeGatewayClient {
	return adapter.gateway.(*fakeGatewayClient)
}

func adapterTestMessenger(adapter *Adapter) *fakeMessenger {
	return adapter.messenger.(*fakeMessenger)
}

func messageEventBody(eventID string, messageID string, chatID string, text string) string {
	return messageEventBodyWithChatType(eventID, messageID, chatID, text, "")
}

func messageEventBodyWithChatType(eventID string, messageID string, chatID string, text string, chatType string) string {
	content, _ := json.Marshal(map[string]string{"text": text})
	payload := map[string]any{
		"header": map[string]any{
			"event_id":   eventID,
			"event_type": "im.message.receive_v1",
			"token":      "verify",
		},
		"event": map[string]any{
			"message": map[string]any{
				"message_id": messageID,
				"chat_id":    chatID,
				"chat_type":  chatType,
				"content":    string(content),
			},
		},
	}
	data, _ := json.Marshal(payload)
	return string(data)
}

func signedRequest(t *testing.T, secret string, body string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/callback", bytes.NewBufferString(body))
	timestamp := strconvTimestamp(time.Now().UTC())
	nonce := "nonce"
	raw := timestamp + nonce + body
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(raw))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	request.Header.Set(headerLarkTimestamp, timestamp)
	request.Header.Set(headerLarkNonce, nonce)
	request.Header.Set(headerLarkSignature, signature)
	return request
}

func strconvTimestamp(now time.Time) string {
	return fmt.Sprintf("%d", now.Unix())
}

func pushGatewayEvent(t *testing.T, gw *fakeGatewayClient, sessionID string, runID string, eventType string, envelope map[string]any) {
	t.Helper()
	frame := map[string]any{
		"session_id": sessionID,
		"run_id":     runID,
		"payload": map[string]any{
			"event_type": eventType,
			"payload":    envelope,
		},
	}
	data, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	gw.notifications <- GatewayNotification{Method: protocol.MethodGatewayEvent, Params: data}
}

func waitForCalls(t *testing.T, gw *fakeGatewayClient, match func(string) bool) {
	t.Helper()
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		calls := strings.Join(gw.snapshotCalls(), "|")
		if match(calls) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within timeout, calls=%v", gw.snapshotCalls())
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, assertErr("read failed")
}
