package provider

import "context"

type Provider interface {
	Chat(ctx context.Context, req ChatRequest, events chan<- StreamEvent) error
}
