package fakegateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"neo-code/internal/tuiv2/gateway"
)

func TestNewRejectsUnknownScenario(t *testing.T) {
	if _, err := New(Config{Scenario: "missing"}); err == nil {
		t.Fatal("New() error = nil, want error")
	}
}

func TestScenariosAreAvailable(t *testing.T) {
	scenarios := []string{
		ScenarioDefault,
		ScenarioEmptySessions,
		ScenarioStreamingChat,
		ScenarioToolApproval,
		ScenarioToolFailed,
		ScenarioGatewayOffline,
		ScenarioRuntimeError,
		ScenarioLongOutput,
		ScenarioManySessions,
		ScenarioSmallTerminal,
		ScenarioSlowGateway,
		ScenarioAskUser,
		ScenarioCancelRunning,
	}

	for _, scenario := range scenarios {
		t.Run(scenario, func(t *testing.T) {
			if !IsKnownScenario(scenario) {
				t.Fatalf("IsKnownScenario(%q) = false", scenario)
			}
			if _, err := NewFakeClient(scenario); err != nil {
				t.Fatalf("NewFakeClient(%q) error = %v", scenario, err)
			}
		})
	}
}

func TestGatewayOfflineHealthReturnsError(t *testing.T) {
	client := mustClient(t, ScenarioGatewayOffline)

	health, err := client.Health(context.Background())
	if !errors.Is(err, errGatewayOffline) {
		t.Fatalf("Health() error = %v, want %v", err, errGatewayOffline)
	}
	if health != nil {
		t.Fatalf("Health() result = %+v, want nil", health)
	}
}

func TestClientMinimalMethods(t *testing.T) {
	client := mustClient(t, ScenarioDefault)

	session, err := client.CreateSession(context.Background())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if session.ID == "" {
		t.Fatal("CreateSession().ID is empty")
	}

	ack, err := client.SendMessage(context.Background(), session.ID, "hello")
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if !ack.Accepted || ack.SessionID != session.ID || ack.Message != "hello" {
		t.Fatalf("SendMessage() ack = %+v", ack)
	}

	if err := client.SetModel(context.Background(), session.ID, "neo-fake-fast"); err != nil {
		t.Fatalf("SetModel() error = %v", err)
	}
	model, err := client.GetModel(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetModel() error = %v", err)
	}
	if model != "neo-fake-fast" {
		t.Fatalf("GetModel() = %q, want neo-fake-fast", model)
	}
}

func TestStreamingChatEventsArriveInOrder(t *testing.T) {
	client := mustClient(t, ScenarioStreamingChat)
	events, err := client.SubscribeEvents(context.Background(), defaultSessionID)
	if err != nil {
		t.Fatalf("SubscribeEvents() error = %v", err)
	}

	var got []gateway.EventType
	for event := range events {
		got = append(got, event.Type)
	}
	if len(got) < 4 {
		t.Fatalf("received %d events, want at least 4", len(got))
	}
	if got[0] != gateway.EventRunStarted {
		t.Fatalf("first event = %q, want %q", got[0], gateway.EventRunStarted)
	}
	if got[len(got)-1] != gateway.EventRunFinished {
		t.Fatalf("last event = %q, want %q", got[len(got)-1], gateway.EventRunFinished)
	}
	// Verify we have some agent chunks in the middle (event types expanded in Phase 9)
	hasChunks := false
	for _, eventType := range got[1 : len(got)-1] {
		if eventType == gateway.EventAgentChunk {
			hasChunks = true
		}
	}
	if !hasChunks {
		t.Fatalf("streaming events must include at least one agent_chunk")
	}
}

func TestSlowGatewayAppliesRPCDelay(t *testing.T) {
	client := mustClient(t, ScenarioSlowGateway)
	start := time.Now()
	if _, err := client.ListSessions(context.Background()); err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if elapsed := time.Since(start); elapsed < 70*time.Millisecond {
		t.Fatalf("ListSessions() delay = %v, want at least 70ms", elapsed)
	}
}

func TestSubscribeEventsStopsOnContextCancel(t *testing.T) {
	client := mustClient(t, ScenarioStreamingChat)
	ctx, cancel := context.WithCancel(context.Background())
	events, err := client.SubscribeEvents(ctx, defaultSessionID)
	if err != nil {
		t.Fatalf("SubscribeEvents() error = %v", err)
	}
	cancel()
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("events channel still open after context cancel")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for events channel to close")
	}
}

func TestCloseRejectsFurtherRPC(t *testing.T) {
	client := mustClient(t, ScenarioDefault)
	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	_, err := client.ListSessions(context.Background())
	if !errors.Is(err, errClientClosed) {
		t.Fatalf("ListSessions() error = %v, want %v", err, errClientClosed)
	}
}

func mustClient(t *testing.T, scenario string) gateway.Client {
	t.Helper()
	client, err := NewFakeClient(scenario)
	if err != nil {
		t.Fatalf("NewFakeClient() error = %v", err)
	}
	return client
}
