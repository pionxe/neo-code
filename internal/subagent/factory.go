package subagent

import "time"

// EngineBuilder 定义基于角色策略构建执行引擎的工厂函数。
type EngineBuilder func(role Role, policy RolePolicy) Engine

// ExecutionContext 描述子代理执行时可选注入的运行时上下文。
type ExecutionContext struct {
	ToolExecutor ToolExecutor
}

// FactoryOption 定义 WorkerFactory 的可选注入项。
type FactoryOption func(*WorkerFactory)

// WorkerFactory 是默认的 WorkerRuntime 工厂实现。
type WorkerFactory struct {
	builder EngineBuilder
	execCtx ExecutionContext
}

// NewWorkerFactory 创建 WorkerFactory；当 builder 为空时使用默认引擎。
func NewWorkerFactory(builder EngineBuilder, opts ...FactoryOption) *WorkerFactory {
	factory := &WorkerFactory{builder: builder}
	for _, opt := range opts {
		if opt != nil {
			opt(factory)
		}
	}
	return factory
}

// WithExecutionContext 注入默认执行上下文。
func WithExecutionContext(execCtx ExecutionContext) FactoryOption {
	return func(factory *WorkerFactory) {
		if factory == nil {
			return
		}
		factory.execCtx = execCtx
	}
}

// WithToolExecutor 注入子代理工具执行桥接。
func WithToolExecutor(executor ToolExecutor) FactoryOption {
	return func(factory *WorkerFactory) {
		if factory == nil {
			return
		}
		factory.execCtx.ToolExecutor = executor
	}
}

// Create 基于角色创建对应策略与执行引擎的 WorkerRuntime。
func (f *WorkerFactory) Create(role Role) (WorkerRuntime, error) {
	policy, err := DefaultRolePolicy(role)
	if err != nil {
		return nil, err
	}
	// 兜底保证默认预算始终可用。
	policy.DefaultBudget = policy.DefaultBudget.normalize(Budget{
		MaxSteps: defaultPolicyMaxSteps,
		Timeout:  defaultPolicyTimeout * time.Second,
	})

	var engine Engine
	if f != nil && f.builder != nil {
		engine = f.builder(role, policy)
	}
	var execCtx ExecutionContext
	if f != nil {
		execCtx = f.execCtx
	}
	return NewWorker(role, policy, engine, withExecutionContext(execCtx))
}
