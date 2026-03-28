package anthropic

import (
	"context"
	"strings"
	"testing"

	"github.com/dust/neo-code/internal/config"
	"github.com/dust/neo-code/internal/provider"
)

func TestProviderScaffold(t *testing.T) {
	t.Parallel()

	cfg := config.ProviderConfig{Name: config.ProviderAnthropic, Type: config.ProviderAnthropic}
	p := New(cfg)
	if p.Name() != config.ProviderAnthropic {
		t.Fatalf("expected provider name %q, got %q", config.ProviderAnthropic, p.Name())
	}

	desc := p.Descriptor()
	if desc.SupportLevel != provider.SupportLevelScaffolded || desc.Available || desc.MVPVisible {
		t.Fatalf("unexpected descriptor: %+v", desc)
	}

	_, err := p.Chat(context.Background(), provider.ChatRequest{}, make(chan provider.StreamEvent))
	if err == nil || !strings.Contains(err.Error(), "only OpenAI-compatible provider is officially supported") {
		t.Fatalf("expected scaffold error, got %v", err)
	}
}
