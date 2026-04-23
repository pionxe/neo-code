package state

import (
	"context"
	"strings"

	"neo-code/internal/config"
	"neo-code/internal/provider"
)

// PromptBudgetSource 标识 prompt budget 最终采用的来源。
type PromptBudgetSource string

const (
	PromptBudgetSourceExplicit PromptBudgetSource = "explicit"
	PromptBudgetSourceDerived  PromptBudgetSource = "derived"
	PromptBudgetSourceFallback PromptBudgetSource = "fallback"
)

// PromptBudgetResolution 描述 prompt budget 的解析结果，供 runtime 直接消费。
type PromptBudgetResolution struct {
	PromptBudget  int
	Source        PromptBudgetSource
	ContextWindow int
	ModelID       string
}

// fallbackPromptBudgetResolution 构造自动推导失败时使用的保底预算结果。
func fallbackPromptBudgetResolution(cfg config.Config) PromptBudgetResolution {
	return PromptBudgetResolution{
		PromptBudget: cfg.Context.Budget.FallbackPromptBudget,
		Source:       PromptBudgetSourceFallback,
		ModelID:      strings.TrimSpace(cfg.CurrentModel),
	}
}

// ResolvePromptBudget 基于当前选择的 provider/model 和模型目录快照解析最终输入预算。
func ResolvePromptBudget(
	ctx context.Context,
	cfg config.Config,
	catalogs ModelCatalog,
) (PromptBudgetResolution, error) {
	budget := cfg.Context.Budget
	if budget.PromptBudget > 0 {
		return PromptBudgetResolution{
			PromptBudget: budget.PromptBudget,
			Source:       PromptBudgetSourceExplicit,
			ModelID:      strings.TrimSpace(cfg.CurrentModel),
		}, nil
	}

	resolution := fallbackPromptBudgetResolution(cfg)
	providerCfg, err := selectedProviderConfig(cfg)
	if err != nil {
		return resolution, nil
	}
	if catalogs == nil {
		return resolution, nil
	}

	input, err := catalogInputFromProvider(providerCfg)
	if err != nil {
		return resolution, nil
	}

	models, err := catalogs.ListProviderModelsSnapshot(ctx, input)
	if err != nil {
		return resolution, err
	}

	modelID := provider.NormalizeKey(cfg.CurrentModel)
	for _, model := range models {
		if provider.NormalizeKey(model.ID) != modelID {
			continue
		}
		resolution.ContextWindow = model.ContextWindow
		if model.ContextWindow > budget.ReserveTokens {
			resolution.PromptBudget = model.ContextWindow - budget.ReserveTokens
			resolution.Source = PromptBudgetSourceDerived
		}
		return resolution, nil
	}

	return resolution, nil
}
