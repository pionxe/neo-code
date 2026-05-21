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
	effectiveWorkdir := strings.TrimSpace(state.effectiveWorkdir)
	state.mu.Unlock()

	rc := agentsession.ResumeCheckpoint{
		ID:                 agentsession.NewID("rc"),
		WorkspaceKey:       resolveResumeWorkspaceKey(session.Workdir, effectiveWorkdir),
		RunID:              runID,
		SessionID:          session.ID,
		Turn:               turn,
		Phase:              phase,
		CompletionState:    completionState,
		TranscriptRevision: sessionTranscriptRevision(session),
		UpdatedAt:          time.Now(),
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
	workspaceKey := resolveResumeWorkspaceKey(state.session.Workdir, state.effectiveWorkdir)
	transcriptRevision := sessionTranscriptRevision(state.session)
	state.mu.Unlock()
	if sessionID == "" {
		return
	}

	resume, err := s.checkpointStore.GetLatestResumeCheckpoint(ctx, sessionID)
	if err != nil || resume == nil {
		return
	}
	if !resumeCheckpointMatchesState(*resume, workspaceKey, transcriptRevision) {
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

// sessionTranscriptRevision 返回当前会话 transcript 的逻辑版本号，供 resume checkpoint 一致性校验使用。
func sessionTranscriptRevision(session agentsession.Session) int64 {
	return int64(len(session.Messages))
}

// resolveResumeWorkspaceKey 统一计算 resume checkpoint 的工作区比较键，优先会话 workdir，缺失时回退运行时生效目录。
func resolveResumeWorkspaceKey(sessionWorkdir string, effectiveWorkdir string) string {
	workdir := strings.TrimSpace(sessionWorkdir)
	if workdir == "" {
		workdir = strings.TrimSpace(effectiveWorkdir)
	}
	return agentsession.WorkspacePathKey(workdir)
}

// resumeCheckpointMatchesState 校验 resume checkpoint 是否仍与当前会话工作区/转录版本一致。
func resumeCheckpointMatchesState(
	resume agentsession.ResumeCheckpoint,
	currentWorkspaceKey string,
	currentTranscriptRevision int64,
) bool {
	resumeWorkspaceKey := strings.TrimSpace(resume.WorkspaceKey)
	workspaceKey := strings.TrimSpace(currentWorkspaceKey)
	if resumeWorkspaceKey == "" || workspaceKey == "" {
		return false
	}
	if !strings.EqualFold(resumeWorkspaceKey, workspaceKey) {
		return false
	}

	if resume.TranscriptRevision < 0 || currentTranscriptRevision < 0 {
		return false
	}
	return resume.TranscriptRevision == currentTranscriptRevision
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
