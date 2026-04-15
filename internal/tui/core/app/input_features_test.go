package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"neo-code/internal/config"
	configstate "neo-code/internal/config/state"
	providertypes "neo-code/internal/provider/types"
)

func TestTokenAndReferenceParsing(t *testing.T) {
	start, end, token, ok := tokenRange("  @file/path", tokenSelectorFirst)
	if !ok || start != 2 || end != len("  @file/path") || token != "@file/path" {
		t.Fatalf("unexpected first token parse: start=%d end=%d token=%q ok=%v", start, end, token, ok)
	}

	start, end, token, ok = tokenRange("hello @image:/tmp/a.png", tokenSelectorLast)
	if !ok || token != "@image:/tmp/a.png" || start <= 0 || end <= start {
		t.Fatalf("unexpected last token parse: start=%d end=%d token=%q ok=%v", start, end, token, ok)
	}

	_, _, _, ok = currentReferenceToken("hello world")
	if ok {
		t.Fatalf("expected non-reference token to be rejected")
	}

	_, _, token, ok = currentReferenceToken("x @a/b.txt")
	if !ok || token != "@a/b.txt" {
		t.Fatalf("expected file reference token, got token=%q ok=%v", token, ok)
	}

	if !isImageReferenceInput("@image:/tmp/p.png") {
		t.Fatalf("expected image reference input recognized")
	}
	if got := extractImageReference("@image:/tmp/p.png"); got != "/tmp/p.png" {
		t.Fatalf("unexpected image reference extraction: %q", got)
	}
}

func TestApplyFileReference(t *testing.T) {
	app, _ := newTestApp(t)
	root := t.TempDir()
	app.state.CurrentWorkdir = root
	inside := filepath.Join(root, "docs", "a.md")
	if err := os.MkdirAll(filepath.Dir(inside), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(inside, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := app.applyFileReference(inside); err != nil {
		t.Fatalf("applyFileReference() error = %v", err)
	}
	if got := app.input.Value(); !strings.Contains(got, "@docs/a.md") {
		t.Fatalf("expected relative file reference, got %q", got)
	}

	app.input.SetValue("prefix @old/ref")
	if err := app.applyFileReference(inside); err != nil {
		t.Fatalf("applyFileReference replace() error = %v", err)
	}
	if got := app.input.Value(); strings.Contains(got, "@old/ref") || !strings.Contains(got, "@docs/a.md") {
		t.Fatalf("expected active token replaced, got %q", got)
	}
}

func TestImageAttachmentLifecycle(t *testing.T) {
	app, _ := newTestApp(t)
	root := t.TempDir()
	imagePath := filepath.Join(root, "test.png")
	if err := os.WriteFile(imagePath, []byte("fake-png"), 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	if err := app.addImageAttachment(imagePath); err != nil {
		t.Fatalf("addImageAttachment() error = %v", err)
	}
	if app.getImageAttachmentCount() != 1 {
		t.Fatalf("expected one attachment, got %d", app.getImageAttachmentCount())
	}
	if !app.hasImageAttachments() {
		t.Fatalf("expected hasImageAttachments() true")
	}
	if _, err := app.loadImageAttachmentData(0); err != nil {
		t.Fatalf("loadImageAttachmentData() error = %v", err)
	}

	if err := app.removeImageAttachment(0); err != nil {
		t.Fatalf("removeImageAttachment() error = %v", err)
	}
	if app.getImageAttachmentCount() != 0 {
		t.Fatalf("expected no attachments after remove")
	}
	if err := app.removeImageAttachment(0); err == nil {
		t.Fatalf("expected out-of-range remove error")
	}
}

func TestAddImageAttachmentLimit(t *testing.T) {
	app, _ := newTestApp(t)
	root := t.TempDir()

	for i := 0; i < maxImageAttachments; i++ {
		path := filepath.Join(root, fmt.Sprintf("%d.png", i))
		if err := os.WriteFile(path, []byte("png"), 0o644); err != nil {
			t.Fatalf("write image: %v", err)
		}
		if err := app.addImageAttachment(path); err != nil {
			t.Fatalf("addImageAttachment(%d) error = %v", i, err)
		}
	}

	over := filepath.Join(root, "over.png")
	if err := os.WriteFile(over, []byte("png"), 0o644); err != nil {
		t.Fatalf("write over image: %v", err)
	}
	if err := app.addImageAttachment(over); err == nil {
		t.Fatalf("expected attachment limit error")
	}
}

func TestCanSendImageInputCacheInvalidationOnModelChange(t *testing.T) {
	app, _ := newTestApp(t)
	providerID := app.state.CurrentProvider

	app.providerSvc = stubProviderService{
		providers: []configstate.ProviderOption{{ID: providerID, Name: providerID}},
		models: []providertypes.ModelDescriptor{{
			ID:   "model-a",
			Name: "model-a",
			CapabilityHints: providertypes.ModelCapabilityHints{
				ImageInput: providertypes.ModelCapabilityStateSupported,
			},
		}},
	}
	app.state.CurrentModel = "model-a"
	if !app.canSendImageInput() {
		t.Fatalf("expected model-a to support images")
	}

	app.providerSvc = stubProviderService{
		providers: []configstate.ProviderOption{{ID: providerID, Name: providerID}},
		models: []providertypes.ModelDescriptor{{
			ID:   "model-b",
			Name: "model-b",
			CapabilityHints: providertypes.ModelCapabilityHints{
				ImageInput: providertypes.ModelCapabilityStateUnsupported,
			},
		}},
	}
	app.syncConfigState(config.Config{SelectedProvider: providerID, CurrentModel: "model-b", Workdir: app.state.CurrentWorkdir})
	if app.canSendImageInput() {
		t.Fatalf("expected model-b to be unsupported after cache invalidation")
	}
}

func TestComposeMessageWithImageAttachments(t *testing.T) {
	app, _ := newTestApp(t)
	app.pendingImageAttachments = []pendingImageAttachment{{
		Path:     "/tmp/a.png",
		Name:     "a.png",
		MimeType: "image/png",
		Size:     12,
	}}

	got := app.composeMessageWithImageAttachments("hello")
	if !strings.Contains(got, "[Attached images]") || !strings.Contains(got, "a.png") || !strings.Contains(got, "path=/tmp/a.png") {
		t.Fatalf("unexpected composed message: %q", got)
	}
}

func TestUpdateSendWithImageAttachmentsComposesRuntimeInput(t *testing.T) {
	app, runtime := newTestApp(t)
	root := t.TempDir()
	imagePath := filepath.Join(root, "queued.png")
	if err := os.WriteFile(imagePath, []byte("fake-png"), 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}
	if err := app.addImageAttachment(imagePath); err != nil {
		t.Fatalf("addImageAttachment() error = %v", err)
	}
	app.providerSvc = stubProviderService{
		providers: []configstate.ProviderOption{{ID: app.state.CurrentProvider, Name: app.state.CurrentProvider}},
		models: []providertypes.ModelDescriptor{{
			ID:   app.state.CurrentModel,
			Name: app.state.CurrentModel,
			CapabilityHints: providertypes.ModelCapabilityHints{
				ImageInput: providertypes.ModelCapabilityStateSupported,
			},
		}},
	}

	app.input.SetValue("hello")
	app.state.InputText = "hello"

	model, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected run command")
	}
	app = model.(App)
	if app.hasImageAttachments() {
		t.Fatalf("expected attachments cleared after send")
	}
	if len(app.activeMessages) == 0 || !strings.Contains(app.activeMessages[len(app.activeMessages)-1].Content, "[Attached images]") {
		t.Fatalf("expected composed user message in transcript")
	}

	msg := cmd()
	_, ok := msg.(runFinishedMsg)
	if !ok {
		t.Fatalf("expected runFinishedMsg, got %T", msg)
	}
	if len(runtime.runInputs) != 1 || !strings.Contains(runtime.runInputs[0].Content, "[Attached images]") {
		t.Fatalf("expected composed runtime input, got %+v", runtime.runInputs)
	}
}
