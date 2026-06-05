package context

import (
	"context"
)

// DefaultBuilder preserves the current runtime context-building behavior.
type DefaultBuilder struct {
	stablePromptSources  []promptSectionSource
	dynamicPromptSources []promptSectionSource
	trimPolicy           messageTrimPolicy
}

// newStablePromptSources 返回稳定提示词来源列表，适合作为缓存前缀。
// extra 会追加到 stable 中（如 memo 持久记忆索引）。
func newStablePromptSources(extra ...SectionSource) []promptSectionSource {
	sources := []promptSectionSource{
		corePromptSource{},
		newRulesPromptSource(nil),
	}
	for _, src := range extra {
		sources = append(sources, src)
	}
	return sources
}

// newDynamicPromptSources 返回动态提示词来源列表，随任务进度、会话状态变化。
func newDynamicPromptSources() []promptSectionSource {
	return []promptSectionSource{
		capabilitiesSource{},
		taskStateSource{},
		planModeContextSource{},
		todosSource{},
		skillPromptSource{},
		repositoryContextSource{},
		&systemStateSource{},
	}
}

// NewConfiguredBuilder 基于可选 SectionSource 列表构建上下文构建器，是推荐的统一构造入口。
func NewConfiguredBuilder(sources ...SectionSource) Builder {
	return &DefaultBuilder{
		stablePromptSources:  newStablePromptSources(sources...),
		dynamicPromptSources: newDynamicPromptSources(),
		trimPolicy:           spanMessageTrimPolicy{},
	}
}

// NewBuilder returns the default context builder implementation.
func NewBuilder() Builder {
	return NewConfiguredBuilder()
}

// collectPromptSections 遍历 promptSectionSource 列表并收集所有 sections。
func collectPromptSections(ctx context.Context, input BuildInput, sources []promptSectionSource) ([]promptSection, error) {
	sections := make([]promptSection, 0, len(sources))
	for _, source := range sources {
		sourceSections, err := source.Sections(ctx, input)
		if err != nil {
			return nil, err
		}
		sections = append(sections, sourceSections...)
	}
	return sections, nil
}

// Build assembles the provider-facing context for the current round.
func (b *DefaultBuilder) Build(ctx context.Context, input BuildInput) (BuildResult, error) {
	if err := ctx.Err(); err != nil {
		return BuildResult{}, err
	}

	stableSections, err := collectPromptSections(ctx, input, b.stablePromptSources)
	if err != nil {
		return BuildResult{}, err
	}
	stablePrompt := composeSystemPrompt(stableSections...)

	dynamicSections, err := collectPromptSections(ctx, input, b.dynamicPromptSources)
	if err != nil {
		return BuildResult{}, err
	}
	dynamicPrompt := composeSystemPrompt(dynamicSections...)

	systemPrompt := joinSystemPromptParts(stablePrompt, dynamicPrompt)

	trimPolicy := b.trimPolicy
	if trimPolicy == nil {
		trimPolicy = spanMessageTrimPolicy{}
	}

	return BuildResult{
		SystemPrompt:        systemPrompt,
		StableSystemPrompt:  stablePrompt,
		DynamicSystemPrompt: dynamicPrompt,
		Messages: projectReadTimeMessagesForModel(
			trimPolicy.Trim(input.Messages, input.Compact),
		),
	}, nil
}
