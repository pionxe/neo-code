package provider

import (
	"context"

	"neo-code/internal/provider/types"
)

// Provider 定义模型生成能力，通过 channel 推送流式事件给上层消费。
type Provider interface {
	Generate(ctx context.Context, req types.GenerateRequest, events chan<- types.StreamEvent) error
}
