package controlplane

import "fmt"

// RunState 表示单次 Run 生命周期中的显式运行态，统一承载主链 phase 与外围治理态。
type RunState string

const (
	// RunStatePlan 表示规划阶段：构建上下文并驱动 provider 产出 assistant 决策。
	RunStatePlan RunState = "plan"
	// RunStateExecute 表示执行阶段：执行本轮 assistant 产生的全部工具调用。
	RunStateExecute RunState = "execute"
	// RunStateVerify 表示验证阶段：工具结果已回灌，等待下一轮模型收尾或继续推进。
	RunStateVerify RunState = "verify"
	// RunStateCompacting 表示当前正在执行 compact 或 reactive compact。
	RunStateCompacting RunState = "compacting"
	// RunStateWaitingUserQuestion 表示当前正在等待 ask_user 的用户回答，执行流显式挂起。
	RunStateWaitingUserQuestion RunState = "waiting_user_question"
	// RunStateWaitingPermission 表示当前正在等待权限决议，执行流被显式挂起。
	RunStateWaitingPermission RunState = "waiting_permission"
	// RunStateStopped 表示本次 Run 已完成终止决议，不再继续推进生命周期。
	RunStateStopped RunState = "stopped"
)

var allowedRunStateTransitions = map[RunState]map[RunState]struct{}{
	"": {
		RunStatePlan: {},
	},
	RunStatePlan: {
		RunStatePlan:                {},
		RunStateVerify:              {},
		RunStateExecute:             {},
		RunStateCompacting:          {},
		RunStateWaitingUserQuestion: {},
		RunStateWaitingPermission:   {},
		RunStateStopped:             {},
	},
	RunStateExecute: {
		RunStateExecute:             {},
		RunStateVerify:              {},
		RunStateCompacting:          {},
		RunStateWaitingUserQuestion: {},
		RunStateWaitingPermission:   {},
		RunStateStopped:             {},
	},
	RunStateVerify: {
		RunStateVerify:              {},
		RunStatePlan:                {},
		RunStateExecute:             {},
		RunStateCompacting:          {},
		RunStateWaitingUserQuestion: {},
		RunStateWaitingPermission:   {},
		RunStateStopped:             {},
	},
	RunStateCompacting: {
		RunStateCompacting:          {},
		RunStatePlan:                {},
		RunStateExecute:             {},
		RunStateVerify:              {},
		RunStateWaitingUserQuestion: {},
		RunStateWaitingPermission:   {},
		RunStateStopped:             {},
	},
	RunStateWaitingUserQuestion: {
		RunStateWaitingUserQuestion: {},
		RunStatePlan:                {},
		RunStateExecute:             {},
		RunStateVerify:              {},
		RunStateCompacting:          {},
		RunStateWaitingPermission:   {},
		RunStateStopped:             {},
	},
	RunStateWaitingPermission: {
		RunStateWaitingPermission:   {},
		RunStatePlan:                {},
		RunStateExecute:             {},
		RunStateVerify:              {},
		RunStateCompacting:          {},
		RunStateWaitingUserQuestion: {},
		RunStateStopped:             {},
	},
	RunStateStopped: {
		RunStateStopped: {},
	},
}

// ValidateRunStateTransition 校验生命周期迁移是否合法，避免主链 phase 与外围治理态分裂成多套规则。
func ValidateRunStateTransition(from RunState, to RunState) error {
	if nextStates, ok := allowedRunStateTransitions[from]; ok {
		if _, allowed := nextStates[to]; allowed {
			return nil
		}
	}
	return fmt.Errorf("runtime: invalid run state transition %q -> %q", from, to)
}
