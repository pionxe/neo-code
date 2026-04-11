package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
	"neo-code/internal/provider"
)

const (
	providersDirName         = "providers"
	customProviderConfigName = "provider.yaml"
)

type customProviderFile struct {
	Name             string                      `yaml:"name"`
	Driver           string                      `yaml:"driver"`
	APIKeyEnv        string                      `yaml:"api_key_env"`
	BaseURL          string                      `yaml:"base_url,omitempty"`
	OpenAICompatible customOpenAICompatibleFile  `yaml:"openai_compatible,omitempty"`
	Gemini           customGeminiProviderFile    `yaml:"gemini,omitempty"`
	Anthropic        customAnthropicProviderFile `yaml:"anthropic,omitempty"`
}

type customOpenAICompatibleFile struct {
	BaseURL  string `yaml:"base_url"`
	APIStyle string `yaml:"api_style,omitempty"`
}

type customGeminiProviderFile struct {
	BaseURL        string `yaml:"base_url,omitempty"`
	DeploymentMode string `yaml:"deployment_mode,omitempty"`
}

type customAnthropicProviderFile struct {
	BaseURL    string `yaml:"base_url,omitempty"`
	APIVersion string `yaml:"api_version,omitempty"`
}

type customProviderSettings struct {
	BaseURL        string
	APIStyle       string
	DeploymentMode string
	APIVersion     string
}

// loadCustomProviders 扫描 baseDir/providers 下的一层子目录，并将其中的 provider.yaml 解析为运行时配置。
func loadCustomProviders(baseDir string) ([]ProviderConfig, error) {
	providersDir := filepath.Join(strings.TrimSpace(baseDir), providersDirName)
	entries, err := os.ReadDir(providersDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("config: read providers dir: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	providers := make([]ProviderConfig, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		providerDir := filepath.Join(providersDir, entry.Name())
		if _, err := os.Stat(filepath.Join(providerDir, customProviderConfigName)); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("config: stat %s: %w", filepath.Join(providerDir, customProviderConfigName), err)
		}
		providerCfg, err := loadCustomProvider(providerDir)
		if err != nil {
			return nil, err
		}
		providers = append(providers, providerCfg)
	}

	return providers, nil
}

// loadCustomProvider 读取单个 provider 目录，并将 provider.yaml 转为 ProviderConfig。
func loadCustomProvider(providerDir string) (ProviderConfig, error) {
	providerPath := filepath.Join(providerDir, customProviderConfigName)
	data, err := os.ReadFile(providerPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ProviderConfig{}, fmt.Errorf("config: custom provider %q missing %s", filepath.Base(providerDir), customProviderConfigName)
		}
		return ProviderConfig{}, fmt.Errorf("config: read %s: %w", providerPath, err)
	}

	var file customProviderFile
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&file); err != nil {
		return ProviderConfig{}, fmt.Errorf("config: parse %s: %w", providerPath, err)
	}

	settings := resolveCustomProviderSettings(file)
	cfg := ProviderConfig{
		Name:           strings.TrimSpace(file.Name),
		Driver:         strings.TrimSpace(file.Driver),
		BaseURL:        settings.BaseURL,
		APIKeyEnv:      strings.TrimSpace(file.APIKeyEnv),
		APIStyle:       settings.APIStyle,
		DeploymentMode: settings.DeploymentMode,
		APIVersion:     settings.APIVersion,
		Source:         ProviderSourceCustom,
	}

	if normalizeProviderDriver(cfg.Driver) == provider.DriverOpenAICompat && strings.TrimSpace(cfg.APIStyle) == "" {
		cfg.APIStyle = provider.OpenAICompatibleAPIStyleChatCompletions
	}

	if err := cfg.Validate(); err != nil {
		return ProviderConfig{}, fmt.Errorf("config: custom provider %q: %w", filepath.Base(providerDir), err)
	}

	return cfg, nil
}

// resolveCustomProviderSettings 根据 driver 只提取当前协议真正生效的配置字段，避免误吃其他协议块的值。
// 已知 driver 仅从协议块读取 base_url；未知 driver 使用顶层 base_url 作为唯一入口。
func resolveCustomProviderSettings(file customProviderFile) customProviderSettings {
	settings := customProviderSettings{}

	switch normalizeProviderDriver(file.Driver) {
	case provider.DriverOpenAICompat:
		settings.BaseURL = strings.TrimSpace(file.OpenAICompatible.BaseURL)
		settings.APIStyle = strings.TrimSpace(file.OpenAICompatible.APIStyle)
	case provider.DriverGemini:
		settings.BaseURL = strings.TrimSpace(file.Gemini.BaseURL)
		settings.DeploymentMode = strings.TrimSpace(file.Gemini.DeploymentMode)
	case provider.DriverAnthropic:
		settings.BaseURL = strings.TrimSpace(file.Anthropic.BaseURL)
		settings.APIVersion = strings.TrimSpace(file.Anthropic.APIVersion)
	default:
		settings.BaseURL = strings.TrimSpace(file.BaseURL)
	}

	return settings
}
