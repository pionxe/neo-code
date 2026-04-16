package session

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
)

func TestJSONStoreSaveAssetOpenAndStat(t *testing.T) {
	t.Parallel()

	store := NewJSONStore(t.TempDir(), t.TempDir())
	sessionID := "session_asset_round_trip"
	payload := []byte("fake-image-bytes")

	meta, err := store.SaveAsset(context.Background(), sessionID, bytes.NewReader(payload), "image/png")
	if err != nil {
		t.Fatalf("SaveAsset() error = %v", err)
	}
	if meta.ID == "" || meta.MimeType != "image/png" || meta.Size != int64(len(payload)) {
		t.Fatalf("unexpected saved meta: %+v", meta)
	}

	statMeta, err := store.Stat(context.Background(), sessionID, meta.ID)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if statMeta != meta {
		t.Fatalf("expected stat meta %+v, got %+v", meta, statMeta)
	}

	rc, openMeta, err := store.Open(context.Background(), sessionID, meta.ID)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll(open) error = %v", err)
	}
	if string(data) != string(payload) {
		t.Fatalf("unexpected open payload: got %q want %q", string(data), string(payload))
	}
	if openMeta != meta {
		t.Fatalf("expected open meta %+v, got %+v", meta, openMeta)
	}

	assetPath := store.assetPath(sessionID, meta.ID)
	if _, err := os.Stat(assetPath); err != nil {
		t.Fatalf("expected asset file %q exists, err=%v", assetPath, err)
	}
	if _, err := os.Stat(store.assetMetaPath(sessionID, meta.ID)); err != nil {
		t.Fatalf("expected asset meta file exists, err=%v", err)
	}
}

func TestJSONStoreSaveAssetRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	store := NewJSONStore(t.TempDir(), t.TempDir())
	sessionID := "session_asset_invalid"

	if _, err := store.SaveAsset(context.Background(), sessionID, nil, "image/png"); err == nil {
		t.Fatalf("expected nil reader error")
	}
	if _, err := store.SaveAsset(context.Background(), sessionID, strings.NewReader("x"), ""); err == nil {
		t.Fatalf("expected empty mime error")
	}
	if _, err := store.SaveAsset(context.Background(), sessionID, strings.NewReader("x"), "text/plain"); err == nil {
		t.Fatalf("expected unsupported mime error")
	}
	if _, err := store.SaveAsset(context.Background(), "../bad", strings.NewReader("x"), "image/png"); err == nil {
		t.Fatalf("expected invalid session id error")
	}
}

func TestJSONStoreAssetOpenAndStatRejectInvalidID(t *testing.T) {
	t.Parallel()

	store := NewJSONStore(t.TempDir(), t.TempDir())

	if _, _, err := store.Open(context.Background(), "session_ok", "../bad"); err == nil {
		t.Fatalf("expected invalid asset id on open")
	}
	if _, err := store.Stat(context.Background(), "session_ok", "../bad"); err == nil {
		t.Fatalf("expected invalid asset id on stat")
	}
}

func TestJSONStoreAssetStoreRespectsCanceledContext(t *testing.T) {
	t.Parallel()

	store := NewJSONStore(t.TempDir(), t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := store.SaveAsset(ctx, "session_ctx_cancel", strings.NewReader("x"), "image/png"); err == nil {
		t.Fatalf("expected canceled SaveAsset error")
	}
	if _, _, err := store.Open(ctx, "session_ctx_cancel", "asset_x"); err == nil {
		t.Fatalf("expected canceled Open error")
	}
	if _, err := store.Stat(ctx, "session_ctx_cancel", "asset_x"); err == nil {
		t.Fatalf("expected canceled Stat error")
	}
}

func TestJSONStoreSaveAssetRejectsOversizedPayload(t *testing.T) {
	t.Parallel()

	store := NewJSONStore(t.TempDir(), t.TempDir())
	oversized := bytes.NewReader(make([]byte, maxSessionAssetWriteBytes+1))

	if _, err := store.SaveAsset(context.Background(), "session_oversize", oversized, "image/png"); err == nil ||
		!strings.Contains(err.Error(), "asset size exceeds") {
		t.Fatalf("expected oversized payload rejection, got %v", err)
	}
}
