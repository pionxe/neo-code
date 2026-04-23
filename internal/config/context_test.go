package config

import (
	"strings"
	"testing"
)

func TestContextConfigCloneIndependence(t *testing.T) {
	t.Parallel()

	original := ContextConfig{
		Compact: CompactConfig{
			ManualStrategy:           CompactManualStrategyKeepRecent,
			ManualKeepRecentMessages: 10,
			MaxSummaryChars:          1200,
			ReadTimeMaxMessageSpans:  24,
		},
		Budget: BudgetConfig{
			PromptBudget:         50000,
			ReserveTokens:        9000,
			FallbackPromptBudget: 88000,
			MaxReactiveCompacts:  2,
		},
	}
	cloned := original.Clone()

	cloned.Compact.ManualStrategy = CompactManualStrategyFullReplace
	cloned.Compact.ManualKeepRecentMessages = 5
	cloned.Budget.PromptBudget = 100000
	cloned.Budget.MaxReactiveCompacts = 4

	if original.Compact.ManualStrategy == cloned.Compact.ManualStrategy {
		t.Fatal("expected Compact clone to be independent")
	}
	if original.Compact.ManualKeepRecentMessages == cloned.Compact.ManualKeepRecentMessages {
		t.Fatal("expected ManualKeepRecentMessages clone to be independent")
	}
	if original.Budget.PromptBudget == cloned.Budget.PromptBudget {
		t.Fatal("expected Budget PromptBudget clone to be independent")
	}
	if original.Budget.MaxReactiveCompacts == cloned.Budget.MaxReactiveCompacts {
		t.Fatal("expected Budget MaxReactiveCompacts clone to be independent")
	}
}

func TestBudgetConfigCloneValueSemantics(t *testing.T) {
	t.Parallel()

	original := BudgetConfig{
		PromptBudget:         75000,
		ReserveTokens:        13000,
		FallbackPromptBudget: 100000,
		MaxReactiveCompacts:  3,
	}
	cloned := original.Clone()
	if original != cloned {
		t.Fatalf("expected equal configs, got %+v vs %+v", original, cloned)
	}
}

func TestContextConfigValidatePropagatesCompactError(t *testing.T) {
	t.Parallel()

	cfg := ContextConfig{
		Compact: CompactConfig{
			ManualKeepRecentMessages: 0,
			MaxSummaryChars:          0,
		},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "compact") {
		t.Fatalf("expected compact validation error, got %v", err)
	}
}

func TestContextConfigApplyDefaultsNilReceiver(t *testing.T) {
	t.Parallel()

	var ctxCfg *ContextConfig
	ctxCfg.ApplyDefaults(ContextConfig{
		Compact: CompactConfig{ManualStrategy: CompactManualStrategyFullReplace},
		Budget: BudgetConfig{
			ReserveTokens:        13000,
			FallbackPromptBudget: 100000,
			MaxReactiveCompacts:  3,
		},
	})
}

func TestCompactConfigApplyDefaultsAllZeroValues(t *testing.T) {
	t.Parallel()

	cfg := CompactConfig{}
	defaults := CompactConfig{
		ManualStrategy:           CompactManualStrategyFullReplace,
		ManualKeepRecentMessages: 15,
		MaxSummaryChars:          2000,
		ReadTimeMaxMessageSpans:  24,
	}
	cfg.ApplyDefaults(defaults)
	if cfg.ManualStrategy != CompactManualStrategyFullReplace {
		t.Fatalf("expected strategy %q, got %q", CompactManualStrategyFullReplace, cfg.ManualStrategy)
	}
	if cfg.ManualKeepRecentMessages != 15 {
		t.Fatalf("expected messages=15, got %d", cfg.ManualKeepRecentMessages)
	}
	if cfg.MaxSummaryChars != 2000 {
		t.Fatalf("expected chars=2000, got %d", cfg.MaxSummaryChars)
	}
	if cfg.ReadTimeMaxMessageSpans != 24 {
		t.Fatalf("expected read-time spans=24, got %d", cfg.ReadTimeMaxMessageSpans)
	}
}
