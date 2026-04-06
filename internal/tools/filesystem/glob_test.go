package filesystem

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"neo-code/internal/tools"
)

func TestGlobToolExecute(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, "main.go"), "package main\n")
	mustWriteFile(t, filepath.Join(workspace, "README.md"), "# readme\n")
	mustWriteFile(t, filepath.Join(workspace, "internal", "app", "app.go"), "package app\n")
	mustWriteFile(t, filepath.Join(workspace, "node_modules", "skip.go"), "package skip\n")

	tests := []struct {
		name           string
		pattern        string
		dir            string
		expectContains []string
		expectErr      string
		expectNoMatch  bool
	}{
		{
			name:           "glob go files recursively",
			pattern:        "**/*.go",
			expectContains: []string{"main.go", normalizeSlashPath(filepath.Join("internal", "app", "app.go"))},
		},
		{
			name:           "scope to directory",
			pattern:        "**/*.go",
			dir:            "internal",
			expectContains: []string{normalizeSlashPath(filepath.Join("internal", "app", "app.go"))},
		},
		{
			name:          "no matches",
			pattern:       "**/*.py",
			expectNoMatch: true,
		},
		{
			name:      "empty pattern",
			pattern:   "",
			expectErr: "pattern is required",
		},
	}

	tool := NewGlob(workspace)
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			args, err := json.Marshal(map[string]string{
				"pattern": tt.pattern,
				"dir":     tt.dir,
			})
			if err != nil {
				t.Fatalf("marshal args: %v", err)
			}

			result, execErr := tool.Execute(context.Background(), tools.ToolCallInput{
				Name:      tool.Name(),
				Arguments: args,
				Workdir:   workspace,
			})

			if tt.expectErr != "" {
				if execErr == nil || !strings.Contains(execErr.Error(), tt.expectErr) {
					t.Fatalf("expected error containing %q, got %v", tt.expectErr, execErr)
				}
				return
			}
			if execErr != nil {
				t.Fatalf("unexpected error: %v", execErr)
			}
			if tt.expectNoMatch {
				if result.Content != "no matches" {
					t.Fatalf("expected no matches, got %q", result.Content)
				}
				return
			}
			normalizedContent := normalizeSlashPath(result.Content)
			for _, expected := range tt.expectContains {
				if !strings.Contains(normalizedContent, normalizeSlashPath(expected)) {
					t.Fatalf("expected result to contain %q, got %q", expected, result.Content)
				}
			}
			if strings.Contains(normalizedContent, "node_modules") {
				t.Fatalf("expected node_modules files to be skipped, got %q", result.Content)
			}
		})
	}
}

func TestBuildGlobMatcherRejectsInvalidUTF8(t *testing.T) {
	t.Parallel()

	_, err := buildGlobMatcher(string([]byte{0xff}))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "utf-8") {
		t.Fatalf("expected invalid utf-8 error, got %v", err)
	}
}

func TestGlobToolErrorFormattingAndTruncation(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	for i := 0; i < 1500; i++ {
		mustWriteFile(t, filepath.Join(workspace, "many", strings.Repeat("a", 20), strings.Repeat("b", 20), "file"+strconv.Itoa(i)+".txt"), "x")
	}

	tool := NewGlob(workspace)
	tests := []struct {
		name           string
		ctx            func() context.Context
		arguments      []byte
		expectErr      string
		expectContent  []string
		expectTruncate bool
	}{
		{
			name:          "invalid json arguments",
			ctx:           context.Background,
			arguments:     []byte(`{invalid`),
			expectErr:     "invalid character",
			expectContent: []string{"tool error", "tool: filesystem_glob", "reason: invalid arguments"},
		},
		{
			name: "canceled context",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			arguments:     mustMarshalFSArgs(t, map[string]string{"pattern": "**/*.txt"}),
			expectErr:     "context canceled",
			expectContent: []string{"tool error", "tool: filesystem_glob", "reason: context canceled"},
		},
		{
			name:           "long output is truncated",
			ctx:            context.Background,
			arguments:      mustMarshalFSArgs(t, map[string]string{"pattern": "**/*.txt"}),
			expectContent:  []string{"...[truncated]"},
			expectTruncate: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := tool.Execute(tt.ctx(), tools.ToolCallInput{
				Name:      tool.Name(),
				Arguments: tt.arguments,
				Workdir:   workspace,
			})

			if tt.expectErr != "" {
				if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.expectErr)) {
					t.Fatalf("expected error containing %q, got %v", tt.expectErr, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, fragment := range tt.expectContent {
				if !strings.Contains(result.Content, fragment) {
					t.Fatalf("expected content containing %q, got %q", fragment, result.Content)
				}
			}
			if truncated, _ := result.Metadata["truncated"].(bool); truncated != tt.expectTruncate {
				t.Fatalf("expected truncated=%v, got %#v", tt.expectTruncate, result.Metadata["truncated"])
			}
		})
	}
}
