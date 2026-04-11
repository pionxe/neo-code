package openaicompat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"neo-code/internal/provider/openaicompat/shared"
)

// openAIModelsResponse 表示 /models 端点的响应结构。
type openAIModelsResponse struct {
	Data []map[string]any `json:"data"`
}

// fetchModels 从 OpenAI 兼容的 /models 端点获取原始模型列表。
func (p *Provider) fetchModels(ctx context.Context) ([]map[string]any, error) {
	endpoint := shared.Endpoint(p.cfg.BaseURL, "/models")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("%sbuild models request: %w", shared.ErrorPrefix, err)
	}
	req.Header.Set("Accept", "application/json")
	shared.SetBearerAuthorization(req.Header, p.cfg.APIKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%ssend models request: %w", shared.ErrorPrefix, err)
	}
	defer func(body io.ReadCloser) {
		_ = body.Close()
	}(resp.Body)

	if resp.StatusCode >= http.StatusBadRequest {
		data, _ := io.ReadAll(resp.Body)
		body := strings.TrimSpace(string(data))
		if body == "" {
			body = resp.Status
		}
		return nil, fmt.Errorf("%smodels endpoint %s", shared.ErrorPrefix, body)
	}

	var payload openAIModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("%sdecode models response: %w", shared.ErrorPrefix, err)
	}
	return payload.Data, nil
}
