package shared

import (
	"errors"
	"net/http"
	"strings"

	"neo-code/internal/provider"
)

// ErrorPrefix 统一收敛 OpenAI 兼容 provider 的错误前缀，避免历史命名残留继续扩散。
const ErrorPrefix = "openaicompat provider: "

func ValidateRuntimeConfig(cfg provider.RuntimeConfig) error {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return errors.New(ErrorPrefix + "base url is empty")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return errors.New(ErrorPrefix + "api key is empty")
	}
	return nil
}

func Endpoint(baseURL string, path string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if path == "" {
		return baseURL
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return baseURL + path
}

func SetBearerAuthorization(header http.Header, apiKey string) {
	if header == nil {
		return
	}

	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return
	}

	header.Set("Authorization", "Bearer "+apiKey)
}
