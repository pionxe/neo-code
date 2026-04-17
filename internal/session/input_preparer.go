package session

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	providertypes "neo-code/internal/provider/types"
)

const imageOnlySessionTitle = "Image Message"

// PrepareImageInput 表示一次用户输入中附带的本地图片引用。
type PrepareImageInput struct {
	Path     string
	MimeType string
}

// PrepareInput 定义会话输入归一化的领域输入参数。
type PrepareInput struct {
	SessionID        string
	Text             string
	Images           []PrepareImageInput
	DefaultWorkdir   string
	RequestedWorkdir string
}

// PreparedInput 表示归一化完成后可直接进入 runtime 的标准输入结果。
type PreparedInput struct {
	SessionID   string
	Workdir     string
	Parts       []providertypes.ContentPart
	SavedAssets []AssetMeta
}

// AssetSaveError 描述图片落盘阶段的结构化失败信息，便于上层统一事件化处理。
type AssetSaveError struct {
	Index int
	Path  string
	Err   error
}

func (e *AssetSaveError) Error() string {
	if e == nil {
		return "session: asset save failed"
	}
	if strings.TrimSpace(e.Path) == "" {
		return fmt.Sprintf("session: save asset at index %d: %v", e.Index, e.Err)
	}
	return fmt.Sprintf("session: save asset %q at index %d: %v", e.Path, e.Index, e.Err)
}

func (e *AssetSaveError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// InputPreparer 负责把用户文本/图片输入归一化为会话级标准 parts。
type InputPreparer struct {
	store      Store
	assetStore AssetStore
}

// NewInputPreparer 创建会话输入归一化组件。
func NewInputPreparer(store Store, assetStore AssetStore) *InputPreparer {
	return &InputPreparer{
		store:      store,
		assetStore: assetStore,
	}
}

// Prepare 负责会话解析/创建、附件落盘与 parts 组装。
func (p *InputPreparer) Prepare(ctx context.Context, input PrepareInput) (PreparedInput, error) {
	if err := ctx.Err(); err != nil {
		return PreparedInput{}, err
	}
	if p == nil || p.store == nil {
		return PreparedInput{}, fmt.Errorf("session: input preparer store is not configured")
	}
	if len(input.Images) > 0 && p.assetStore == nil {
		return PreparedInput{}, fmt.Errorf("session: asset store is not configured")
	}

	trimmedText := strings.TrimSpace(input.Text)
	if trimmedText == "" && len(input.Images) == 0 {
		return PreparedInput{}, fmt.Errorf("session: input content is empty")
	}

	sessionTitle := buildSessionTitle(trimmedText, len(input.Images) > 0)
	session, sessionCreated, err := p.loadOrCreateSession(
		ctx,
		input.SessionID,
		sessionTitle,
		input.DefaultWorkdir,
		input.RequestedWorkdir,
	)
	if err != nil {
		return PreparedInput{}, err
	}

	parts := make([]providertypes.ContentPart, 0, 1+len(input.Images))
	if trimmedText != "" {
		parts = append(parts, providertypes.NewTextPart(trimmedText))
	}

	savedAssets := make([]AssetMeta, 0, len(input.Images))
	for index, image := range input.Images {
		path := strings.TrimSpace(image.Path)
		if path == "" {
			p.rollbackCreatedSession(ctx, session.ID, sessionCreated)
			return PreparedInput{}, &AssetSaveError{
				Index: index,
				Path:  path,
				Err:   fmt.Errorf("image path is empty"),
			}
		}
		mimeType := strings.TrimSpace(image.MimeType)

		meta, err := p.saveImageAsset(ctx, session.ID, path, mimeType)
		if err != nil {
			p.rollbackCreatedSession(ctx, session.ID, sessionCreated)
			return PreparedInput{}, &AssetSaveError{
				Index: index,
				Path:  path,
				Err:   err,
			}
		}
		savedAssets = append(savedAssets, meta)
		parts = append(parts, providertypes.NewSessionAssetImagePart(meta.ID, meta.MimeType))
	}

	if err := providertypes.ValidateParts(parts); err != nil {
		p.rollbackCreatedSession(ctx, session.ID, sessionCreated)
		return PreparedInput{}, fmt.Errorf("session: normalize parts: %w", err)
	}

	return PreparedInput{
		SessionID:   session.ID,
		Workdir:     session.Workdir,
		Parts:       parts,
		SavedAssets: savedAssets,
	}, nil
}

func (p *InputPreparer) saveImageAsset(ctx context.Context, sessionID string, path string, mimeType string) (AssetMeta, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return AssetMeta{}, fmt.Errorf("resolve image path: %w", err)
	}

	file, err := os.Open(absolutePath)
	if err != nil {
		return AssetMeta{}, fmt.Errorf("open image file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	resolvedMimeType, err := resolveImageMimeType(path, mimeType, file)
	if err != nil {
		return AssetMeta{}, err
	}

	meta, err := p.assetStore.SaveAsset(ctx, sessionID, file, resolvedMimeType)
	if err != nil {
		return AssetMeta{}, err
	}
	return meta, nil
}

// resolveImageMimeType 解析图片 MIME 类型，优先使用显式传入值，其次回退到扩展名与文件头探测。
func resolveImageMimeType(path string, declared string, file *os.File) (string, error) {
	if normalized := strings.ToLower(strings.TrimSpace(declared)); normalized != "" {
		return normalized, nil
	}

	extMime := strings.ToLower(strings.TrimSpace(mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))))
	if extMime != "" {
		if idx := strings.Index(extMime, ";"); idx >= 0 {
			extMime = strings.TrimSpace(extMime[:idx])
		}
		if strings.HasPrefix(extMime, "image/") {
			return extMime, nil
		}
	}

	buffer := make([]byte, 512)
	n, readErr := file.Read(buffer)
	if readErr != nil && readErr != io.EOF {
		return "", fmt.Errorf("detect image mime type: %w", readErr)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("reset image reader: %w", err)
	}

	detected := strings.ToLower(strings.TrimSpace(http.DetectContentType(buffer[:n])))
	if strings.HasPrefix(detected, "image/") {
		return detected, nil
	}
	return "", fmt.Errorf("unsupported image format")
}

func (p *InputPreparer) loadOrCreateSession(
	ctx context.Context,
	sessionID string,
	title string,
	defaultWorkdir string,
	requestedWorkdir string,
) (Session, bool, error) {
	if strings.TrimSpace(sessionID) == "" {
		sessionWorkdir, err := resolveWorkdirForInput(defaultWorkdir, "", requestedWorkdir)
		if err != nil {
			return Session{}, false, err
		}
		session := NewWithWorkdir(title, sessionWorkdir)
		if err := p.store.Save(ctx, &session); err != nil {
			return Session{}, false, err
		}
		return session, true, nil
	}

	session, err := p.store.Load(ctx, sessionID)
	if err != nil {
		return Session{}, false, err
	}
	if strings.TrimSpace(requestedWorkdir) == "" && strings.TrimSpace(session.Workdir) != "" {
		return session, false, nil
	}

	resolved, err := resolveWorkdirForInput(defaultWorkdir, session.Workdir, requestedWorkdir)
	if err != nil {
		return Session{}, false, err
	}
	if session.Workdir == resolved {
		return session, false, nil
	}

	session.Workdir = resolved
	session.UpdatedAt = time.Now()
	if err := p.store.Save(ctx, &session); err != nil {
		return Session{}, false, err
	}
	return session, false, nil
}

// rollbackCreatedSession 在本次 Prepare 新建会话后发生错误时回滚会话目录，避免残留孤儿会话。
func (p *InputPreparer) rollbackCreatedSession(ctx context.Context, sessionID string, created bool) {
	if !created {
		return
	}
	store, ok := p.store.(*JSONStore)
	if !ok {
		return
	}
	if err := ctx.Err(); err != nil {
		return
	}
	target := store.sessionDir(sessionID)
	if err := ensurePathWithinBase(store.baseDir, target); err != nil {
		return
	}
	_ = os.RemoveAll(target)
}

func resolveWorkdirForInput(defaultWorkdir string, currentWorkdir string, requestedWorkdir string) (string, error) {
	base := EffectiveWorkdir(currentWorkdir, defaultWorkdir)
	if strings.TrimSpace(requestedWorkdir) == "" {
		return ResolveExistingDir(base)
	}

	target := strings.TrimSpace(requestedWorkdir)
	if !filepath.IsAbs(target) {
		target = filepath.Join(base, target)
	}
	return ResolveExistingDir(target)
}

func buildSessionTitle(text string, hasImages bool) string {
	if strings.TrimSpace(text) != "" {
		return strings.TrimSpace(text)
	}
	if hasImages {
		return imageOnlySessionTitle
	}
	return "New Session"
}
