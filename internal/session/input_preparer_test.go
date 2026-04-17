package session

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	providertypes "neo-code/internal/provider/types"
)

func TestInputPreparerPrepareTextOnly(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	store := NewStore(t.TempDir(), workdir)
	preparer := NewInputPreparer(store, store)

	result, err := preparer.Prepare(context.Background(), PrepareInput{
		Text:           "hello world",
		DefaultWorkdir: workdir,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if result.SessionID == "" {
		t.Fatalf("expected non-empty session id")
	}
	if len(result.Parts) != 1 || result.Parts[0].Kind != providertypes.ContentPartText || result.Parts[0].Text != "hello world" {
		t.Fatalf("unexpected prepared parts: %+v", result.Parts)
	}
	if len(result.SavedAssets) != 0 {
		t.Fatalf("expected no saved assets, got %+v", result.SavedAssets)
	}
}

func TestInputPreparerPrepareTextAndImage(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	store := NewStore(t.TempDir(), workdir)
	preparer := NewInputPreparer(store, store)

	imagePath := filepath.Join(workdir, "img.png")
	payload := []byte("fake-png")
	if err := os.WriteFile(imagePath, payload, 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	result, err := preparer.Prepare(context.Background(), PrepareInput{
		Text:           "with image",
		Images:         []PrepareImageInput{{Path: imagePath, MimeType: "image/png"}},
		DefaultWorkdir: workdir,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if len(result.SavedAssets) != 1 {
		t.Fatalf("expected one saved asset, got %+v", result.SavedAssets)
	}
	if len(result.Parts) != 2 {
		t.Fatalf("expected 2 parts, got %+v", result.Parts)
	}
	imagePart := result.Parts[1]
	if imagePart.Kind != providertypes.ContentPartImage || imagePart.Image == nil || imagePart.Image.Asset == nil {
		t.Fatalf("expected session asset image part, got %+v", imagePart)
	}
	if imagePart.Image.Asset.ID != result.SavedAssets[0].ID {
		t.Fatalf("expected image part asset id %q, got %+v", result.SavedAssets[0].ID, imagePart.Image.Asset)
	}

	rc, meta, err := store.Open(context.Background(), result.SessionID, result.SavedAssets[0].ID)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = rc.Close() }()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if meta.MimeType != "image/png" || string(got) != string(payload) {
		t.Fatalf("unexpected stored asset mime=%q payload=%q", meta.MimeType, string(got))
	}
}

func TestInputPreparerPrepareImageInfersMimeWhenMissing(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	store := NewStore(t.TempDir(), workdir)
	preparer := NewInputPreparer(store, store)

	imagePath := filepath.Join(workdir, "auto.png")
	if err := os.WriteFile(imagePath, []byte("fake-png"), 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	result, err := preparer.Prepare(context.Background(), PrepareInput{
		Text:           "infer mime",
		Images:         []PrepareImageInput{{Path: imagePath}},
		DefaultWorkdir: workdir,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if len(result.SavedAssets) != 1 {
		t.Fatalf("expected one saved asset, got %+v", result.SavedAssets)
	}
	if result.SavedAssets[0].MimeType != "image/png" {
		t.Fatalf("expected inferred mime image/png, got %q", result.SavedAssets[0].MimeType)
	}
}

func TestInputPreparerPrepareImageOnlyUsesImageTitle(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	store := NewStore(t.TempDir(), workdir)
	preparer := NewInputPreparer(store, store)

	imagePath := filepath.Join(workdir, "only.png")
	if err := os.WriteFile(imagePath, []byte("img"), 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	result, err := preparer.Prepare(context.Background(), PrepareInput{
		Images:         []PrepareImageInput{{Path: imagePath, MimeType: "image/png"}},
		DefaultWorkdir: workdir,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if len(result.Parts) != 1 || result.Parts[0].Kind != providertypes.ContentPartImage {
		t.Fatalf("expected one image part, got %+v", result.Parts)
	}

	session, err := store.Load(context.Background(), result.SessionID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if session.Title != imageOnlySessionTitle {
		t.Fatalf("expected image-only title %q, got %q", imageOnlySessionTitle, session.Title)
	}
}

func TestInputPreparerPrepareErrors(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	store := NewStore(t.TempDir(), workdir)

	t.Run("missing store", func(t *testing.T) {
		preparer := NewInputPreparer(nil, nil)
		if _, err := preparer.Prepare(context.Background(), PrepareInput{Text: "x", DefaultWorkdir: workdir}); err == nil {
			t.Fatalf("expected missing store error")
		}
	})

	t.Run("missing asset store", func(t *testing.T) {
		preparer := NewInputPreparer(store, nil)
		_, err := preparer.Prepare(context.Background(), PrepareInput{
			Images:         []PrepareImageInput{{Path: "x", MimeType: "image/png"}},
			DefaultWorkdir: workdir,
		})
		if err == nil {
			t.Fatalf("expected missing asset store error")
		}
	})

	t.Run("empty content", func(t *testing.T) {
		preparer := NewInputPreparer(store, store)
		if _, err := preparer.Prepare(context.Background(), PrepareInput{DefaultWorkdir: workdir}); err == nil {
			t.Fatalf("expected empty content error")
		}
	})

	t.Run("asset save error is structured", func(t *testing.T) {
		preparer := NewInputPreparer(store, store)
		_, err := preparer.Prepare(context.Background(), PrepareInput{
			Images:         []PrepareImageInput{{Path: "not-found.png", MimeType: "image/png"}},
			DefaultWorkdir: workdir,
		})
		if err == nil {
			t.Fatalf("expected asset save error")
		}
		var saveErr *AssetSaveError
		if !errors.As(err, &saveErr) {
			t.Fatalf("expected AssetSaveError, got %T %v", err, err)
		}
		if saveErr.Index != 0 {
			t.Fatalf("expected save error index 0, got %d", saveErr.Index)
		}
	})
}

func TestInputPreparerPrepareUpdatesExistingSessionWorkdir(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	defaultWorkdir := filepath.Join(base, "workspace")
	if err := os.MkdirAll(defaultWorkdir, 0o755); err != nil {
		t.Fatalf("mkdir default workdir: %v", err)
	}
	currentWorkdir := filepath.Join(defaultWorkdir, "current")
	if err := os.MkdirAll(currentWorkdir, 0o755); err != nil {
		t.Fatalf("mkdir current workdir: %v", err)
	}
	targetWorkdir := filepath.Join(currentWorkdir, "nested")
	if err := os.MkdirAll(targetWorkdir, 0o755); err != nil {
		t.Fatalf("mkdir nested workdir: %v", err)
	}

	store := NewStore(t.TempDir(), defaultWorkdir)
	session := NewWithWorkdir("existing", currentWorkdir)
	if err := store.Save(context.Background(), &session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	preparer := NewInputPreparer(store, store)
	result, err := preparer.Prepare(context.Background(), PrepareInput{
		SessionID:        session.ID,
		Text:             "update workdir",
		DefaultWorkdir:   defaultWorkdir,
		RequestedWorkdir: "nested",
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if result.Workdir != targetWorkdir {
		t.Fatalf("expected target workdir %q, got %q", targetWorkdir, result.Workdir)
	}

	loaded, err := store.Load(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Workdir != targetWorkdir {
		t.Fatalf("expected persisted workdir %q, got %q", targetWorkdir, loaded.Workdir)
	}
}
