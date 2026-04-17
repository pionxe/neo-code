package subagent

import (
	"time"

	agentsession "neo-code/internal/session"
)

// TodoStore 定义调度器读写 Todo 的最小依赖。
type TodoStore interface {
	ListTodos() []agentsession.TodoItem
	FindTodo(id string) (agentsession.TodoItem, bool)
	UpdateTodo(id string, patch agentsession.TodoPatch, expectedRevision int64) error
	ClaimTodo(id string, ownerType string, ownerID string, expectedRevision int64) error
	CompleteTodo(id string, artifacts []string, expectedRevision int64) error
	FailTodo(id string, reason string, expectedRevision int64) error
}

// RoleSelector 根据 Todo 节点选择执行角色。
type RoleSelector func(todo agentsession.TodoItem) Role

// BudgetSelector 根据 Todo 节点生成预算。
type BudgetSelector func(todo agentsession.TodoItem, defaults Budget) Budget

// CapabilitySelector 根据 Todo 节点生成能力边界。
type CapabilitySelector func(todo agentsession.TodoItem) Capability

// ContextSliceBuilder 根据任务快照构建可消费的上下文切片。
type ContextSliceBuilder func(input TaskContextSliceInput) TaskContextSlice

// ContextSkillSelector 根据任务快照选择激活技能列表。
type ContextSkillSelector func(todo agentsession.TodoItem, snapshot map[string]agentsession.TodoItem) []string

// ContextFileSelector 根据任务快照选择相关文件摘要。
type ContextFileSelector func(todo agentsession.TodoItem, snapshot map[string]agentsession.TodoItem) []TaskContextFileSummary

// RetryBackoff 计算第 N 次重试的退避时长。
type RetryBackoff func(attempt int) time.Duration

// SchedulerEventType 描述调度器可观测事件类型。
type SchedulerEventType string

const (
	// SchedulerEventQueued 表示任务进入可调度队列。
	SchedulerEventQueued SchedulerEventType = "queued"
	// SchedulerEventClaimed 表示任务已经被领取。
	SchedulerEventClaimed SchedulerEventType = "claimed"
	// SchedulerEventRunning 表示任务开始执行。
	SchedulerEventRunning SchedulerEventType = "running"
	// SchedulerEventCompleted 表示任务成功完成。
	SchedulerEventCompleted SchedulerEventType = "completed"
	// SchedulerEventRetryScheduled 表示任务失败后进入重试窗口。
	SchedulerEventRetryScheduled SchedulerEventType = "retry_scheduled"
	// SchedulerEventFailed 表示任务失败且不再重试。
	SchedulerEventFailed SchedulerEventType = "failed"
	// SchedulerEventCanceled 表示任务被取消。
	SchedulerEventCanceled SchedulerEventType = "canceled"
	// SchedulerEventBlocked 表示任务因依赖或退避暂不可执行。
	SchedulerEventBlocked SchedulerEventType = "blocked"
	// SchedulerEventFinished 表示调度轮次结束。
	SchedulerEventFinished SchedulerEventType = "finished"
)

// SchedulerEvent 描述单次调度决策事件。
type SchedulerEvent struct {
	Type      SchedulerEventType
	TaskID    string
	Attempt   int
	Running   int
	QueueSize int
	Reason    string
	At        time.Time
}

// SchedulerObserver 消费调度事件。
type SchedulerObserver func(event SchedulerEvent)

// ScheduleResult 汇总一次调度执行结果。
type ScheduleResult struct {
	StartedAt   time.Time
	EndedAt     time.Time
	Total       int
	Succeeded   []string
	Failed      []string
	Retried     map[string]int
	BlockedLeft []string
}

// SchedulerConfig 描述调度策略参数。
type SchedulerConfig struct {
	MaxConcurrency int
	DefaultRole    Role
	DefaultBudget  Budget
	MaxRetries     int
	WorkerIDPrefix string
	PollInterval   time.Duration
	Clock          func() time.Time
	RoleSelector   RoleSelector
	BudgetSelector BudgetSelector
	Backoff        RetryBackoff
	Capabilities   CapabilitySelector

	ContextBuilder ContextSliceBuilder
	ContextSkills  ContextSkillSelector
	ContextFiles   ContextFileSelector

	ContextMaxChars               int
	ContextMaxTodoFragments       int
	ContextMaxDependencyArtifacts int
	ContextMaxRelatedFiles        int

	Observer SchedulerObserver
}

// normalize 返回带默认值的配置副本，避免执行阶段出现隐式零值。
func (c SchedulerConfig) normalize() SchedulerConfig {
	out := c
	if out.MaxConcurrency <= 0 {
		out.MaxConcurrency = 2
	}
	if !out.DefaultRole.Valid() {
		out.DefaultRole = RoleCoder
	}
	out.DefaultBudget = out.DefaultBudget.normalize(Budget{MaxSteps: 6, Timeout: 30 * time.Second})
	if out.MaxRetries < 0 {
		out.MaxRetries = 0
	}
	if out.PollInterval <= 0 {
		out.PollInterval = 200 * time.Millisecond
	}
	if out.WorkerIDPrefix == "" {
		out.WorkerIDPrefix = "subagent"
	}
	if out.Clock == nil {
		out.Clock = time.Now
	}
	if out.RoleSelector == nil {
		out.RoleSelector = defaultRoleSelector(out.DefaultRole)
	}
	if out.BudgetSelector == nil {
		out.BudgetSelector = defaultBudgetSelector
	}
	if out.Backoff == nil {
		out.Backoff = defaultRetryBackoff
	}
	if out.Capabilities == nil {
		out.Capabilities = func(todo agentsession.TodoItem) Capability {
			_ = todo
			return Capability{}
		}
	}
	if out.ContextBuilder == nil {
		out.ContextBuilder = BuildTaskContextSlice
	}
	if out.ContextSkills == nil {
		out.ContextSkills = func(todo agentsession.TodoItem, snapshot map[string]agentsession.TodoItem) []string {
			_ = todo
			_ = snapshot
			return nil
		}
	}
	if out.ContextFiles == nil {
		out.ContextFiles = func(todo agentsession.TodoItem, snapshot map[string]agentsession.TodoItem) []TaskContextFileSummary {
			_ = todo
			_ = snapshot
			return nil
		}
	}
	if out.ContextMaxChars <= 0 {
		out.ContextMaxChars = defaultTaskContextMaxChars
	}
	if out.ContextMaxTodoFragments <= 0 {
		out.ContextMaxTodoFragments = defaultTaskContextMaxTodoFragments
	}
	if out.ContextMaxDependencyArtifacts <= 0 {
		out.ContextMaxDependencyArtifacts = defaultTaskContextMaxDependencyArtifacts
	}
	if out.ContextMaxRelatedFiles <= 0 {
		out.ContextMaxRelatedFiles = defaultTaskContextMaxRelatedFiles
	}
	if out.Observer == nil {
		out.Observer = func(SchedulerEvent) {}
	}
	return out
}

// defaultRoleSelector 返回默认角色选择策略。
func defaultRoleSelector(defaultRole Role) RoleSelector {
	return func(todo agentsession.TodoItem) Role {
		_ = todo
		return defaultRole
	}
}

// defaultBudgetSelector 返回默认预算策略。
func defaultBudgetSelector(todo agentsession.TodoItem, defaults Budget) Budget {
	_ = todo
	return defaults
}

// defaultRetryBackoff 提供线性重试退避。
func defaultRetryBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	return time.Duration(attempt) * time.Second
}
