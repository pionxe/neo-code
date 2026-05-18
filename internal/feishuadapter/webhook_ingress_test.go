package feishuadapter

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type failingIngressHandler struct {
	messageErr error
	cardErr    error
}

func (f *failingIngressHandler) HandleMessage(_ context.Context, _ FeishuMessageEvent) error {
	return f.messageErr
}

func (f *failingIngressHandler) HandleCardAction(_ context.Context, _ FeishuCardActionEvent) error {
	return f.cardErr
}

func TestNewWebhookIngressAndRunContextCancel(t *testing.T) {
	ingress, ok := NewWebhookIngress(Config{
		ListenAddress: "127.0.0.1:0",
		EventPath:     "/events",
		CardPath:      "/cards",
	}, nil).(*WebhookIngress)
	if !ok {
		t.Fatal("expected webhook ingress instance")
	}
	if ingress.nowFn == nil {
		t.Fatal("expected default nowFn")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- ingress.Run(ctx, &captureIngressHandler{})
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for webhook ingress shutdown")
	}
}

func TestNewWebhookIngressUsesProvidedClockAndRunReturnsListenError(t *testing.T) {
	fixed := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { return fixed }
	ingress, ok := NewWebhookIngress(Config{
		ListenAddress: "bad::addr",
		EventPath:     "/events",
		CardPath:      "/cards",
	}, nowFn).(*WebhookIngress)
	if !ok {
		t.Fatal("expected webhook ingress instance")
	}
	if got := ingress.nowFn(); !got.Equal(fixed) {
		t.Fatalf("expected provided nowFn, got %v", got)
	}
	if err := ingress.Run(context.Background(), &captureIngressHandler{}); err == nil {
		t.Fatal("expected listen error for invalid address")
	}
}

func TestWebhookIngressHandleFeishuEventVerificationIgnoreAndHandlerError(t *testing.T) {
	ingress, ok := NewWebhookIngress(Config{
		VerifyToken:   "verify",
		SigningSecret: "sign-secret",
	}, nil).(*WebhookIngress)
	if !ok {
		t.Fatal("expected webhook ingress instance")
	}

	t.Run("url verification", func(t *testing.T) {
		request := signedRequest(t, ingress.cfg.SigningSecret, `{"type":"url_verification","challenge":"hello","token":"verify"}`)
		recorder := httptest.NewRecorder()

		ingress.handleFeishuEvent(&captureIngressHandler{}).ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
		}
		if !strings.Contains(recorder.Body.String(), `"challenge":"hello"`) {
			t.Fatalf("response = %s, want challenge body", recorder.Body.String())
		}
	})

	t.Run("unsupported event ignored before token check", func(t *testing.T) {
		request := signedRequest(t, ingress.cfg.SigningSecret, `{"header":{"event_type":"other"}}`)
		recorder := httptest.NewRecorder()

		ingress.handleFeishuEvent(&captureIngressHandler{}).ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
		}
		if !strings.Contains(recorder.Body.String(), `"message":"ignored"`) {
			t.Fatalf("response = %s, want ignored", recorder.Body.String())
		}
	})

	t.Run("handler error returns retryable response", func(t *testing.T) {
		body := `{"header":{"event_id":"evt-1","event_type":"im.message.receive_v1","token":"verify"},"event":{"message":{"message_id":"msg-1","chat_id":"chat-1","content":"{\"text\":\"hello\"}"}}}`
		request := signedRequest(t, ingress.cfg.SigningSecret, body)
		recorder := httptest.NewRecorder()

		ingress.handleFeishuEvent(&failingIngressHandler{messageErr: errors.New("boom")}).ServeHTTP(recorder, request)

		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
		}
		if !strings.Contains(recorder.Body.String(), "retryable_error") {
			t.Fatalf("response = %s, want retryable_error", recorder.Body.String())
		}
	})
}

func TestWebhookIngressHandleCardCallbackActionResponses(t *testing.T) {
	ingress, ok := NewWebhookIngress(Config{
		VerifyToken:   "verify",
		SigningSecret: "sign-secret",
	}, nil).(*WebhookIngress)
	if !ok {
		t.Fatal("expected webhook ingress instance")
	}

	t.Run("url verification", func(t *testing.T) {
		request := signedRequest(t, ingress.cfg.SigningSecret, `{"type":"url_verification","challenge":"card-ok","token":"verify","header":{"token":"verify"}}`)
		recorder := httptest.NewRecorder()

		ingress.handleCardCallback(&captureIngressHandler{}).ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
		}
		if !strings.Contains(recorder.Body.String(), `"challenge":"card-ok"`) {
			t.Fatalf("response = %s, want challenge body", recorder.Body.String())
		}
	})

	t.Run("invalid callback returns ready toast", func(t *testing.T) {
		request := signedRequest(t, ingress.cfg.SigningSecret, `{"action":{"value":{"action_type":"permission","request_id":"perm-1","decision":"allow_all"}},"token":"verify","header":{"token":"verify"}}`)
		recorder := httptest.NewRecorder()

		ingress.handleCardCallback(&captureIngressHandler{}).ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
		}
		if !strings.Contains(recorder.Body.String(), "callback ready") {
			t.Fatalf("response = %s, want callback ready", recorder.Body.String())
		}
	})

	t.Run("permission success toast", func(t *testing.T) {
		request := signedRequest(t, ingress.cfg.SigningSecret, `{"action":{"value":{"request_id":"perm-2","decision":"allow_once"}},"token":"verify","header":{"event_id":"evt-perm","token":"verify"}}`)
		recorder := httptest.NewRecorder()
		handler := &captureIngressHandler{}

		ingress.handleCardCallback(handler).ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
		}
		if len(handler.cards) != 1 || handler.cards[0].Decision != "allow_once" {
			t.Fatalf("unexpected cards: %#v", handler.cards)
		}
		if !strings.Contains(recorder.Body.String(), "审批已提交") {
			t.Fatalf("response = %s, want permission toast", recorder.Body.String())
		}
	})

	t.Run("user question success toast", func(t *testing.T) {
		request := signedRequest(t, ingress.cfg.SigningSecret, `{"action":{"value":{"action_type":"user_question","request_id":"ask-1","status":"answered","value":"A"}},"open_message_id":"card-1","token":"verify","header":{"event_id":"evt-ask","token":"verify"}}`)
		recorder := httptest.NewRecorder()
		handler := &captureIngressHandler{}

		ingress.handleCardCallback(handler).ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
		}
		if len(handler.cards) != 1 || handler.cards[0].ActionType != "user_question" {
			t.Fatalf("unexpected cards: %#v", handler.cards)
		}
		if !strings.Contains(recorder.Body.String(), "回答已提交") {
			t.Fatalf("response = %s, want user question toast", recorder.Body.String())
		}
	})

	t.Run("handler error returns server error", func(t *testing.T) {
		request := signedRequest(t, ingress.cfg.SigningSecret, `{"action":{"value":{"request_id":"perm-3","decision":"reject"}},"token":"verify","header":{"token":"verify"}}`)
		recorder := httptest.NewRecorder()

		ingress.handleCardCallback(&failingIngressHandler{cardErr: errors.New("boom")}).ServeHTTP(recorder, request)

		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
		}
		if !strings.Contains(recorder.Body.String(), "card action failed") {
			t.Fatalf("response = %s, want card action failed", recorder.Body.String())
		}
	})
}
