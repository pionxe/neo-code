package provider

import "testing"

func TestProviderIdentityKeyIncludesDriverSpecificFields(t *testing.T) {
	t.Parallel()

	identity := ProviderIdentity{
		Driver:         "openaicompat",
		BaseURL:        "https://api.example.com/v1",
		APIStyle:       "responses",
		DeploymentMode: "ignored",
		APIVersion:     "ignored",
	}

	if got, want := identity.Key(), "openaicompat|https://api.example.com/v1|responses|ignored|ignored"; got != want {
		t.Fatalf("expected identity key %q, got %q", want, got)
	}
}

func TestNormalizeProviderIdentityUsesDriverSpecificNormalization(t *testing.T) {
	t.Parallel()

	identity, err := NormalizeProviderIdentity(ProviderIdentity{
		Driver:   " OpenAICompat ",
		BaseURL:  "https://API.EXAMPLE.COM/v1/",
		APIStyle: " Responses ",
	})
	if err != nil {
		t.Fatalf("NormalizeProviderIdentity() error = %v", err)
	}

	if identity.Driver != "openaicompat" {
		t.Fatalf("expected normalized driver %q, got %q", "openaicompat", identity.Driver)
	}
	if identity.BaseURL != "https://api.example.com/v1" {
		t.Fatalf("expected normalized base url %q, got %q", "https://api.example.com/v1", identity.BaseURL)
	}
	if identity.APIStyle != "responses" {
		t.Fatalf("expected normalized api_style %q, got %q", "responses", identity.APIStyle)
	}
}

func TestNormalizeProviderIdentityPreservesDriverSpecificFields(t *testing.T) {
	t.Parallel()

	identity, err := NormalizeProviderIdentity(ProviderIdentity{
		Driver:         " Gemini ",
		BaseURL:        "https://API.EXAMPLE.COM/v1/",
		DeploymentMode: " Vertex ",
	})
	if err != nil {
		t.Fatalf("NormalizeProviderIdentity() error = %v", err)
	}

	if identity.Driver != "gemini" {
		t.Fatalf("expected normalized driver %q, got %q", "gemini", identity.Driver)
	}
	if identity.BaseURL != "https://api.example.com/v1" {
		t.Fatalf("expected normalized base url %q, got %q", "https://api.example.com/v1", identity.BaseURL)
	}
	if identity.DeploymentMode != "vertex" {
		t.Fatalf("expected normalized deployment_mode %q, got %q", "vertex", identity.DeploymentMode)
	}
}

func TestProviderIdentityStringMatchesKey(t *testing.T) {
	t.Parallel()

	identity := ProviderIdentity{
		Driver:   "openaicompat",
		BaseURL:  "https://api.example.com/v1",
		APIStyle: "responses",
	}
	if identity.String() != identity.Key() {
		t.Fatalf("expected String() to match Key(), got %q vs %q", identity.String(), identity.Key())
	}
}

func TestNewProviderIdentityValidatesInputs(t *testing.T) {
	t.Parallel()

	identity, err := NewProviderIdentity(" OpenAICompat ", "https://API.EXAMPLE.COM/v1/")
	if err != nil {
		t.Fatalf("NewProviderIdentity() error = %v", err)
	}
	if identity.Driver != "openaicompat" || identity.BaseURL != "https://api.example.com/v1" {
		t.Fatalf("unexpected identity: %+v", identity)
	}

	if _, err := NewProviderIdentity("   ", "https://api.example.com/v1"); err == nil {
		t.Fatalf("expected empty driver to fail")
	}
	if _, err := NewProviderIdentity("openaicompat", "not-a-url"); err == nil {
		t.Fatalf("expected invalid base URL to fail")
	}
}

func TestNormalizeProviderIdentityAnthropicAndUnknownDriver(t *testing.T) {
	t.Parallel()

	anthropicIdentity, err := NormalizeProviderIdentity(ProviderIdentity{
		Driver:     " Anthropic ",
		BaseURL:    "https://API.EXAMPLE.COM/v1/",
		APIVersion: " 2023-06-01 ",
	})
	if err != nil {
		t.Fatalf("NormalizeProviderIdentity() anthropic error = %v", err)
	}
	if anthropicIdentity.Driver != "anthropic" {
		t.Fatalf("expected anthropic driver, got %+v", anthropicIdentity)
	}
	if anthropicIdentity.APIVersion != "2023-06-01" {
		t.Fatalf("expected normalized api version, got %+v", anthropicIdentity)
	}

	fallbackIdentity, err := NormalizeProviderIdentity(ProviderIdentity{
		Driver:         " custom ",
		BaseURL:        "https://API.EXAMPLE.COM/v1/",
		APIStyle:       "responses",
		DeploymentMode: "vertex",
		APIVersion:     "2023-06-01",
	})
	if err != nil {
		t.Fatalf("NormalizeProviderIdentity() fallback error = %v", err)
	}
	if fallbackIdentity.Driver != "custom" || fallbackIdentity.BaseURL != "https://api.example.com/v1" {
		t.Fatalf("expected fallback identity to normalize driver and base URL, got %+v", fallbackIdentity)
	}
	if fallbackIdentity.APIStyle != "" || fallbackIdentity.DeploymentMode != "" || fallbackIdentity.APIVersion != "" {
		t.Fatalf("expected fallback identity to drop protocol-specific fields, got %+v", fallbackIdentity)
	}
}
