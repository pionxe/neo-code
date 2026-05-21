package runtime

import (
	"context"
	"encoding/json"
	"hash/fnv"
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
	legacyTranscriptRevision := sessionLegacyTranscriptRevision(state.session)
	state.mu.Unlock()
	if sessionID == "" {
		return
	}

	resume, err := s.checkpointStore.GetLatestResumeCheckpoint(ctx, sessionID)
	if err != nil || resume == nil {
		return
	}
	if !resumeCheckpointMatchesState(*resume, workspaceKey, transcriptRevision, legacyTranscriptRevision) {
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
	currentPlanID := ""
	currentPlanRevision := 0
	if session.CurrentPlan != nil {
		currentPlanID = strings.TrimSpace(session.CurrentPlan.ID)
		currentPlanRevision = session.CurrentPlan.Revision
	}

	snapshot := resumeConsistencySnapshot{
		MessageCount:                    len(session.Messages),
		TodoVersion:                     session.TodoVersion,
		Todos:                           buildResumeTodoFingerprints(session.Todos),
		TaskState:                       buildResumeTaskStateFingerprint(session.TaskState),
		CurrentPlanID:                   currentPlanID,
		CurrentPlanRevision:             currentPlanRevision,
		LastFullPlanRevision:            session.LastFullPlanRevision,
		PlanApprovalPendingFullAlign:    session.PlanApprovalPendingFullAlign,
		PlanCompletionPendingFullReview: session.PlanCompletionPendingFullReview,
		PlanContextDirty:                session.PlanContextDirty,
		PlanRestorePendingAlign:         session.PlanRestorePendingAlign,
	}
	// snapshot 字段仅由基础可序列化类型组成，Marshal 对该结构是稳定可达的。
	raw, _ := json.Marshal(snapshot)
	hasher := fnv.New64a()
	// fnv.Hash Write 对内存写入不会返回错误，这里忽略 error 以保持实现简洁。
	_, _ = hasher.Write(raw)
	const positiveInt64Mask = uint64(1<<63 - 1)
	return int64(hasher.Sum64() & positiveInt64Mask)
}

// sessionLegacyTranscriptRevision 返回升级前使用的 transcript 版本语义：消息条数。
func sessionLegacyTranscriptRevision(session agentsession.Session) int64 {
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
	legacyTranscriptRevision int64,
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
	if resume.TranscriptRevision == currentTranscriptRevision {
		return true
	}
	// 兼容升级前 checkpoint：旧语义使用 len(messages)。
	if legacyTranscriptRevision < 0 {
		return false
	}
	return resume.TranscriptRevision == legacyTranscriptRevision
}

// resumeConsistencySnapshot 用于构建 resume 一致性指纹，避免仅靠消息数导致的误命中。
type resumeConsistencySnapshot struct {
	MessageCount                    int                        `json:"message_count"`
	TodoVersion                     int                        `json:"todo_version"`
	Todos                           []resumeTodoFingerprint    `json:"todos,omitempty"`
	TaskState                       resumeTaskStateFingerprint `json:"task_state"`
	CurrentPlanID                   string                     `json:"current_plan_id,omitempty"`
	CurrentPlanRevision             int                        `json:"current_plan_revision,omitempty"`
	LastFullPlanRevision            int                        `json:"last_full_plan_revision,omitempty"`
	PlanApprovalPendingFullAlign    bool                       `json:"plan_approval_pending_full_align,omitempty"`
	PlanCompletionPendingFullReview bool                       `json:"plan_completion_pending_full_review,omitempty"`
	PlanContextDirty                bool                       `json:"plan_context_dirty,omitempty"`
	PlanRestorePendingAlign         bool                       `json:"plan_restore_pending_align,omitempty"`
}

// resumeTodoFingerprint 收敛 resume 判定所需的最小 todo 状态。
type resumeTodoFingerprint struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	Required      bool   `json:"required"`
	OwnerType     string `json:"owner_type,omitempty"`
	OwnerID       string `json:"owner_id,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`
	BlockedReason string `json:"blocked_reason,omitempty"`
	Revision      int64  `json:"revision"`
	UpdatedAtMS   int64  `json:"updated_at_ms"`
}

// resumeTaskStateFingerprint 收敛 resume 判定所需的最小 task_state 摘要。
type resumeTaskStateFingerprint struct {
	VerificationProfile string   `json:"verification_profile,omitempty"`
	Goal                string   `json:"goal,omitempty"`
	Progress            []string `json:"progress,omitempty"`
	OpenItems           []string `json:"open_items,omitempty"`
	NextStep            string   `json:"next_step,omitempty"`
	Blockers            []string `json:"blockers,omitempty"`
	KeyArtifacts        []string `json:"key_artifacts,omitempty"`
	Decisions           []string `json:"decisions,omitempty"`
	UserConstraints     []string `json:"user_constraints,omitempty"`
	LastUpdatedAtMS     int64    `json:"last_updated_at_ms"`
}

// buildResumeTodoFingerprints 生成 resume 一致性计算所需的 todo 指纹切片。
func buildResumeTodoFingerprints(items []agentsession.TodoItem) []resumeTodoFingerprint {
	if len(items) == 0 {
		return nil
	}
	result := make([]resumeTodoFingerprint, 0, len(items))
	for _, item := range items {
		result = append(result, resumeTodoFingerprint{
			ID:            strings.TrimSpace(item.ID),
			Status:        strings.TrimSpace(string(item.Status)),
			Required:      item.RequiredValue(),
			OwnerType:     strings.TrimSpace(item.OwnerType),
			OwnerID:       strings.TrimSpace(item.OwnerID),
			FailureReason: strings.TrimSpace(item.FailureReason),
			BlockedReason: strings.TrimSpace(string(item.BlockedReason)),
			Revision:      item.Revision,
			UpdatedAtMS:   item.UpdatedAt.UnixMilli(),
		})
	}
	return result
}

// buildResumeTaskStateFingerprint 生成 resume 一致性计算所需的 task_state 指纹。
func buildResumeTaskStateFingerprint(state agentsession.TaskState) resumeTaskStateFingerprint {
	return resumeTaskStateFingerprint{
		VerificationProfile: strings.TrimSpace(string(state.VerificationProfile)),
		Goal:                strings.TrimSpace(state.Goal),
		Progress:            cloneTrimmedStringList(state.Progress),
		OpenItems:           cloneTrimmedStringList(state.OpenItems),
		NextStep:            strings.TrimSpace(state.NextStep),
		Blockers:            cloneTrimmedStringList(state.Blockers),
		KeyArtifacts:        cloneTrimmedStringList(state.KeyArtifacts),
		Decisions:           cloneTrimmedStringList(state.Decisions),
		UserConstraints:     cloneTrimmedStringList(state.UserConstraints),
		LastUpdatedAtMS:     state.LastUpdatedAt.UnixMilli(),
	}
}

// cloneTrimmedStringList 复制并清洗字符串切片，保证指纹输入稳定。
func cloneTrimmedStringList(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, strings.TrimSpace(item))
	}
	return result
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
