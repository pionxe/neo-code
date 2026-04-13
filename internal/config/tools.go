package config

import (
	"errors"
	"fmt"
	"mime"
	"strings"
)

const (
	DefaultWebFetchMaxResponseBytes int64 = 256 * 1024
)

var defaultWebFetchSupportedContentTypes = []string{
	"text/html",
	"application/xhtml+xml",
	"text/plain",
	"application/json",
	"application/xml",
	"text/xml",
}

// DefaultWebFetchSupportedContentTypes 返回 WebFetch 允许抓取的默认内容类型列表副本。
func DefaultWebFetchSupportedContentTypes() []string {
	return append([]string(nil), defaultWebFetchSupportedContentTypes...)
}

type ToolsConfig struct {
	WebFetch WebFetchConfig `yaml:"webfetch,omitempty"`
	MCP      MCPConfig      `yaml:"mcp,omitempty"`
}

type WebFetchConfig struct {
	MaxResponseBytes      int64    `yaml:"max_response_bytes,omitempty"`
	SupportedContentTypes []string `yaml:"supported_content_types,omitempty"`
}

func defaultWebFetchConfig() WebFetchConfig {
	return WebFetchConfig{
		MaxResponseBytes:      DefaultWebFetchMaxResponseBytes,
		SupportedContentTypes: DefaultWebFetchSupportedContentTypes(),
	}
}

func (c ToolsConfig) Clone() ToolsConfig {
	return ToolsConfig{
		WebFetch: c.WebFetch.Clone(),
		MCP:      c.MCP.Clone(),
	}
}

func (c *ToolsConfig) ApplyDefaults(defaults ToolsConfig) {
	if c == nil {
		return
	}

	c.WebFetch.ApplyDefaults(defaults.WebFetch)
	c.MCP.ApplyDefaults(defaults.MCP)
}

func (c ToolsConfig) Validate() error {
	if err := c.WebFetch.Validate(); err != nil {
		return fmt.Errorf("webfetch: %w", err)
	}
	if err := c.MCP.Validate(); err != nil {
		return fmt.Errorf("mcp: %w", err)
	}
	return nil
}

func (c WebFetchConfig) Clone() WebFetchConfig {
	clone := c
	clone.SupportedContentTypes = append([]string(nil), c.SupportedContentTypes...)
	return clone
}

func (c *WebFetchConfig) ApplyDefaults(defaults WebFetchConfig) {
	if c == nil {
		return
	}

	if c.MaxResponseBytes <= 0 {
		c.MaxResponseBytes = defaults.MaxResponseBytes
	}
	c.SupportedContentTypes = normalizeContentTypes(c.SupportedContentTypes, defaults.SupportedContentTypes)
}

func (c WebFetchConfig) Validate() error {
	if c.MaxResponseBytes <= 0 {
		return errors.New("max_response_bytes must be greater than 0")
	}
	if len(c.SupportedContentTypes) == 0 {
		return errors.New("supported_content_types is empty")
	}

	for i, contentType := range c.SupportedContentTypes {
		if normalizeContentType(contentType) == "" {
			return fmt.Errorf("supported_content_types[%d] is empty", i)
		}
	}
	return nil
}

func normalizeContentTypes(values []string, defaults []string) []string {
	source := values
	if len(source) == 0 {
		source = defaults
	}

	normalized := make([]string, 0, len(source))
	seen := make(map[string]struct{}, len(source))
	for _, value := range source {
		contentType := normalizeContentType(value)
		if contentType == "" {
			continue
		}
		if _, exists := seen[contentType]; exists {
			continue
		}
		seen[contentType] = struct{}{}
		normalized = append(normalized, contentType)
	}
	return normalized
}

func normalizeContentType(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return ""
	}

	mediaType, _, err := mime.ParseMediaType(trimmed)
	if err == nil {
		return mediaType
	}

	if index := strings.Index(trimmed, ";"); index >= 0 {
		return strings.TrimSpace(trimmed[:index])
	}
	return trimmed
}
