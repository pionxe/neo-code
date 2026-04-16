package types

import (
	"context"
	"io"
)

// SessionAssetReader 定义 provider 请求阶段读取会话附件内容的最小能力。
type SessionAssetReader interface {
	Open(ctx context.Context, assetID string) (io.ReadCloser, string, error)
}

// GenerateRequest 是 provider.Generate() 的请求参数。
type GenerateRequest struct {
	Model              string             `json:"model"`
	SystemPrompt       string             `json:"system_prompt"`
	Messages           []Message          `json:"messages"`
	Tools              []ToolSpec         `json:"tools,omitempty"`
	SessionAssetReader SessionAssetReader `json:"-"`
}

// Usage 记录本次请求的 token 使用统计。
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}
