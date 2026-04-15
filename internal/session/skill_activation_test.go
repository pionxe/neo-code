package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	providertypes "neo-code/internal/provider/types"
)

func TestSessionSkillActivationHelpers(t *testing.T) {
	t.Parallel()

	session := New("skills")
	if !session.ActivateSkill("  Go_Review  ") {
		t.Fatalf("expected first activation to report change")
	}
	if session.ActivateSkill("go-review") {
		t.Fatalf("expected duplicate activation to be idempotent")
	}
	if !session.ActivateSkill("zeta") {
		t.Fatalf("expected second activation to report change")
	}
	if got := session.ActiveSkillIDs(); len(got) != 2 || got[0] != "go-review" || got[1] != "zeta" {
		t.Fatalf("unexpected active skill ids: %+v", got)
	}
	if !session.DeactivateSkill("GO_REVIEW") {
		t.Fatalf("expected deactivate to remove normalized id")
	}
	if session.DeactivateSkill("go-review") {
		t.Fatalf("expected duplicate deactivate to be idempotent")
	}
	if got := session.ActiveSkillIDs(); len(got) != 1 || got[0] != "zeta" {
		t.Fatalf("unexpected active skill ids after deactivate: %+v", got)
	}
}

func TestJSONStoreSaveLoadRoundTripActivatedSkills(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	workspaceRoot := t.TempDir()
	store := NewJSONStore(baseDir, workspaceRoot)

	session := &Session{
		SchemaVersion: CurrentSchemaVersion,
		ID:            "skills-round-trip",
		Title:         "Skills Round Trip",
		CreatedAt:     time.Now().Add(-time.Minute),
		UpdatedAt:     time.Now(),
		TaskState:     TaskState{},
		ActivatedSkills: []SkillActivation{
			{SkillID: "  zeta "},
			{SkillID: "go_review"},
			{SkillID: "go-review"},
		},
		Messages: []providertypes.Message{{Role: "user", Content: "hello"}},
	}

	if err := store.Save(context.Background(), session); err != nil {
		t.Fatalf("save session with activated skills: %v", err)
	}
	if got := session.ActiveSkillIDs(); len(got) != 2 || got[0] != "go-review" || got[1] != "zeta" {
		t.Fatalf("expected normalized in-memory activations, got %+v", got)
	}

	loaded, err := store.Load(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("load session with activated skills: %v", err)
	}
	if got := loaded.ActiveSkillIDs(); len(got) != 2 || got[0] != "go-review" || got[1] != "zeta" {
		t.Fatalf("expected normalized loaded activations, got %+v", got)
	}

	rawPath := filepath.Join(sessionDirectory(baseDir, workspaceRoot), session.ID+".json")
	raw, err := os.ReadFile(rawPath)
	if err != nil {
		t.Fatalf("read saved session: %v", err)
	}
	if !strings.Contains(string(raw), "\"activated_skills\"") {
		t.Fatalf("expected persisted activated_skills field, got:\n%s", string(raw))
	}
}

func TestJSONStoreLoadAllowsMissingActivatedSkillsField(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	workspaceRoot := t.TempDir()
	store := NewJSONStore(baseDir, workspaceRoot)

	mustWriteSessionFile(t, filepath.Join(sessionDirectory(baseDir, workspaceRoot), "no-activated-skills.json"), strings.Join([]string{
		`{`,
		`  "schema_version": 1,`,
		`  "id": "no-activated-skills",`,
		`  "title": "No Activated Skills",`,
		`  "created_at": "2026-04-15T10:00:00Z",`,
		`  "updated_at": "2026-04-15T10:05:00Z",`,
		`  "task_state": {`,
		`    "goal": "",`,
		`    "progress": [],`,
		`    "open_items": [],`,
		`    "next_step": "",`,
		`    "blockers": [],`,
		`    "key_artifacts": [],`,
		`    "decisions": [],`,
		`    "user_constraints": [],`,
		`    "last_updated_at": "2026-04-15T10:05:00Z"`,
		`  },`,
		`  "messages": []`,
		`}`,
	}, "\n"))

	loaded, err := store.Load(context.Background(), "no-activated-skills")
	if err != nil {
		t.Fatalf("load session without activated_skills field: %v", err)
	}
	if len(loaded.ActiveSkillIDs()) != 0 {
		t.Fatalf("expected no activated skills, got %+v", loaded.ActiveSkillIDs())
	}
}
