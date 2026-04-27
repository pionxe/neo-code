package runtime

import (
	"context"
	"strings"

	providertypes "neo-code/internal/provider/types"
	"neo-code/internal/runtime/acceptance"
	"neo-code/internal/runtime/verify"
	agentsession "neo-code/internal/session"
)

const finalContinueReminder = "There are unfinished required todos or unmet acceptance checks. Continue execution. Do not finalize yet."

// beforeAcceptFinal 在 runtime 接受模型 final 前执行唯一的 completion/verifier/acceptance 闭环。
func (s *Service) beforeAcceptFinal(
	ctx context.Context,
	state *runState,
	snapshot TurnBudgetSnapshot,
	assistant providertypes.Message,
	completionPassed bool,
) (acceptance.AcceptanceDecision, error) {
	if state == nil {
		return acceptance.AcceptanceDecision{}, nil
	}

	verificationCfg := snapshot.Config.Runtime.Verification.Clone()
	policy := acceptance.DefaultPolicy{
		Executor: verify.PolicyCommandExecutor{},
	}
	engine := acceptance.NewEngine(policy)

	maxNoProgress := verificationCfg.MaxNoProgress
	if maxNoProgress <= 0 {
		maxNoProgress = 3
	}
	noProgressStreak := state.finalInterceptStreak
	if noProgressStreak < 0 {
		noProgressStreak = 0
	}
	maxTurnsLimit := state.maxTurnsLimit
	maxTurnsReached := state.maxTurnsReached
	if !maxTurnsReached {
		resolvedMaxTurns := resolveRuntimeMaxTurns(snapshot.Config.Runtime)
		if resolvedMaxTurns > 0 && state.turn+1 >= resolvedMaxTurns {
			maxTurnsReached = true
			maxTurnsLimit = resolvedMaxTurns
		}
	}

	input := acceptance.FinalAcceptanceInput{
		CompletionGate: acceptance.CompletionGateDecision{
			Passed: completionPassed,
			Reason: string(state.completion.CompletionBlockedReason),
		},
		VerificationInput: verify.FinalVerifyInput{
			SessionID:          state.session.ID,
			RunID:              state.runID,
			TaskID:             state.taskID,
			Workdir:            snapshot.Workdir,
			Messages:           buildVerifyMessages(state.session.Messages),
			Todos:              buildVerifyTodos(state.session.Todos),
			LastAssistantFinal: renderPartsForVerification(assistant.Parts),
			ToolResults:        nil,
			TaskState:          buildVerifyTaskState(state.session.TaskState),
			RuntimeState: verify.RuntimeStateSnapshot{
				Turn:                 state.turn,
				MaxTurns:             resolveRuntimeMaxTurns(snapshot.Config.Runtime),
				MaxTurnsReached:      maxTurnsReached,
				FinalInterceptStreak: noProgressStreak,
			},
			VerificationConfig: verificationCfg,
		},
		NoProgressExceeded: noProgressStreak >= maxNoProgress,
		MaxTurnsReached:    maxTurnsReached,
		MaxTurnsLimit:      maxTurnsLimit,
	}

	decision, err := engine.EvaluateFinal(ctx, input)
	if err != nil {
		return acceptance.AcceptanceDecision{}, err
	}
	if decision.Status == acceptance.AcceptanceContinue && state.pendingFinalProgress {
		decision.HasProgress = true
	}
	return decision, nil
}

// recordAcceptanceTerminal 将 acceptance 输出映射为 runtime 唯一终态记录。
func recordAcceptanceTerminal(state *runState, decision acceptance.AcceptanceDecision) {
	if state == nil {
		return
	}
	status := acceptance.TerminalStatusFromAcceptance(decision.Status)
	state.markTerminalDecision(status, decision.StopReason, strings.TrimSpace(decision.InternalSummary))
}

// buildVerifyTodos 将 session todo 转换为 verifier 快照。
func buildVerifyTodos(items []agentsession.TodoItem) []verify.TodoSnapshot {
	if len(items) == 0 {
		return nil
	}
	todos := make([]verify.TodoSnapshot, 0, len(items))
	for _, item := range items {
		todos = append(todos, verify.TodoSnapshot{
			ID:            strings.TrimSpace(item.ID),
			Content:       strings.TrimSpace(item.Content),
			Status:        strings.TrimSpace(string(item.Status)),
			Required:      item.RequiredValue(),
			BlockedReason: string(item.BlockedReasonValue()),
			Acceptance:    append([]string(nil), item.Acceptance...),
			Artifacts:     append([]string(nil), item.Artifacts...),
			Supersedes:    append([]string(nil), item.Supersedes...),
			ContentChecks: buildVerifyTodoContentChecks(item.ContentChecks),
			RetryCount:    item.RetryCount,
			RetryLimit:    item.RetryLimit,
			FailureReason: strings.TrimSpace(item.FailureReason),
		})
	}
	return todos
}

// buildVerifyTodoContentChecks 将 session 内容校验规则转换为 verifier 快照。
func buildVerifyTodoContentChecks(items []agentsession.TodoContentCheck) []verify.TodoContentCheckSnapshot {
	if len(items) == 0 {
		return nil
	}
	checks := make([]verify.TodoContentCheckSnapshot, 0, len(items))
	for _, item := range items {
		checks = append(checks, verify.TodoContentCheckSnapshot{
			Artifact: strings.TrimSpace(item.Artifact),
			Contains: append([]string(nil), item.Contains...),
		})
	}
	return checks
}

// buildVerifyTaskState 将 task_state 中与验收相关的结构化字段投影给 verifier。
func buildVerifyTaskState(state agentsession.TaskState) verify.TaskStateSnapshot {
	return verify.TaskStateSnapshot{
		VerificationProfile: string(state.VerificationProfile),
		KeyArtifacts:        append([]string(nil), state.KeyArtifacts...),
	}
}

// buildVerifyMessages 将会话消息压缩为 verifier 所需的最小快照。
func buildVerifyMessages(messages []providertypes.Message) []verify.MessageLike {
	if len(messages) == 0 {
		return nil
	}
	out := make([]verify.MessageLike, 0, len(messages))
	for _, message := range messages {
		out = append(out, verify.MessageLike{
			Role:    strings.TrimSpace(message.Role),
			Content: renderPartsForVerification(message.Parts),
		})
	}
	return out
}

// renderPartsForVerification 将消息分片合并为 verifier 侧可读文本。
func renderPartsForVerification(parts []providertypes.ContentPart) string {
	if len(parts) == 0 {
		return ""
	}
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Kind != providertypes.ContentPartText {
			continue
		}
		text := strings.TrimSpace(part.Text)
		if text == "" {
			continue
		}
		segments = append(segments, text)
	}
	return strings.Join(segments, "\n")
}

// applyAcceptanceResultProgress 根据 acceptance 输出更新 final 拦截计数唯一真相源。
func applyAcceptanceResultProgress(state *runState, decision acceptance.AcceptanceDecision) {
	if state == nil {
		return
	}
	switch decision.Status {
	case acceptance.AcceptanceContinue:
		if state.pendingFinalProgress {
			state.finalInterceptStreak = 0
		} else {
			state.finalInterceptStreak++
		}
	default:
		state.finalInterceptStreak = 0
	}
	state.pendingFinalProgress = false
}
