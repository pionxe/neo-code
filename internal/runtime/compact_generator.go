package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	agentcontext "neo-code/internal/context"
	contextcompact "neo-code/internal/context/compact"
	"neo-code/internal/provider"
	"neo-code/internal/provider/streaming"
	providertypes "neo-code/internal/provider/types"
)

type compactSummaryGenerator struct {
	providerFactory ProviderFactory
	providerConfig  provider.RuntimeConfig
	model           string
}

func newCompactSummaryGenerator(
	providerFactory ProviderFactory,
	providerCfg provider.RuntimeConfig,
	model string,
) contextcompact.SummaryGenerator {
	return &compactSummaryGenerator{
		providerFactory: providerFactory,
		providerConfig:  providerCfg,
		model:           strings.TrimSpace(model),
	}
}

type compactSummaryResponse struct {
	TaskState struct {
		Goal            string   `json:"goal"`
		Progress        []string `json:"progress"`
		OpenItems       []string `json:"open_items"`
		NextStep        string   `json:"next_step"`
		Blockers        []string `json:"blockers"`
		KeyArtifacts    []string `json:"key_artifacts"`
		Decisions       []string `json:"decisions"`
		UserConstraints []string `json:"user_constraints"`
	} `json:"task_state"`
	DisplaySummary string `json:"display_summary"`
}

// Generate 使用冻结后的 provider 配置生成新的 durable task state 与展示摘要。
func (g *compactSummaryGenerator) Generate(
	ctx context.Context,
	input contextcompact.SummaryInput,
) (contextcompact.SummaryOutput, error) {
	if err := ctx.Err(); err != nil {
		return contextcompact.SummaryOutput{}, err
	}
	if g.providerFactory == nil {
		return contextcompact.SummaryOutput{}, errors.New("runtime: compact summary generator provider factory is nil")
	}
	if strings.TrimSpace(g.providerConfig.Driver) == "" ||
		strings.TrimSpace(g.providerConfig.BaseURL) == "" ||
		strings.TrimSpace(g.providerConfig.APIKey) == "" {
		return contextcompact.SummaryOutput{}, errors.New("runtime: compact summary generator provider config is incomplete")
	}

	prompt := agentcontext.BuildCompactPrompt(agentcontext.CompactPromptInput{
		Mode:                     string(input.Mode),
		ManualStrategy:           input.Config.ManualStrategy,
		ManualKeepRecentMessages: input.Config.ManualKeepRecentMessages,
		ArchivedMessageCount:     input.ArchivedMessageCount,
		MaxSummaryChars:          input.Config.MaxSummaryChars,
		CurrentTaskState:         input.CurrentTaskState,
		ArchivedMessages:         input.ArchivedMessages,
		RetainedMessages:         input.RetainedMessages,
	})

	modelProvider, err := g.providerFactory.Build(ctx, g.providerConfig)
	if err != nil {
		return contextcompact.SummaryOutput{}, err
	}

	outcome := generateStreamingMessage(ctx, modelProvider, providertypes.GenerateRequest{
		Model:        g.model,
		SystemPrompt: prompt.SystemPrompt,
		Messages: []providertypes.Message{{
			Role:    providertypes.RoleUser,
			Content: prompt.UserPrompt,
		}},
	}, streaming.Hooks{})
	if outcome.err != nil {
		return contextcompact.SummaryOutput{}, outcome.err
	}

	message := outcome.message
	if len(message.ToolCalls) > 0 {
		return contextcompact.SummaryOutput{}, errors.New("runtime: compact summary response must not contain tool calls")
	}

	return parseCompactSummaryOutput(message.Content)
}

// parseCompactSummaryOutput 解析 compact 生成器返回的 JSON 响应。
func parseCompactSummaryOutput(content string) (contextcompact.SummaryOutput, error) {
	jsonText, err := extractJSONObject(content)
	if err != nil {
		return contextcompact.SummaryOutput{}, err
	}

	var response compactSummaryResponse
	if err := json.Unmarshal([]byte(jsonText), &response); err != nil {
		return contextcompact.SummaryOutput{}, err
	}

	output := contextcompact.SummaryOutput{
		DisplaySummary: strings.TrimSpace(response.DisplaySummary),
	}
	output.TaskState.Goal = response.TaskState.Goal
	output.TaskState.Progress = append([]string(nil), response.TaskState.Progress...)
	output.TaskState.OpenItems = append([]string(nil), response.TaskState.OpenItems...)
	output.TaskState.NextStep = response.TaskState.NextStep
	output.TaskState.Blockers = append([]string(nil), response.TaskState.Blockers...)
	output.TaskState.KeyArtifacts = append([]string(nil), response.TaskState.KeyArtifacts...)
	output.TaskState.Decisions = append([]string(nil), response.TaskState.Decisions...)
	output.TaskState.UserConstraints = append([]string(nil), response.TaskState.UserConstraints...)

	if output.DisplaySummary == "" {
		return contextcompact.SummaryOutput{}, errors.New("runtime: compact summary response is empty")
	}
	return output, nil
}

// extractJSONObject 从模型响应中提取最外层 JSON 对象，容忍前后噪音。
func extractJSONObject(text string) (string, error) {
	start := strings.Index(text, "{")
	if start < 0 {
		return "", errors.New("runtime: compact summary response does not contain a JSON object")
	}

	depth := 0
	inString := false
	escaped := false
	for index := start; index < len(text); index++ {
		ch := text[index]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return strings.TrimSpace(text[start : index+1]), nil
			}
		}
	}

	return "", errors.New("runtime: compact summary response contains an incomplete JSON object")
}
