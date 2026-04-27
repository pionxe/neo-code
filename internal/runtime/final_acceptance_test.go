package runtime

import (
	"context"
	"testing"

	"neo-code/internal/config"
	providertypes "neo-code/internal/provider/types"
	"neo-code/internal/runtime/acceptance"
	agentsession "neo-code/internal/session"
)

func TestBeforeAcceptFinalDecisionPaths(t *testing.T) {
	t.Parallel()

	service := &Service{}
	baseCfg := config.StaticDefaults().Clone()
	snapshot := TurnBudgetSnapshot{
		Config:  baseCfg,
		Workdir: t.TempDir(),
	}

	t.Run("pending required todo -> continue", func(t *testing.T) {
		t.Parallel()
		state := newRunState("run-continue", agentsession.New("continue"))
		required := true
		state.session.TaskState.VerificationProfile = agentsession.VerificationProfileTaskOnly
		state.session.Todos = []agentsession.TodoItem{
			{ID: "todo-1", Content: "do work", Status: agentsession.TodoStatusPending, Required: &required},
		}
		decision, err := service.beforeAcceptFinal(context.Background(), &state, snapshot, providertypes.Message{
			Role:  providertypes.RoleAssistant,
			Parts: []providertypes.ContentPart{providertypes.NewTextPart("done")},
		}, true)
		if err != nil {
			t.Fatalf("beforeAcceptFinal() error = %v", err)
		}
		if decision.Status != acceptance.AcceptanceContinue {
			t.Fatalf("status = %q, want continue", decision.Status)
		}
	})

	t.Run("invalid profile -> failed", func(t *testing.T) {
		t.Parallel()
		state := newRunState("run-invalid-profile", agentsession.New("invalid-profile"))
		state.session.TaskState.VerificationProfile = "bad"
		decision, err := service.beforeAcceptFinal(context.Background(), &state, snapshot, providertypes.Message{}, true)
		if err != nil {
			t.Fatalf("beforeAcceptFinal() error = %v", err)
		}
		if decision.Status != acceptance.AcceptanceFailed {
			t.Fatalf("status = %q, want failed", decision.Status)
		}
	})

	t.Run("continue carries pending final progress signal", func(t *testing.T) {
		t.Parallel()
		state := newRunState("run-progress", agentsession.New("progress"))
		required := true
		state.pendingFinalProgress = true
		state.session.TaskState.VerificationProfile = agentsession.VerificationProfileTaskOnly
		state.session.Todos = []agentsession.TodoItem{
			{ID: "todo-1", Content: "do work", Status: agentsession.TodoStatusPending, Required: &required},
		}
		decision, err := service.beforeAcceptFinal(context.Background(), &state, snapshot, providertypes.Message{}, true)
		if err != nil {
			t.Fatalf("beforeAcceptFinal() error = %v", err)
		}
		if !decision.HasProgress {
			t.Fatal("expected continue decision to carry pending final progress")
		}
	})

	t.Run("all converged -> accepted", func(t *testing.T) {
		t.Parallel()
		state := newRunState("run-accepted", agentsession.New("accepted"))
		state.session.TaskState.VerificationProfile = agentsession.VerificationProfileTaskOnly
		decision, err := service.beforeAcceptFinal(context.Background(), &state, snapshot, providertypes.Message{}, true)
		if err != nil {
			t.Fatalf("beforeAcceptFinal() error = %v", err)
		}
		if decision.Status != acceptance.AcceptanceAccepted {
			t.Fatalf("status = %q, want accepted", decision.Status)
		}
	})

	t.Run("final intercept streak drives no-progress breaker", func(t *testing.T) {
		t.Parallel()
		state := newRunState("run-incomplete", agentsession.New("incomplete"))
		required := true
		state.finalInterceptStreak = snapshot.Config.Runtime.Verification.MaxNoProgress
		state.session.TaskState.VerificationProfile = agentsession.VerificationProfileTaskOnly
		state.session.Todos = []agentsession.TodoItem{
			{ID: "todo-1", Content: "do work", Status: agentsession.TodoStatusPending, Required: &required},
		}
		decision, err := service.beforeAcceptFinal(context.Background(), &state, snapshot, providertypes.Message{}, true)
		if err != nil {
			t.Fatalf("beforeAcceptFinal() error = %v", err)
		}
		if decision.Status != acceptance.AcceptanceIncomplete {
			t.Fatalf("status = %q, want incomplete", decision.Status)
		}
	})
}

func TestFinalAcceptanceHelpers(t *testing.T) {
	t.Parallel()

	t.Run("buildVerifyTaskState includes profile", func(t *testing.T) {
		t.Parallel()
		got := buildVerifyTaskState(agentsession.TaskState{
			VerificationProfile: agentsession.VerificationProfileDocs,
			KeyArtifacts:        []string{"README.md"},
		})
		if got.VerificationProfile != "docs" || len(got.KeyArtifacts) != 1 {
			t.Fatalf("unexpected task state snapshot: %+v", got)
		}
	})

	t.Run("applyAcceptanceResultProgress uses pending final progress", func(t *testing.T) {
		t.Parallel()
		state := newRunState("run-progress", agentsession.New("progress"))
		state.finalInterceptStreak = 2
		state.pendingFinalProgress = true
		applyAcceptanceResultProgress(&state, acceptance.AcceptanceDecision{Status: acceptance.AcceptanceContinue})
		if state.finalInterceptStreak != 0 || state.pendingFinalProgress {
			t.Fatalf("unexpected state after progress reset: %+v", state)
		}

		applyAcceptanceResultProgress(&state, acceptance.AcceptanceDecision{Status: acceptance.AcceptanceContinue})
		if state.finalInterceptStreak != 1 {
			t.Fatalf("streak = %d, want 1", state.finalInterceptStreak)
		}
	})
}
