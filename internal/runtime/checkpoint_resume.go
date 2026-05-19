package runtime

import (
	"context"
	"log"
	"strings"
	"time"

	"neo-code/internal/runtime/controlplane"
	agentsession "neo-code/internal/session"
)

const (
	resumeStrategyReplayPlan         = "replay_plan"
	resumeStrategyVerifyClosureFirst = "resume_verify_closure"
)

// updateResumeCheckpoint 在 phase 转换时写入或更新 ResumeCheckpoint。
// 失败仅 log，不阻塞主流程。
func (s *Service) updateResumeCheckpoint(ctx context.Context, state *runState, phase string, completionState string) {
	if s.checkpointStore == nil {
		return
	}

	state.mu.Lock()
	session := state.session
	runID := state.runID
	turn := state.turn
	state.mu.Unlock()

	rc := agentsession.ResumeCheckpoint{
		ID:              agentsession.NewID("rc"),
		WorkspaceKey:    agentsession.WorkspacePathKey(session.Workdir),
		RunID:           runID,
		SessionID:       session.ID,
		Turn:            turn,
		Phase:           phase,
		CompletionState: completionState,
		UpdatedAt:       time.Now(),
	}

	if err := s.checkpointStore.SetResumeCheckpoint(ctx, rc); err != nil {
		log.Printf("checkpoint: set resume checkpoint for %s: %v", session.ID, err)
	}
}

// applyResumeCheckpoint 在 run 启动时应用最新的 resume checkpoint 策略。
func (s *Service) applyResumeCheckpoint(ctx context.Context, state *runState) {
	if s == nil || state == nil || s.checkpointStore == nil {
		return
	}

	state.mu.Lock()
	sessionID := strings.TrimSpace(state.session.ID)
	state.mu.Unlock()
	if sessionID == "" {
		return
	}

	resume, err := s.checkpointStore.GetLatestResumeCheckpoint(ctx, sessionID)
	if err != nil || resume == nil {
		return
	}

	phase := strings.ToLower(strings.TrimSpace(resume.Phase))
	completionState := strings.ToLower(strings.TrimSpace(resume.CompletionState))

	strategy := ""
	reminder := ""
	override := deriveResumeBaseLifecycle(phase, completionState)
	switch override {
	case controlplane.RunStateVerify:
		strategy = resumeStrategyVerifyClosureFirst
		reminder = "恢复提示：上一轮已完成工具执行，请优先验证并收尾，仅在证据不足时再调用工具。"
	case controlplane.RunStatePlan:
		strategy = resumeStrategyReplayPlan
		reminder = "恢复提示：检测到上一轮未完整结束，请先梳理当前状态再继续执行，避免重复危险操作。"
	default:
		return
	}

	state.mu.Lock()
	state.pendingSystemReminder = strings.TrimSpace(reminder)
	state.resumeNextBaseLifecycle = override
	state.mu.Unlock()

	s.emitRunScopedOptional(EventResumeApplied, state, ResumeAppliedPayload{
		CheckpointRunID: strings.TrimSpace(resume.RunID),
		CheckpointPhase: phase,
		CheckpointTurn:  resume.Turn,
		Strategy:        strategy,
	})
	s.emitRuntimeSnapshotUpdated(ctx, state, "resume_applied")
}

// deriveResumeBaseLifecycle 将 checkpoint phase/completion_state 映射为恢复时首轮运行态。
func deriveResumeBaseLifecycle(phase string, completionState string) controlplane.RunState {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case "verify":
		if strings.EqualFold(strings.TrimSpace(completionState), "completed") {
			return controlplane.RunStateVerify
		}
		return controlplane.RunStatePlan
	case "plan", "execute":
		return controlplane.RunStatePlan
	default:
		return ""
	}
}
