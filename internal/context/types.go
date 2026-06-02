package context

import (
	"context"

	providertypes "neo-code/internal/provider/types"
	"neo-code/internal/repository"
	agentsession "neo-code/internal/session"
	"neo-code/internal/skills"
)

// Builder builds the provider-facing context for a single model round.
type Builder interface {
	Build(ctx context.Context, input BuildInput) (BuildResult, error)
}

// BuildInput contains the runtime state needed to assemble model context.
type BuildInput struct {
	Messages          []providertypes.Message
	TaskState         agentsession.TaskState
	Todos             []agentsession.TodoItem
	AgentMode         agentsession.AgentMode
	PlanStage         string
	CurrentPlan       *agentsession.PlanArtifact
	InjectFullPlan    bool
	ActiveSkills      []skills.Skill
	RepositorySummary *RepositorySummarySection
	Repository        RepositoryContext
	Metadata          Metadata
	Compact           CompactOptions
}

// BuildResult is the provider-facing context produced for a single round.
type BuildResult struct {
	// SystemPrompt 是最终拼接结果，兼容旧链路：StableSystemPrompt + "\n\n" + DynamicSystemPrompt。
	SystemPrompt string
	// StableSystemPrompt 是长期稳定、适合作为缓存前缀的系统提示词。
	StableSystemPrompt string
	// DynamicSystemPrompt 是当前轮运行态提示词，随任务进度变化。
	DynamicSystemPrompt string
	Messages            []providertypes.Message
}

// RepositorySummarySection 承载 runtime 已决策好的最小 repository summary 投影。
type RepositorySummarySection struct {
	InGitRepo bool
	Branch    string
	Dirty     bool
	Ahead     int
	Behind    int
}

// RepositoryContext 承载 runtime 已决策好的 repository 事实投影，供 context 只读渲染。
type RepositoryContext struct {
	ChangedFiles *RepositoryChangedFilesSection
	Retrieval    *RepositoryRetrievalSection
}

// RepositoryChangedFilesSection 描述当前轮允许注入的变更文件摘要。
type RepositoryChangedFilesSection struct {
	Files         []repository.ChangedFile
	Truncated     bool
	ReturnedCount int
	TotalCount    int
}

// RepositoryRetrievalSection 描述当前轮允许注入的定向检索结果。
type RepositoryRetrievalSection struct {
	Hits      []repository.RetrievalHit
	Truncated bool
	Mode      string
	Query     string
}

// CompactOptions controls read-time context behavior inside the context builder.
type CompactOptions struct {
	ReadTimeMaxMessageSpans int
}
