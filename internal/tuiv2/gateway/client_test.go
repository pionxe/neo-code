package gateway

import "testing"

func TestEventTypeValues(t *testing.T) {
	tests := map[EventType]string{
		EventSessionUpdated:        "session_updated",
		EventRunStarted:            "run_started",
		EventRunFinished:           "run_finished",
		EventRunCancelled:          "run_cancelled",
		EventAssistantDelta:        "assistant_delta",
		EventToolStarted:           "tool_started",
		EventToolFinished:          "tool_finished",
		EventPermissionRequested:   "permission_requested",
		EventUserQuestionRequested: "user_question_requested",
		EventModelChanged:          "model_changed",
		EventGatewayOffline:        "gateway_offline",
		EventError:                 "error",
	}

	for got, want := range tests {
		if string(got) != want {
			t.Fatalf("event value = %q, want %q", got, want)
		}
	}
}

func TestRealClientSatisfiesClient(t *testing.T) {
	var client Client = NewRealClient()
	if client == nil {
		t.Fatal("NewRealClient() = nil")
	}
}
