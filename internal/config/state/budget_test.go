package state

import (
	"context"
	"errors"
	"testing"

	configpkg "neo-code/internal/config"
	providertypes "neo-code/internal/provider/types"
)

func assertPromptBudgetResolution(
	t *testing.T,
	got PromptBudgetResolution,
	wantBudget int,
	wantSource PromptBudgetSource,
) {
	t.Helper()

	if got.PromptBudget != wantBudget || got.Source != wantSource {
		t.Fatalf("expected budget=%d source=%s, got %+v", wantBudget, wantSource, got)
	}
}

func TestResolvePromptBudgetExplicitWins(t *testing.T) {
	t.Parallel()

	cfg := configpkg.StaticDefaults().Clone()
	cfg.Context.Budget.PromptBudget = 42000

	resolution, err := ResolvePromptBudget(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("ResolvePromptBudget() error = %v", err)
	}
	assertPromptBudgetResolution(t, resolution, 42000, PromptBudgetSourceExplicit)
}

func TestResolvePromptBudgetDerivedFromContextWindow(t *testing.T) {
	t.Parallel()

	cfg := testDefaultConfig().Clone()
	cfg.Context.Budget.PromptBudget = 0
	cfg.Context.Budget.ReserveTokens = 13000
	cfg.CurrentModel = "deepseek-coder"
	cfg.Providers[0].Model = "deepseek-coder"
	cfg.Providers[0].Models = []providertypes.ModelDescriptor{{
		ID:            "deepseek-coder",
		ContextWindow: 131072,
	}}

	resolution, err := ResolvePromptBudget(context.Background(), cfg, catalogMethodsStub{
		snapshotModels: cfg.Providers[0].Models,
	})
	if err != nil {
		t.Fatalf("ResolvePromptBudget() error = %v", err)
	}
	assertPromptBudgetResolution(t, resolution, 118072, PromptBudgetSourceDerived)
}

func TestResolvePromptBudgetFallsBackWhenWindowTooSmall(t *testing.T) {
	t.Parallel()

	cfg := testDefaultConfig().Clone()
	cfg.Context.Budget.PromptBudget = 0
	cfg.Context.Budget.ReserveTokens = 13000
	cfg.Context.Budget.FallbackPromptBudget = 88000
	cfg.CurrentModel = "small-model"
	cfg.Providers[0].Model = "small-model"

	resolution, err := ResolvePromptBudget(context.Background(), cfg, catalogMethodsStub{
		snapshotModels: []providertypes.ModelDescriptor{{
			ID:            "small-model",
			ContextWindow: 8000,
		}},
	})
	if err != nil {
		t.Fatalf("ResolvePromptBudget() error = %v", err)
	}
	assertPromptBudgetResolution(t, resolution, 88000, PromptBudgetSourceFallback)
}

func TestResolvePromptBudgetFallsBackWhenModelMissing(t *testing.T) {
	t.Parallel()

	cfg := testDefaultConfig().Clone()
	cfg.Context.Budget.PromptBudget = 0
	cfg.Context.Budget.FallbackPromptBudget = 88000
	cfg.CurrentModel = "missing-model"

	resolution, err := ResolvePromptBudget(context.Background(), cfg, catalogMethodsStub{
		snapshotModels: []providertypes.ModelDescriptor{{ID: "other-model", ContextWindow: 131072}},
	})
	if err != nil {
		t.Fatalf("ResolvePromptBudget() error = %v", err)
	}
	assertPromptBudgetResolution(t, resolution, 88000, PromptBudgetSourceFallback)
}

func TestResolvePromptBudgetFallsBackWhenSelectedProviderInvalid(t *testing.T) {
	t.Parallel()

	cfg := testDefaultConfig().Clone()
	cfg.Context.Budget.PromptBudget = 0
	cfg.Context.Budget.FallbackPromptBudget = 88000
	cfg.SelectedProvider = "missing-provider"

	resolution, err := ResolvePromptBudget(context.Background(), cfg, catalogMethodsStub{})
	if err != nil {
		t.Fatalf("ResolvePromptBudget() error = %v", err)
	}
	assertPromptBudgetResolution(t, resolution, 88000, PromptBudgetSourceFallback)
}

func TestResolvePromptBudgetFallsBackWhenCatalogInputResolutionFails(t *testing.T) {
	t.Parallel()

	cfg := testDefaultConfig().Clone()
	cfg.Context.Budget.PromptBudget = 0
	cfg.Context.Budget.FallbackPromptBudget = 88000
	cfg.Providers[0].BaseURL = ""

	resolution, err := ResolvePromptBudget(context.Background(), cfg, catalogMethodsStub{})
	if err != nil {
		t.Fatalf("ResolvePromptBudget() error = %v", err)
	}
	assertPromptBudgetResolution(t, resolution, 88000, PromptBudgetSourceFallback)
}

func TestResolvePromptBudgetFallsBackWhenSnapshotLookupFails(t *testing.T) {
	t.Parallel()

	cfg := testDefaultConfig().Clone()
	cfg.Context.Budget.PromptBudget = 0
	cfg.Context.Budget.FallbackPromptBudget = 88000

	resolution, err := ResolvePromptBudget(context.Background(), cfg, catalogMethodsStub{
		snapshotErr: errors.New("snapshot failed"),
	})
	if err == nil {
		t.Fatalf("ResolvePromptBudget() error = nil, want non-nil")
	}
	assertPromptBudgetResolution(t, resolution, 88000, PromptBudgetSourceFallback)
}
