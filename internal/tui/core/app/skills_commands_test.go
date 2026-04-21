package tui

import (
	"errors"
	"strings"
	"testing"

	agentruntime "neo-code/internal/runtime"
	"neo-code/internal/skills"
)

func TestFormatAvailableSkills(t *testing.T) {
	t.Parallel()

	if got := formatAvailableSkills(nil, ""); !strings.Contains(got, "No skills found") {
		t.Fatalf("expected empty message, got %q", got)
	}

	text := formatAvailableSkills([]agentruntime.AvailableSkillState{
		{
			Descriptor: skills.Descriptor{
				ID:          "go-review",
				Description: "review go code",
				Scope:       skills.ScopeSession,
				Version:     "v1",
				Source:      skills.Source{Kind: skills.SourceKindLocal},
			},
			Active: true,
		},
	}, "session-1")
	if !strings.Contains(text, "go-review [active]") {
		t.Fatalf("expected active entry, got %q", text)
	}
}

func TestFormatSessionSkills(t *testing.T) {
	t.Parallel()

	if got := formatSessionSkills(nil); !strings.Contains(got, "No active skills") {
		t.Fatalf("expected empty active message, got %q", got)
	}

	text := formatSessionSkills([]agentruntime.SessionSkillState{
		{SkillID: "missing", Missing: true},
		{SkillID: "go-review", Descriptor: &skills.Descriptor{ID: "go-review", Description: "review"}},
	})
	if !strings.Contains(text, "missing [missing]") {
		t.Fatalf("expected missing entry, got %q", text)
	}
	if !strings.Contains(text, "go-review [active]") {
		t.Fatalf("expected active entry, got %q", text)
	}
}

func TestSkillCommandErrorAndPlaceholderHelpers(t *testing.T) {
	t.Parallel()

	if !isSkillUsagePlaceholder("<id>") {
		t.Fatalf("expected placeholder marker")
	}
	if isSkillUsagePlaceholder("go-review") {
		t.Fatalf("did not expect normal id as placeholder")
	}

	unsupported := normalizeSkillCommandError(errors.New("unsupported_action_in_gateway_mode"))
	if unsupported == nil || !strings.Contains(strings.ToLower(unsupported.Error()), "gateway") {
		t.Fatalf("expected gateway hint, got %v", unsupported)
	}
	plain := errors.New("plain")
	if normalizeSkillCommandError(plain) != plain {
		t.Fatalf("expected non-gateway error passthrough")
	}
}
