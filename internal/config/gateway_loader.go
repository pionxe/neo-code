package config

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadGatewayConfig 轻量读取 config.yaml 中 gateway 段并补齐默认值，不触发 provider 级校验。
func LoadGatewayConfig(ctx context.Context, baseDir string) (GatewayConfig, error) {
	if err := ctx.Err(); err != nil {
		return GatewayConfig{}, err
	}

	resolvedBaseDir := strings.TrimSpace(baseDir)
	if resolvedBaseDir == "" {
		resolvedBaseDir = defaultBaseDir()
	}
	configPath := filepath.Join(resolvedBaseDir, configName)
	defaults := defaultGatewayConfig()

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return defaults, nil
		}
		return GatewayConfig{}, fmt.Errorf("config: read gateway config file: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return defaults, nil
	}

	var file struct {
		Gateway GatewayConfig  `yaml:"gateway,omitempty"`
		Extra   map[string]any `yaml:",inline"`
	}
	file.Gateway = defaults.Clone()
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&file); err != nil {
		return GatewayConfig{}, fmt.Errorf("config: parse gateway config file: %w", err)
	}

	gatewayConfig := file.Gateway
	if err := gatewayConfig.Validate(); err != nil {
		return GatewayConfig{}, fmt.Errorf("config: gateway: %w", err)
	}
	return gatewayConfig, nil
}
