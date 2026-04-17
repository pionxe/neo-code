package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"neo-code/internal/config"
	providertypes "neo-code/internal/provider/types"
	agentsession "neo-code/internal/session"
)

func TestServicePrepareUserInputEmitsNormalizeAndAssetSaved(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	svc, _ := newPrepareTestService(t, workdir, true)

	imagePath := filepath.Join(workdir, "img.png")
	if err := os.WriteFile(imagePath, []byte("fake-png"), 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	input, err := svc.PrepareUserInput(context.Background(), PrepareInput{
		RunID:  "run-prepare-1",
		Text:   "hello",
		Images: []UserImageInput{{Path: imagePath, MimeType: "image/png"}},
	})
	if err != nil {
		t.Fatalf("PrepareUserInput() error = %v", err)
	}
	if input.SessionID == "" || input.RunID != "run-prepare-1" {
		t.Fatalf("unexpected prepared user input: %+v", input)
	}
	if len(input.Parts) != 2 || input.Parts[0].Kind != providertypes.ContentPartText || input.Parts[1].Kind != providertypes.ContentPartImage {
		t.Fatalf("unexpected prepared parts: %+v", input.Parts)
	}

	normalizedEvent := mustReadRuntimeEvent(t, svc.Events())
	if normalizedEvent.Type != EventInputNormalized {
		t.Fatalf("expected first event %q, got %q", EventInputNormalized, normalizedEvent.Type)
	}
	normalizedPayload, ok := normalizedEvent.Payload.(InputNormalizedPayload)
	if !ok || normalizedPayload.ImageCount != 1 {
		t.Fatalf("unexpected normalized payload: %#v", normalizedEvent.Payload)
	}

	assetSavedEvent := mustReadRuntimeEvent(t, svc.Events())
	if assetSavedEvent.Type != EventAssetSaved {
		t.Fatalf("expected second event %q, got %q", EventAssetSaved, assetSavedEvent.Type)
	}
	assetSavedPayload, ok := assetSavedEvent.Payload.(AssetSavedPayload)
	if !ok || assetSavedPayload.AssetID == "" || assetSavedPayload.MimeType != "image/png" {
		t.Fatalf("unexpected asset_saved payload: %#v", assetSavedEvent.Payload)
	}
}

func TestServicePrepareUserInputEmitsAssetSaveFailed(t *testing.T) {
	t.Parallel()

	svc, _ := newPrepareTestService(t, t.TempDir(), true)
	_, err := svc.PrepareUserInput(context.Background(), PrepareInput{
		RunID:  "run-prepare-2",
		Text:   "hello",
		Images: []UserImageInput{{Path: filepath.Join(t.TempDir(), "missing.png"), MimeType: "image/png"}},
	})
	if err == nil {
		t.Fatalf("expected PrepareUserInput() to fail")
	}

	failedEvent := mustReadRuntimeEvent(t, svc.Events())
	if failedEvent.Type != EventAssetSaveFailed {
		t.Fatalf("expected event %q, got %q", EventAssetSaveFailed, failedEvent.Type)
	}
	payload, ok := failedEvent.Payload.(AssetSaveFailedPayload)
	if !ok || payload.Index != 0 {
		t.Fatalf("unexpected asset_save_failed payload: %#v", failedEvent.Payload)
	}
}

func TestServicePrepareUserInputWithoutPreparerEmitsErrorEvent(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	svc, _ := newPrepareTestService(t, workdir, false)

	_, err := svc.PrepareUserInput(context.Background(), PrepareInput{
		RunID: "run-prepare-3",
		Text:  "hello",
	})
	if err == nil {
		t.Fatalf("expected PrepareUserInput() to fail without preparer")
	}

	errorEvent := mustReadRuntimeEvent(t, svc.Events())
	if errorEvent.Type != EventError {
		t.Fatalf("expected event %q, got %q", EventError, errorEvent.Type)
	}
}

func TestServiceSubmitWithoutPreparerReturnsError(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	svc, _ := newPrepareTestService(t, workdir, false)

	err := svc.Submit(context.Background(), PrepareInput{
		RunID: "run-submit-1",
		Text:  "hello",
	})
	if err == nil {
		t.Fatalf("expected Submit() to fail without preparer")
	}

	errorEvent := mustReadRuntimeEvent(t, svc.Events())
	if errorEvent.Type != EventError {
		t.Fatalf("expected event %q, got %q", EventError, errorEvent.Type)
	}
}

func newPrepareTestService(t *testing.T, workdir string, withPreparer bool) (*Service, *agentsession.JSONStore) {
	t.Helper()

	cfg := config.StaticDefaults()
	cfg.Workdir = workdir
	loader := config.NewLoader(t.TempDir(), cfg)
	manager := config.NewManager(loader)
	if _, err := manager.Load(context.Background()); err != nil {
		t.Fatalf("load config: %v", err)
	}

	store := agentsession.NewStore(t.TempDir(), workdir)
	svc := NewWithFactory(manager, nil, store, nil, nil)
	svc.SetSessionAssetStore(store)
	if withPreparer {
		svc.SetUserInputPreparer(NewSessionInputPreparer(store, store))
	}
	return svc, store
}

func mustReadRuntimeEvent(t *testing.T, events <-chan RuntimeEvent) RuntimeEvent {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for runtime event")
		return RuntimeEvent{}
	}
}
