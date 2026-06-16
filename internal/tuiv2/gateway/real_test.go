package gateway

import (
	"context"
	"errors"
	"testing"
)

// RealClient 是 Phase 20 占位，所有方法应返回保留错误（Close 除外，返回 nil）。
func TestRealClientReservedErrors(t *testing.T) {
	c := NewRealClient()
	ctx := context.Background()

	checks := []struct {
		name string
		fn   func() error
	}{
		{"Health", func() error { _, err := c.Health(ctx); return err }},
		{"ListSessions", func() error { _, err := c.ListSessions(ctx); return err }},
		{"LoadSession", func() error { _, err := c.LoadSession(ctx, "s"); return err }},
		{"CreateSession", func() error { _, err := c.CreateSession(ctx); return err }},
		{"SendMessage", func() error { _, err := c.SendMessage(ctx, "s", "hi"); return err }},
		{"CancelRun", func() error { return c.CancelRun(ctx, "s", "r") }},
		{"SubscribeEvents", func() error { _, err := c.SubscribeEvents(ctx, "s"); return err }},
		{"ResolvePermission", func() error { return c.ResolvePermission(ctx, PermissionDecision{}) }},
		{"AnswerUserQuestion", func() error { return c.AnswerUserQuestion(ctx, UserQuestionAnswer{}) }},
		{"ListModels", func() error { _, err := c.ListModels(ctx); return err }},
		{"SetModel", func() error { return c.SetModel(ctx, "s", "m") }},
		{"GetModel", func() error { _, err := c.GetModel(ctx, "s"); return err }},
	}
	for _, ch := range checks {
		if err := ch.fn(); !errors.Is(err, errRealClientReserved) {
			t.Fatalf("%s should return errRealClientReserved, got %v", ch.name, err)
		}
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close should return nil, got %v", err)
	}
}
