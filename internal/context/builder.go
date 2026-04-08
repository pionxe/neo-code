package context

import (
	"context"

	providertypes "neo-code/internal/provider/types"
)

// DefaultBuilder preserves the current runtime context-building behavior.
type DefaultBuilder struct {
	promptSources        []promptSectionSource
	trimPolicy           messageTrimPolicy
	microCompactPolicies MicroCompactPolicySource
}

// NewBuilder returns the default context builder implementation.
func NewBuilder() Builder {
	return NewBuilderWithToolPolicies(nil)
}

// NewBuilderWithToolPolicies 返回带工具 micro compact 策略源的默认上下文构建器。
func NewBuilderWithToolPolicies(policies MicroCompactPolicySource) Builder {
	systemSource := &systemStateSource{gitRunner: runGitCommand}
	return &DefaultBuilder{
		promptSources: []promptSectionSource{
			corePromptSource{},
			&projectRulesSource{},
			systemSource,
		},
		trimPolicy:           spanMessageTrimPolicy{},
		microCompactPolicies: policies,
	}
}

// Build assembles the provider-facing context for the current round.
func (b *DefaultBuilder) Build(ctx context.Context, input BuildInput) (BuildResult, error) {
	if err := ctx.Err(); err != nil {
		return BuildResult{}, err
	}

	sections := make([]promptSection, 0, len(b.promptSources)+1)
	for _, source := range b.promptSources {
		sourceSections, err := source.Sections(ctx, input)
		if err != nil {
			return BuildResult{}, err
		}
		sections = append(sections, sourceSections...)
	}

	trimPolicy := b.trimPolicy
	if trimPolicy == nil {
		trimPolicy = spanMessageTrimPolicy{}
	}

	shouldAutoCompact := input.Compact.AutoCompactThreshold > 0 &&
		input.Metadata.SessionInputTokens >= input.Compact.AutoCompactThreshold

	return BuildResult{
		SystemPrompt:      composeSystemPrompt(sections...),
		Messages:          applyReadTimeContextProjection(trimPolicy.Trim(input.Messages), input.Compact, b.microCompactPolicies),
		ShouldAutoCompact: shouldAutoCompact,
	}, nil
}

// applyReadTimeContextProjection 负责在 provider 请求前按开关应用只读上下文投影，避免改写原始会话消息。
func applyReadTimeContextProjection(messages []providertypes.Message, options CompactOptions, policies MicroCompactPolicySource) []providertypes.Message {
	if options.DisableMicroCompact {
		return cloneContextMessages(messages)
	}
	return microCompactMessagesWithPolicies(messages, policies)
}
