package tools

import (
	"errors"
	"strings"
	"testing"
)

func TestApplyOutputLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		result            ToolResult
		limit             int
		wantContent       string
		wantTruncated     bool
		wantMetadataNil   bool
		wantPreserveValue any
	}{
		{
			name: "no limit keeps content",
			result: ToolResult{
				Content: "hello",
			},
			limit:           0,
			wantContent:     "hello",
			wantTruncated:   false,
			wantMetadataNil: true,
		},
		{
			name: "content within limit keeps metadata",
			result: ToolResult{
				Content:  "hello",
				Metadata: map[string]any{"path": "a.txt"},
			},
			limit:             10,
			wantContent:       "hello",
			wantTruncated:     false,
			wantMetadataNil:   false,
			wantPreserveValue: "a.txt",
		},
		{
			name: "content over limit truncates and marks metadata",
			result: ToolResult{
				Content: "hello world",
			},
			limit:           5,
			wantContent:     "hello" + truncatedSuffix,
			wantTruncated:   true,
			wantMetadataNil: false,
		},
		{
			name: "existing truncated true is preserved",
			result: ToolResult{
				Content:  "hello world",
				Metadata: map[string]any{"truncated": true},
			},
			limit:           5,
			wantContent:     "hello" + truncatedSuffix,
			wantTruncated:   true,
			wantMetadataNil: false,
		},
		{
			name: "existing truncated false is overwritten",
			result: ToolResult{
				Content:  "hello world",
				Metadata: map[string]any{"truncated": false},
			},
			limit:           5,
			wantContent:     "hello" + truncatedSuffix,
			wantTruncated:   true,
			wantMetadataNil: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ApplyOutputLimit(tt.result, tt.limit)
			if got.Content != tt.wantContent {
				t.Fatalf("expected content %q, got %q", tt.wantContent, got.Content)
			}

			if tt.wantMetadataNil {
				if got.Metadata != nil {
					t.Fatalf("expected nil metadata, got %#v", got.Metadata)
				}
				return
			}

			if got.Metadata == nil {
				t.Fatal("expected metadata to be initialized")
			}
			if truncated, _ := got.Metadata["truncated"].(bool); truncated != tt.wantTruncated {
				t.Fatalf("expected truncated=%v, got %#v", tt.wantTruncated, got.Metadata["truncated"])
			}
			if tt.wantPreserveValue != nil && got.Metadata["path"] != tt.wantPreserveValue {
				t.Fatalf("expected path metadata %v, got %v", tt.wantPreserveValue, got.Metadata["path"])
			}
		})
	}
}

func TestFormatHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		toolName   string
		reason     string
		details    string
		err        error
		wantReason string
		wantBody   []string
	}{
		{
			name:       "format trims fields",
			toolName:   " bash ",
			reason:     " failed ",
			details:    " bad input ",
			err:        errors.New("bash: failed"),
			wantReason: "failed",
			wantBody:   []string{"tool error", "tool: bash", "reason: failed", "details: bad input"},
		},
		{
			name:       "normalize without tool prefix keeps message",
			toolName:   "webfetch",
			reason:     "unsupported content type",
			details:    "",
			err:        errors.New("network unavailable"),
			wantReason: "network unavailable",
			wantBody:   []string{"tool error", "tool: webfetch", "reason: unsupported content type"},
		},
		{
			name:       "empty fields collapse cleanly",
			toolName:   "",
			reason:     "",
			details:    "",
			err:        nil,
			wantReason: "",
			wantBody:   []string{"tool error"},
		},
		{
			name:       "tool name empty keeps raw reason",
			toolName:   "",
			reason:     "boom",
			details:    "",
			err:        errors.New("bash: failed"),
			wantReason: "bash: failed",
			wantBody:   []string{"tool error", "reason: boom"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := NormalizeErrorReason(tt.toolName, tt.err); got != tt.wantReason {
				t.Fatalf("expected reason %q, got %q", tt.wantReason, got)
			}

			body := FormatError(tt.toolName, tt.reason, tt.details)
			for _, fragment := range tt.wantBody {
				if !strings.Contains(body, fragment) {
					t.Fatalf("expected body to contain %q, got %q", fragment, body)
				}
			}

			result := NewErrorResult(tt.toolName, tt.reason, tt.details, map[string]any{"k": "v"})
			if !result.IsError {
				t.Fatal("expected error result")
			}
			if result.Name != tt.toolName {
				t.Fatalf("expected name %q, got %q", tt.toolName, result.Name)
			}
			if result.Metadata["k"] != "v" {
				t.Fatalf("expected metadata to be preserved, got %#v", result.Metadata)
			}
			if result.Content != body {
				t.Fatalf("expected content %q, got %q", body, result.Content)
			}
		})
	}
}
