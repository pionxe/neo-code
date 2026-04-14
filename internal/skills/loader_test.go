package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalLoaderLoad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		setup         func(t *testing.T, root string)
		wantSkills    int
		wantIssueCode LoadIssueCode
	}{
		{
			name: "load success with frontmatter and sections",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeSkillFile(t, root, "go-review", `---
id: go-review
name: Go Review
description: review go code
version: v1.2.0
scope: explicit
source: local
tool_hints:
  - filesystem_read_file
---
# Go Review
## Instruction
Review Go code with concrete findings.

## References
- [Go Spec](https://go.dev/ref/spec)

## Examples
- report critical bug with file path and line

## ToolHints
- filesystem_grep
`)
			},
			wantSkills: 1,
		},
		{
			name: "missing skill file creates issue but no fatal error",
			setup: func(t *testing.T, root string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(root, "missing-skill"), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
			},
			wantSkills:    0,
			wantIssueCode: IssueSkillFileMissing,
		},
		{
			name: "invalid metadata creates issue and skips bad skill",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeSkillFile(t, root, "bad-metadata", `---
id: bad-meta
scope: invalid_scope
---
# bad
## Instruction
test`)
			},
			wantSkills:    0,
			wantIssueCode: IssueInvalidMetadata,
		},
		{
			name: "empty content creates issue and skips bad skill",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeSkillFile(t, root, "empty-content", `---
id: empty-content
scope: explicit
---
`)
			},
			wantSkills:    0,
			wantIssueCode: IssueEmptyContent,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			tt.setup(t, root)

			loader := NewLocalLoader(root)
			snapshot, err := loader.Load(context.Background())
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if len(snapshot.Skills) != tt.wantSkills {
				t.Fatalf("skills count = %d, want %d", len(snapshot.Skills), tt.wantSkills)
			}
			if tt.wantIssueCode != "" {
				found := false
				for _, issue := range snapshot.Issues {
					if issue.Code == tt.wantIssueCode {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("expected issue code %q, got %+v", tt.wantIssueCode, snapshot.Issues)
				}
			}

			if len(snapshot.Skills) > 0 {
				skill := snapshot.Skills[0]
				if strings.TrimSpace(skill.Descriptor.ID) == "" {
					t.Fatalf("expected non-empty skill id")
				}
				if strings.TrimSpace(skill.Content.Instruction) == "" {
					t.Fatalf("expected non-empty instruction")
				}
				if skill.Descriptor.Source.Kind != SourceKindLocal {
					t.Fatalf("expected source kind local, got %q", skill.Descriptor.Source.Kind)
				}
			}
		})
	}
}

func TestLocalLoaderLoadBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		root      string
		prepare   func(t *testing.T, root string)
		expectErr string
	}{
		{
			name:      "empty root",
			root:      "",
			expectErr: "root directory not found",
		},
		{
			name:      "root not exists",
			root:      filepath.Join(t.TempDir(), "missing-root"),
			expectErr: "root directory not found",
		},
		{
			name: "root is file",
			root: filepath.Join(t.TempDir(), "root.txt"),
			prepare: func(t *testing.T, root string) {
				t.Helper()
				if err := os.WriteFile(root, []byte("x"), 0o644); err != nil {
					t.Fatalf("write file: %v", err)
				}
			},
			expectErr: "not directory",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.prepare != nil {
				tt.prepare(t, tt.root)
			}
			loader := NewLocalLoader(tt.root)
			_, err := loader.Load(context.Background())
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.expectErr)) {
				t.Fatalf("expected error containing %q, got %v", tt.expectErr, err)
			}
		})
	}
}

func writeSkillFile(t *testing.T, root string, dir string, content string) {
	t.Helper()
	skillDir := filepath.Join(root, dir)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, skillFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}
