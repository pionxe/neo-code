package fakegateway

import (
	"context"
	"testing"

	"neo-code/internal/tuiv2/gateway"
)

func TestNewRejectsUnknownScenario(t *testing.T) {
	if _, err := New(Config{Scenario: "missing"}); err == nil {
		t.Fatal("New() error = nil, want error")
	}
}

func TestClientHealthReflectsScenario(t *testing.T) {
	client := mustClient(t, ScenarioGatewayOffline)

	health, err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if health.OK {
		t.Fatal("Health().OK = true, want false")
	}
	if health.Status != "offline" {
		t.Fatalf("Health().Status = %q, want offline", health.Status)
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
	if !ack.Accepted || ack.SessionID != session.ID {
		t.Fatalf("SendMessage() ack = %+v", ack)
	}

	model, err := client.GetModel(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetModel() error = %v", err)
	}
	if model != "fake" {
		t.Fatalf("GetModel() = %q, want fake", model)
	}
}

func mustClient(t *testing.T, scenario string) gateway.Client {
	t.Helper()
	client, err := New(Config{Scenario: scenario})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return client
}
