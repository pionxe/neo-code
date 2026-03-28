package anthropic

import (
	"context"

	"github.com/dust/neo-code/internal/config"
	"github.com/dust/neo-code/internal/provider"
)

type Provider struct {
	cfg config.ProviderConfig
}

func New(cfg config.ProviderConfig) *Provider {
	return &Provider{cfg: cfg}
}

func (p *Provider) Name() string {
	return p.cfg.Name
}

func (p *Provider) Descriptor() provider.ProviderDescriptor {
	return provider.ProviderDescriptor{
		Name:         p.cfg.Name,
		DisplayName:  "Anthropic",
		SupportLevel: provider.SupportLevelScaffolded,
		MVPVisible:   false,
		Available:    false,
		Summary:      "Scaffolded only. Not officially available in the current MVP.",
	}
}

func (p *Provider) Chat(ctx context.Context, req provider.ChatRequest, events chan<- provider.StreamEvent) (provider.ChatResponse, error) {
	return provider.ChatResponse{}, provider.ScaffoldedProviderError(p.Name())
}
