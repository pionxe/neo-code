package session

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// AssetMeta 描述会话附件最小元数据，用于 provider 请求阶段定位与发送。
type AssetMeta struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
}

// AssetStore 定义会话级附件读写契约。
type AssetStore interface {
	SaveAsset(ctx context.Context, sessionID string, r io.Reader, mimeType string) (AssetMeta, error)
	Open(ctx context.Context, sessionID string, assetID string) (io.ReadCloser, AssetMeta, error)
	Stat(ctx context.Context, sessionID string, assetID string) (AssetMeta, error)
}

type storedAssetMeta struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
}

// newAssetMeta 生成新的会话附件元数据，并校验 MIME 约束。
func newAssetMeta(mimeType string) (AssetMeta, error) {
	normalized := strings.ToLower(strings.TrimSpace(mimeType))
	if normalized == "" {
		return AssetMeta{}, fmt.Errorf("session: asset mime type is empty")
	}
	if !strings.HasPrefix(normalized, "image/") {
		return AssetMeta{}, fmt.Errorf("session: unsupported asset mime type %q", mimeType)
	}
	return AssetMeta{
		ID:       NewID("asset"),
		MimeType: normalized,
	}, nil
}

// decodeStoredAssetMeta 将磁盘上的附件元数据反序列化为运行时结构。
func decodeStoredAssetMeta(data []byte) (AssetMeta, error) {
	var stored storedAssetMeta
	if err := json.Unmarshal(data, &stored); err != nil {
		return AssetMeta{}, fmt.Errorf("session: decode asset meta: %w", err)
	}
	if err := validateStorageID("asset id", stored.ID); err != nil {
		return AssetMeta{}, fmt.Errorf("session: %w", err)
	}
	normalizedMime := strings.ToLower(strings.TrimSpace(stored.MimeType))
	if normalizedMime == "" {
		return AssetMeta{}, fmt.Errorf("session: asset meta mime_type is empty")
	}
	return AssetMeta{
		ID:       stored.ID,
		MimeType: normalizedMime,
		Size:     stored.Size,
	}, nil
}

// encodeStoredAssetMeta 将运行时附件元数据编码为可持久化 JSON。
func encodeStoredAssetMeta(meta AssetMeta) ([]byte, error) {
	if err := validateStorageID("asset id", meta.ID); err != nil {
		return nil, fmt.Errorf("session: %w", err)
	}
	normalizedMime := strings.ToLower(strings.TrimSpace(meta.MimeType))
	if normalizedMime == "" {
		return nil, fmt.Errorf("session: asset mime type is empty")
	}
	payload, err := json.MarshalIndent(storedAssetMeta{
		ID:       meta.ID,
		MimeType: normalizedMime,
		Size:     meta.Size,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("session: marshal asset meta: %w", err)
	}
	return append(payload, '\n'), nil
}
