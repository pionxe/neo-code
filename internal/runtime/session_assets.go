package runtime

import (
	"context"
	"fmt"
	"io"
	"strings"

	providertypes "neo-code/internal/provider/types"
	agentsession "neo-code/internal/session"
)

// buildSessionAssetReader 按当前会话构造 provider 可用的附件读取器；缺失依赖时返回 nil。
func (s *Service) buildSessionAssetReader(ctx context.Context, sessionID string) providertypes.SessionAssetReader {
	if s.sessionAssetStore == nil {
		return nil
	}
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return nil
	}
	return runtimeSessionAssetReader{
		ctx:       ctx,
		store:     s.sessionAssetStore,
		sessionID: id,
	}
}

type runtimeSessionAssetReader struct {
	ctx       context.Context
	store     agentsession.AssetStore
	sessionID string
}

// Open 在会话范围内打开指定附件，供 provider 请求阶段读取并转换协议。
func (r runtimeSessionAssetReader) Open(ctx context.Context, assetID string) (io.ReadCloser, string, error) {
	openCtx := ctx
	if openCtx == nil {
		openCtx = r.ctx
	}
	if openCtx == nil {
		openCtx = context.Background()
	}
	rc, meta, err := r.store.Open(openCtx, r.sessionID, strings.TrimSpace(assetID))
	if err != nil {
		return nil, "", fmt.Errorf("runtime: open session asset %q: %w", assetID, err)
	}
	return rc, meta.MimeType, nil
}
