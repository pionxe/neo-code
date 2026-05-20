package runtime

import (
	"context"

	agentsession "neo-code/internal/session"
)

const todoBootstrapRequiredReason = "todo_bootstrap_required"

const todoBootstrapRequiredReminder = `[Runtime Control]

todo_bootstrap_required: This build run has no active todos.

Before project analysis, documentation writing, code changes, multi-step debugging, or verification work, call todo_write with action=plan or action=add to create required todos for this run.

If a Current Plan is attached, use it only as planning context. Create current-run execution todos explicitly instead of assuming plan steps already exist as todos.

Do not update or complete old todo IDs that are not present in the current Todo State.`

// maybeAppendTodoBootstrapReminder 在 build 缺少执行态 todo 时注入一次结构化提醒。
func (s *Service) maybeAppendTodoBootstrapReminder(ctx context.Context, state *runState) error {
	if !shouldInjectTodoBootstrapReminder(state) {
		return nil
	}
	return s.appendSystemMessageAndSave(ctx, state, todoBootstrapRequiredReminder)
}

// shouldInjectTodoBootstrapReminder 判断本轮 build 是否需要先创建当前 run 的 todo。
func shouldInjectTodoBootstrapReminder(state *runState) bool {
	if state == nil || state.disableTools || !state.planningEnabled {
		return false
	}
	state.mu.Lock()
	session := state.session
	state.mu.Unlock()

	if agentsession.NormalizeAgentMode(session.AgentMode) != agentsession.AgentModeBuild {
		return false
	}
	if hasActiveTodoForBootstrap(session.Todos) {
		return false
	}
	return true
}

// hasActiveTodoForBootstrap 判断会话中是否已有可继续推进的非终态 todo。
func hasActiveTodoForBootstrap(todos []agentsession.TodoItem) bool {
	for _, todo := range todos {
		if !todo.Status.IsTerminal() {
			return true
		}
	}
	return false
}

const planBootstrapRequiredReason = "plan_bootstrap_required"

const planBootstrapRequiredReminder = `[Runtime Control]

plan_bootstrap_required: You are in plan mode but no current plan exists.

Before research, analysis, or conversational response, you MUST complete the following:

1. Research the codebase as needed using read-only tools.
2. Output a JSON object containing "plan_spec" and "summary_candidate" that defines the current plan.
3. Focus plan_spec on goal, steps, constraints, and open_questions. Do not create execution todos in plan mode.

Do not end this turn without producing a plan.`

// maybeAppendPlanBootstrapReminder 在 plan 模式缺少 CurrentPlan 时注入一次结构化提醒。
func (s *Service) maybeAppendPlanBootstrapReminder(ctx context.Context, state *runState) error {
	if !shouldInjectPlanBootstrapReminder(state) {
		return nil
	}
	return s.appendSystemMessageAndSave(ctx, state, planBootstrapRequiredReminder)
}

// shouldInjectPlanBootstrapReminder 判断本轮 plan 模式是否需要先创建 plan。
func shouldInjectPlanBootstrapReminder(state *runState) bool {
	if state == nil || state.disableTools || !state.planningEnabled {
		return false
	}
	if resolvePlanningStageForState(state) != planStagePlan {
		return false
	}
	state.mu.Lock()
	plan := state.session.CurrentPlan
	state.mu.Unlock()
	return plan == nil
}
