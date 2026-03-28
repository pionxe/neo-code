package provider_test

import (
	"strings"
	"testing"

	"github.com/dust/neo-code/internal/config"
	"github.com/dust/neo-code/internal/provider"
	"github.com/dust/neo-code/internal/provider/anthropic"
	"github.com/dust/neo-code/internal/provider/gemini"
	"github.com/dust/neo-code/internal/provider/openai"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	t.Parallel()

	openAIProvider, err := openai.New(config.ProviderConfig{
		Name:      config.ProviderOpenAI,
		Type:      config.ProviderOpenAI,
		BaseURL:   config.DefaultOpenAIBaseURL,
		Model:     config.DefaultOpenAIModel,
		APIKeyEnv: config.DefaultOpenAIAPIKeyEnv,
	})
	if err != nil {
		t.Fatalf("openai.New() error = %v", err)
	}
	anthropicProvider := anthropic.New(config.ProviderConfig{
		Name:      config.ProviderAnthropic,
		Type:      config.ProviderAnthropic,
		BaseURL:   config.DefaultAnthropicBaseURL,
		Model:     config.DefaultAnthropicModel,
		APIKeyEnv: config.DefaultAnthropicAPIKeyEnv,
	})
	geminiProvider := gemini.New(config.ProviderConfig{
		Name:      config.ProviderGemini,
		Type:      config.ProviderGemini,
		BaseURL:   config.DefaultGeminiBaseURL,
		Model:     config.DefaultGeminiModel,
		APIKeyEnv: config.DefaultGeminiAPIKeyEnv,
	})

	registry := provider.NewRegistry()
	registry.Register(nil)
	registry.Register(openAIProvider)
	registry.Register(anthropicProvider)
	registry.Register(geminiProvider)

	tests := []struct {
		name       string
		lookup     string
		expectName string
	}{
		{
			name:       "gets openai provider case insensitively",
			lookup:     "OPENAI",
			expectName: config.ProviderOpenAI,
		},
		{
			name:       "gets anthropic provider case insensitively",
			lookup:     "Anthropic",
			expectName: config.ProviderAnthropic,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := registry.Get(tt.lookup)
			if err != nil {
				t.Fatalf("Get(%q) error = %v", tt.lookup, err)
			}
			if got == nil || !strings.EqualFold(got.Name(), tt.expectName) {
				t.Fatalf("expected provider %q, got %+v", tt.expectName, got)
			}
		})
	}
}

func TestRegistryGetMissingProvider(t *testing.T) {
	t.Parallel()

	registry := provider.NewRegistry()
	_, err := registry.Get("missing")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestRegistryDescriptorsFiltersSupportSurface(t *testing.T) {
	t.Parallel()

	openAIProvider, err := openai.New(config.ProviderConfig{
		Name:      config.ProviderOpenAI,
		Type:      config.ProviderOpenAI,
		BaseURL:   config.DefaultOpenAIBaseURL,
		Model:     config.DefaultOpenAIModel,
		APIKeyEnv: config.DefaultOpenAIAPIKeyEnv,
	})
	if err != nil {
		t.Fatalf("openai.New() error = %v", err)
	}

	registry := provider.NewRegistry()
	registry.Register(openAIProvider)
	registry.Register(anthropic.New(config.ProviderConfig{
		Name:      config.ProviderAnthropic,
		Type:      config.ProviderAnthropic,
		BaseURL:   config.DefaultAnthropicBaseURL,
		Model:     config.DefaultAnthropicModel,
		APIKeyEnv: config.DefaultAnthropicAPIKeyEnv,
	}))
	registry.Register(gemini.New(config.ProviderConfig{
		Name:      config.ProviderGemini,
		Type:      config.ProviderGemini,
		BaseURL:   config.DefaultGeminiBaseURL,
		Model:     config.DefaultGeminiModel,
		APIKeyEnv: config.DefaultGeminiAPIKeyEnv,
	}))

	all := registry.Descriptors()
	if len(all) != 3 {
		t.Fatalf("expected 3 descriptors, got %d", len(all))
	}

	mvp := registry.MVPDescriptors()
	if len(mvp) != 1 {
		t.Fatalf("expected 1 MVP descriptor, got %d", len(mvp))
	}
	if mvp[0].Name != config.ProviderOpenAI {
		t.Fatalf("expected MVP provider %q, got %q", config.ProviderOpenAI, mvp[0].Name)
	}
	if mvp[0].SupportLevel != provider.SupportLevelMVP || !mvp[0].Available || !mvp[0].MVPVisible {
		t.Fatalf("unexpected MVP descriptor: %+v", mvp[0])
	}

	available := registry.AvailableDescriptors()
	if len(available) != 1 {
		t.Fatalf("expected 1 available descriptor, got %d", len(available))
	}
	if available[0].Name != config.ProviderOpenAI {
		t.Fatalf("expected available provider %q, got %q", config.ProviderOpenAI, available[0].Name)
	}
}
