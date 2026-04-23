package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"neo-code/internal/provider"
)

func TestDriverDiscover(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-goog-api-key"); got != "test-key" {
			t.Fatalf("expected x-goog-api-key header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{"name": "gemini-2.5-flash"},
			},
		})
	}))
	defer server.Close()

	driver := Driver()
	models, err := driver.Discover(context.Background(), provider.RuntimeConfig{
		Driver:                DriverName,
		BaseURL:               server.URL,
		APIKeyEnv:             "TEST_GEMINI_KEY",
		APIKeyResolver:        provider.StaticAPIKeyResolver("test-key"),
		DiscoveryEndpointPath: "/models",
	})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected one model, got %+v", models)
	}
}

func TestDriverBuild(t *testing.T) {
	t.Parallel()

	driver := Driver()
	p, err := driver.Build(context.Background(), provider.RuntimeConfig{
		Driver:         DriverName,
		BaseURL:        "https://generativelanguage.googleapis.com/v1beta",
		APIKeyEnv:      "TEST_GEMINI_KEY",
		APIKeyResolver: provider.StaticAPIKeyResolver("test-key"),
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestDriverValidateCatalogIdentity(t *testing.T) {
	t.Parallel()

	driver := Driver()

	t.Run("accepts default identity", func(t *testing.T) {
		t.Parallel()

		err := driver.ValidateCatalogIdentity(provider.ProviderIdentity{
			Driver:                DriverName,
			DiscoveryEndpointPath: "/models",
		})
		if err != nil {
			t.Fatalf("expected valid identity, got %v", err)
		}
	})

	t.Run("accepts custom endpoints in sdk mode", func(t *testing.T) {
		t.Parallel()

		err := driver.ValidateCatalogIdentity(provider.ProviderIdentity{
			Driver:                DriverName,
			ChatEndpointPath:      "/gateway/models",
			DiscoveryEndpointPath: "/custom/models",
		})
		if err != nil {
			t.Fatalf("expected custom endpoints to be accepted, got %v", err)
		}
	})

	t.Run("accepts non-relative endpoints in catalog identity", func(t *testing.T) {
		t.Parallel()

		err := driver.ValidateCatalogIdentity(provider.ProviderIdentity{
			Driver:                DriverName,
			DiscoveryEndpointPath: "https://api.example.com/models",
			ChatEndpointPath:      "https://api.example.com/models",
		})
		if err != nil {
			t.Fatalf("expected non-relative endpoints to be accepted, got %v", err)
		}
	})
}
