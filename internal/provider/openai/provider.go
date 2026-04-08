package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"neo-code/internal/config"
	providertypes "neo-code/internal/provider/types"
)

// Provider 封装 OpenAI 兼容 API 的客户端配置和 HTTP 连接。
type Provider struct {
	cfg    config.ResolvedProviderConfig
	client *http.Client
}

// buildOptions 控制构造行为，用于注入自定义 Transport 等选项。
type buildOptions struct {
	transport http.RoundTripper
}

// buildOption 是 New() 的函数式配置选项。
type buildOption func(*buildOptions)

// withTransport 注入自定义 HTTP Transport（如 RetryTransport）。
func withTransport(rt http.RoundTripper) buildOption {
	return func(o *buildOptions) {
		o.transport = rt
	}
}

// New 创建 OpenAI provider 实例。cfg 必须 Validate 通过且包含有效 API Key。
func New(cfg config.ResolvedProviderConfig, opts ...buildOption) (*Provider, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("openai provider: %w", err)
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("openai provider: api key is empty")
	}

	o := &buildOptions{
		transport: http.DefaultTransport,
	}
	for _, apply := range opts {
		apply(o)
	}

	return &Provider{
		cfg: cfg,
		client: &http.Client{
			Timeout:   90 * time.Second,
			Transport: o.transport,
		},
	}, nil
}

// DiscoverModels 通过 /models 端点查询可用模型列表。
func (p *Provider) DiscoverModels(ctx context.Context) ([]config.ModelDescriptor, error) {
	rawModels, err := p.fetchModels(ctx)
	if err != nil {
		return nil, err
	}

	descriptors := make([]config.ModelDescriptor, 0, len(rawModels))
	for _, raw := range rawModels {
		descriptor, ok := config.DescriptorFromRawModel(raw)
		if !ok {
			continue
		}
		descriptors = append(descriptors, descriptor)
	}
	return config.MergeModelDescriptors(descriptors), nil
}

// Chat 发起 SSE 流式对话请求。
// 流中途断连或协议错误时直接返回错误，由上层调用方决定重试策略。
func (p *Provider) Generate(ctx context.Context, req providertypes.GenerateRequest, events chan<- providertypes.StreamEvent) error {
	payload, err := p.buildRequest(req)
	if err != nil {
		return err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("openai provider: marshal request: %w", err)
	}

	endpoint := strings.TrimRight(p.cfg.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("openai provider: build request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("openai provider: send request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Printf("openai provider: close response body: %v", err)
		}
	}(resp.Body)

	if resp.StatusCode >= http.StatusBadRequest {
		return p.parseError(resp)
	}

	return p.consumeStream(ctx, resp.Body, events)
}
