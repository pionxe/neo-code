package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestDefaultWebFetchSupportedContentTypesReturnsCopy(t *testing.T) {
	t.Parallel()

	types1 := DefaultWebFetchSupportedContentTypes()
	types2 := DefaultWebFetchSupportedContentTypes()
	if &types1[0] == &types2[0] {
		t.Fatal("expected independent slices")
	}
	if len(types1) == 0 {
		t.Fatal("expected non-empty default content types")
	}
}

func TestWebFetchConfigCloneIndependence(t *testing.T) {
	t.Parallel()

	original := WebFetchConfig{
		MaxResponseBytes:      4096,
		SupportedContentTypes: []string{"text/html", "application/json"},
	}
	cloned := original.Clone()

	cloned.MaxResponseBytes = 1024
	cloned.SupportedContentTypes[0] = "text/plain"

	if original.MaxResponseBytes == cloned.MaxResponseBytes {
		t.Fatal("expected MaxResponseBytes clone to be independent")
	}
	if original.SupportedContentTypes[0] == cloned.SupportedContentTypes[0] {
		t.Fatal("expected SupportedContentTypes clone to be independent")
	}
}

func TestWebFetchConfigValidateRejectsEmptyContentType(t *testing.T) {
	t.Parallel()

	cfg := WebFetchConfig{
		MaxResponseBytes:      1024,
		SupportedContentTypes: []string{"text/html", "", "application/json"},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "[1] is empty") {
		t.Fatalf("expected empty content type error, got %v", err)
	}
}

func TestNormalizeContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{input: "text/html", expected: "text/html"},
		{input: " Text/HTML ", expected: "text/html"},
		{input: "text/html; charset=utf-8", expected: "text/html"},
		{input: "APPLICATION/JSON", expected: "application/json"},
		{input: "   ", expected: ""},
		{input: "", expected: ""},
		{input: "application/x-www-form-urlencoded; charset=utf-8", expected: "application/x-www-form-urlencoded"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := normalizeContentType(tt.input)
			if got != tt.expected {
				t.Fatalf("normalizeContentType(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNormalizeContentTypesBehaviors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		values   []string
		defaults []string
		want     []string
	}{
		{
			name:     "deduplicates and cleans",
			values:   []string{"TEXT/HTML", " text/plain ", "text/html", "", "  "},
			defaults: []string{"text/html", "text/plain"},
			want:     []string{"text/html", "text/plain"},
		},
		{
			name:     "drops empty without fallback",
			values:   []string{"", "   "},
			defaults: []string{"text/html", "text/plain"},
			want:     []string{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := normalizeContentTypes(tt.values, tt.defaults)
			if !reflect.DeepEqual(result, tt.want) {
				t.Fatalf("normalizeContentTypes(%+v) = %+v, want %+v", tt.values, result, tt.want)
			}
		})
	}
}

func TestToolsConfigCloneIndependence(t *testing.T) {
	t.Parallel()

	original := ToolsConfig{
		WebFetch: WebFetchConfig{
			MaxResponseBytes:      2048,
			SupportedContentTypes: []string{"text/html"},
		},
		MCP: MCPConfig{
			Servers: []MCPServerConfig{{ID: "s1"}},
		},
	}
	cloned := original.Clone()
	cloned.WebFetch.MaxResponseBytes = 999
	cloned.MCP.Servers[0].ID = "s2"

	if original.WebFetch.MaxResponseBytes == cloned.WebFetch.MaxResponseBytes {
		t.Fatal("expected WebFetch clone independence")
	}
	if original.MCP.Servers[0].ID == cloned.MCP.Servers[0].ID {
		t.Fatal("expected MCP clone independence")
	}
}

func TestWebFetchConfigApplyDefaultsNilReceiver(t *testing.T) {
	t.Parallel()

	var wfCfg *WebFetchConfig
	wfCfg.ApplyDefaults(WebFetchConfig{
		MaxResponseBytes:      512,
		SupportedContentTypes: []string{"text/plain"},
	})
}

func TestWebFetchConfigApplyDefaultsPreservesExplicitMaxResponseBytes(t *testing.T) {
	t.Parallel()

	cfg := WebFetchConfig{MaxResponseBytes: 9999}
	cfg.ApplyDefaults(WebFetchConfig{MaxResponseBytes: 100})
	if cfg.MaxResponseBytes != 9999 {
		t.Fatalf("expected explicit MaxResponseBytes=9999 to be preserved, got %d", cfg.MaxResponseBytes)
	}
}
