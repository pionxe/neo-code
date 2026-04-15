package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"neo-code/internal/config"
	agentcontext "neo-code/internal/context"
	contextcompact "neo-code/internal/context/compact"
	providertypes "neo-code/internal/provider/types"
	"neo-code/internal/skills"
	"neo-code/internal/tools"
)

type stubSkillsRegistry struct {
	skills map[string]skills.Skill
	getErr error
}

func (r *stubSkillsRegistry) List(ctx context.Context, input skills.ListInput) ([]skills.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]skills.Descriptor, 0, len(r.skills))
	for _, skill := range r.skills {
		result = append(result, skill.Descriptor)
	}
	return result, nil
}

func (r *stubSkillsRegistry) Get(ctx context.Context, id string) (skills.Descriptor, skills.Content, error) {
	if err := ctx.Err(); err != nil {
		return skills.Descriptor{}, skills.Content{}, err
	}
	if r.getErr != nil {
		return skills.Descriptor{}, skills.Content{}, r.getErr
	}
	for _, skill := range r.skills {
		if normalizeRuntimeSkillID(skill.Descriptor.ID) == normalizeRuntimeSkillID(id) {
			return skill.Descriptor, skill.Content, nil
		}
	}
	return skills.Descriptor{}, skills.Content{}, fmt.Errorf("%w: %s", skills.ErrSkillNotFound, id)
}

func (r *stubSkillsRegistry) Refresh(ctx context.Context) error {
	return ctx.Err()
}

func TestActivateSessionSkillPersistsAndEmitsEvent(t *testing.T) {
	t.Parallel()

	manager := newRuntimeConfigManager(t)
	store := newMemoryStore()
	session := newRuntimeSession("session-activate-skill")
	store.sessions[session.ID] = cloneSession(session)

	service := NewWithFactory(manager, &stubToolManager{}, store, &scriptedProviderFactory{provider: &scriptedProvider{}}, &stubContextBuilder{})
	service.SetSkillsRegistry(&stubSkillsRegistry{
		skills: map[string]skills.Skill{
			"go-review": {
				Descriptor: skills.Descriptor{ID: "go-review", Name: "Go Review"},
				Content:    skills.Content{Instruction: "review code"},
			},
		},
	})

	if err := service.ActivateSessionSkill(context.Background(), session.ID, "go_review"); err != nil {
		t.Fatalf("ActivateSessionSkill() error = %v", err)
	}

	loaded := store.sessions[session.ID]
	if got := loaded.ActiveSkillIDs(); len(got) != 1 || got[0] != "go-review" {
		t.Fatalf("expected activated skill persisted, got %+v", got)
	}

	events := collectRuntimeEvents(service.Events())
	if len(events) != 1 || events[0].Type != EventSkillActivated {
		t.Fatalf("expected skill_activated event, got %+v", events)
	}
	payload, ok := events[0].Payload.(SessionSkillEventPayload)
	if !ok || payload.SkillID != "go-review" {
		t.Fatalf("unexpected event payload: %+v", events[0].Payload)
	}
}

func TestActivateSessionSkillRejectsMissingSkill(t *testing.T) {
	t.Parallel()

	manager := newRuntimeConfigManager(t)
	store := newMemoryStore()
	session := newRuntimeSession("session-activate-missing")
	store.sessions[session.ID] = cloneSession(session)

	service := NewWithFactory(manager, &stubToolManager{}, store, &scriptedProviderFactory{provider: &scriptedProvider{}}, &stubContextBuilder{})
	service.SetSkillsRegistry(&stubSkillsRegistry{skills: map[string]skills.Skill{}})

	if err := service.ActivateSessionSkill(context.Background(), session.ID, "missing"); err == nil {
		t.Fatalf("expected missing skill activation to fail")
	}
	if got := store.sessions[session.ID].ActiveSkillIDs(); len(got) != 0 {
		t.Fatalf("expected session to remain clean, got %+v", got)
	}
}

func TestDeactivateSessionSkillIsIdempotentForUnknownSkill(t *testing.T) {
	t.Parallel()

	manager := newRuntimeConfigManager(t)
	store := newMemoryStore()
	session := newRuntimeSession("session-deactivate-skill")
	session.ActivateSkill("go-review")
	store.sessions[session.ID] = cloneSession(session)

	service := NewWithFactory(manager, &stubToolManager{}, store, &scriptedProviderFactory{provider: &scriptedProvider{}}, &stubContextBuilder{})
	if err := service.DeactivateSessionSkill(context.Background(), session.ID, "missing"); err != nil {
		t.Fatalf("DeactivateSessionSkill() error = %v", err)
	}
	if got := store.sessions[session.ID].ActiveSkillIDs(); len(got) != 1 || got[0] != "go-review" {
		t.Fatalf("expected unchanged activations, got %+v", got)
	}
}

func TestPrepareTurnSnapshotPassesResolvedSkillsToContextBuilder(t *testing.T) {
	t.Parallel()

	manager := newRuntimeConfigManager(t)
	store := newMemoryStore()
	session := newRuntimeSession("session-build-skill")
	session.ActivateSkill("go-review")
	store.sessions[session.ID] = cloneSession(session)

	builder := &stubContextBuilder{}
	service := NewWithFactory(manager, &stubToolManager{}, store, &scriptedProviderFactory{provider: &scriptedProvider{}}, builder)
	service.SetSkillsRegistry(&stubSkillsRegistry{
		skills: map[string]skills.Skill{
			"go-review": {
				Descriptor: skills.Descriptor{ID: "go-review", Name: "Go Review"},
				Content:    skills.Content{Instruction: "review code"},
			},
		},
	})

	state := newRunState("run-build-skill", session)
	if _, rebuilt, err := service.prepareTurnSnapshot(context.Background(), &state); err != nil {
		t.Fatalf("prepareTurnSnapshot() error = %v", err)
	} else if rebuilt {
		t.Fatalf("did not expect snapshot rebuild")
	}
	if len(builder.lastInput.ActiveSkills) != 1 || builder.lastInput.ActiveSkills[0].Descriptor.ID != "go-review" {
		t.Fatalf("expected active skills forwarded to context builder, got %+v", builder.lastInput.ActiveSkills)
	}
}

func TestPrepareTurnSnapshotEmitsSkillMissingAndContinues(t *testing.T) {
	t.Parallel()

	manager := newRuntimeConfigManager(t)
	store := newMemoryStore()
	session := newRuntimeSession("session-missing-skill")
	session.ActivateSkill("missing-skill")
	store.sessions[session.ID] = cloneSession(session)

	builder := &stubContextBuilder{}
	service := NewWithFactory(manager, &stubToolManager{}, store, &scriptedProviderFactory{provider: &scriptedProvider{}}, builder)

	state := newRunState("run-missing-skill", session)
	if _, rebuilt, err := service.prepareTurnSnapshot(context.Background(), &state); err != nil {
		t.Fatalf("prepareTurnSnapshot() error = %v", err)
	} else if rebuilt {
		t.Fatalf("did not expect snapshot rebuild")
	}
	if len(builder.lastInput.ActiveSkills) != 0 {
		t.Fatalf("expected missing skill to be skipped, got %+v", builder.lastInput.ActiveSkills)
	}

	events := collectRuntimeEvents(service.Events())
	if len(events) != 1 || events[0].Type != EventSkillMissing {
		t.Fatalf("expected skill_missing event, got %+v", events)
	}
}

func TestPrepareTurnSnapshotPropagatesRegistryFailure(t *testing.T) {
	t.Parallel()

	manager := newRuntimeConfigManager(t)
	store := newMemoryStore()
	session := newRuntimeSession("session-skill-registry-failure")
	session.ActivateSkill("go-review")
	store.sessions[session.ID] = cloneSession(session)

	builder := &stubContextBuilder{}
	service := NewWithFactory(manager, &stubToolManager{}, store, &scriptedProviderFactory{provider: &scriptedProvider{}}, builder)
	service.SetSkillsRegistry(&stubSkillsRegistry{getErr: os.ErrPermission})

	state := newRunState("run-skill-registry-failure", session)
	if _, _, err := service.prepareTurnSnapshot(context.Background(), &state); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected registry failure to propagate, got %v", err)
	}
	if len(collectRuntimeEvents(service.Events())) != 0 {
		t.Fatalf("expected no skill_missing event on registry failure")
	}
}

func TestListSessionSkillsPropagatesRegistryFailure(t *testing.T) {
	t.Parallel()

	manager := newRuntimeConfigManager(t)
	store := newMemoryStore()
	session := newRuntimeSession("session-list-skill-registry-failure")
	session.ActivateSkill("go-review")
	store.sessions[session.ID] = cloneSession(session)

	service := NewWithFactory(manager, &stubToolManager{}, store, &scriptedProviderFactory{provider: &scriptedProvider{}}, &stubContextBuilder{})
	service.SetSkillsRegistry(&stubSkillsRegistry{getErr: os.ErrPermission})

	if _, err := service.ListSessionSkills(context.Background(), session.ID); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected registry failure to propagate, got %v", err)
	}
}

func TestServiceRunReinjectsSkillsAfterAutoCompact(t *testing.T) {
	t.Parallel()

	manager := newRuntimeConfigManager(t)
	if err := manager.Update(context.Background(), func(cfg *config.Config) error {
		cfg.Context.AutoCompact.Enabled = true
		cfg.Context.AutoCompact.InputTokenThreshold = 1
		return nil
	}); err != nil {
		t.Fatalf("update config: %v", err)
	}

	store := newMemoryStore()
	session := newRuntimeSession("session-auto-compact-skills")
	session.ActivateSkill("go-review")
	session.TokenInputTotal = 3
	store.sessions[session.ID] = cloneSession(session)

	builder := &stubContextBuilder{
		buildFn: func(ctx context.Context, input agentcontext.BuildInput) (agentcontext.BuildResult, error) {
			return agentcontext.BuildResult{
				SystemPrompt:         "prompt",
				Messages:             append([]providertypes.Message(nil), input.Messages...),
				AutoCompactSuggested: input.Metadata.SessionInputTokens >= 1,
			}, nil
		},
	}
	compactRunner := &stubCompactRunner{
		runFn: func(ctx context.Context, input contextcompact.Input) (contextcompact.Result, error) {
			return contextcompact.Result{
				Applied:   true,
				Messages:  append([]providertypes.Message(nil), input.Messages...),
				TaskState: input.TaskState.Clone(),
				Metrics: contextcompact.Metrics{
					TriggerMode: string(contextcompact.ModeAuto),
				},
			}, nil
		},
	}
	scripted := &scriptedProvider{
		streams: [][]providertypes.StreamEvent{
			{providertypes.NewTextDeltaStreamEvent("done")},
		},
	}
	registry := tools.NewRegistry()
	registry.Register(&stubTool{name: "filesystem_read_file", content: "default"})

	service := NewWithFactory(manager, registry, store, &scriptedProviderFactory{provider: scripted}, builder)
	service.compactRunner = compactRunner
	service.SetSkillsRegistry(&stubSkillsRegistry{
		skills: map[string]skills.Skill{
			"go-review": {
				Descriptor: skills.Descriptor{ID: "go-review", Name: "Go Review"},
				Content:    skills.Content{Instruction: "review code"},
			},
		},
	})

	if err := service.Run(context.Background(), UserInput{SessionID: session.ID, RunID: "run-auto-compact-skills", Content: "hello"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(builder.builds) < 2 {
		t.Fatalf("expected context builder to run before and after compact, got %d", len(builder.builds))
	}
	for idx, build := range builder.builds[:2] {
		if len(build.ActiveSkills) != 1 || build.ActiveSkills[0].Descriptor.ID != "go-review" {
			t.Fatalf("expected active skill on build %d, got %+v", idx, build.ActiveSkills)
		}
	}
}
