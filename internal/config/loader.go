package config

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

const (
	dirName    = ".neocode"
	configName = "config.yaml"
	envName    = ".env"
)

type Loader struct {
	baseDir  string
	defaults Config
}

type persistedConfig struct {
	ProviderOverrides []ProviderOverride `yaml:"provider_overrides,omitempty"`
	SelectedProvider  string             `yaml:"selected_provider"`
	CurrentModel      string             `yaml:"current_model"`
	Workdir           string             `yaml:"workdir"`
	Shell             string             `yaml:"shell"`
	MaxLoops          int                `yaml:"max_loops,omitempty"`
	ToolTimeoutSec    int                `yaml:"tool_timeout_sec,omitempty"`
	Tools             ToolsConfig        `yaml:"tools,omitempty"`

	// Legacy read-only field. New saves never emit it.
	Providers []ProviderConfig `yaml:"providers,omitempty"`
}

func NewLoader(baseDir string, defaults *Config) *Loader {
	if defaults == nil {
		panic("config: loader defaults are nil")
	}

	if strings.TrimSpace(baseDir) == "" {
		baseDir = defaultBaseDir()
	}

	snapshot := defaults.Clone()
	snapshot.ApplyDefaultsFrom(*Default())
	if err := snapshot.Validate(); err != nil {
		panic(fmt.Sprintf("config: invalid loader defaults: %v", err))
	}

	return &Loader{
		baseDir:  baseDir,
		defaults: snapshot,
	}
}

func (l *Loader) BaseDir() string {
	return l.baseDir
}

func (l *Loader) ConfigPath() string {
	return filepath.Join(l.baseDir, configName)
}

func (l *Loader) EnvPath() string {
	return filepath.Join(l.baseDir, envName)
}

func (l *Loader) DefaultConfig() Config {
	return l.defaults.Clone()
}

func (l *Loader) Load(ctx context.Context) (*Config, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	l.LoadEnvironment()

	if err := os.MkdirAll(l.baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("config: create config dir: %w", err)
	}
	if _, err := os.Stat(l.ConfigPath()); os.IsNotExist(err) {
		defaultCfg := l.DefaultConfig()
		if err := l.Save(ctx, &defaultCfg); err != nil {
			return nil, err
		}
	}

	data, err := os.ReadFile(l.ConfigPath())
	if err != nil {
		return nil, fmt.Errorf("config: read config file: %w", err)
	}

	cfg, err := parseConfig(data, l.defaults)
	if err != nil {
		return nil, fmt.Errorf("config: parse config file: %w", err)
	}
	cfg.ApplyDefaultsFrom(l.defaults)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if requiresConfigRewrite(data) {
		if err := l.Save(ctx, cfg); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func (l *Loader) Save(ctx context.Context, cfg *Config) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := os.MkdirAll(l.baseDir, 0o755); err != nil {
		return fmt.Errorf("config: create config dir: %w", err)
	}

	snapshot := cfg.Clone()
	snapshot.ApplyDefaultsFrom(l.defaults)
	if err := snapshot.Validate(); err != nil {
		return err
	}

	file := persistedConfig{
		ProviderOverrides: DeriveProviderOverrides(snapshot.Providers, l.defaults.Providers),
		SelectedProvider:  snapshot.SelectedProvider,
		CurrentModel:      snapshot.CurrentModel,
		Workdir:           snapshot.Workdir,
		Shell:             snapshot.Shell,
		MaxLoops:          snapshot.MaxLoops,
		ToolTimeoutSec:    snapshot.ToolTimeoutSec,
		Tools:             snapshot.Tools,
	}

	data, err := yaml.Marshal(&file)
	if err != nil {
		return fmt.Errorf("config: marshal config: %w", err)
	}

	if len(data) == 0 || data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}

	if err := os.WriteFile(l.ConfigPath(), data, 0o644); err != nil {
		return fmt.Errorf("config: write config file: %w", err)
	}

	return nil
}

func (l *Loader) LoadEnvironment() {
	_ = godotenv.Load()
	_ = godotenv.Load(l.EnvPath())
}

func defaultBaseDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return dirName
	}
	return filepath.Join(home, dirName)
}

func parseConfig(data []byte, defaults Config) (*Config, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return &Config{}, nil
	}

	cfg, currentErr := parseCurrentConfig(data, defaults)
	if currentErr == nil {
		return cfg, nil
	}

	legacy, legacyErr := parseLegacyConfig(data, defaults)
	if legacyErr == nil {
		return legacy, nil
	}

	return nil, currentErr
}

type aliasConfig struct {
	MaxLoop       int    `yaml:"max_loop"`
	WorkspaceRoot string `yaml:"workspace_root"`
}

type legacyConfig struct {
	SelectedProvider string                          `yaml:"selected_provider"`
	CurrentModel     string                          `yaml:"current_model"`
	MaxLoop          int                             `yaml:"max_loop"`
	ToolTimeoutSec   int                             `yaml:"tool_timeout_sec"`
	WorkspaceRoot    string                          `yaml:"workspace_root"`
	Shell            string                          `yaml:"shell"`
	Providers        map[string]legacyProviderConfig `yaml:"providers"`
}

type legacyProviderConfig struct {
	Type      string   `yaml:"type"`
	BaseURL   string   `yaml:"base_url"`
	APIKeyEnv string   `yaml:"api_key_env"`
	Models    []string `yaml:"models"`
}

func parseCurrentConfig(data []byte, defaults Config) (*Config, error) {
	var file persistedConfig
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, err
	}

	var aliases aliasConfig
	if err := yaml.Unmarshal(data, &aliases); err == nil {
		if file.MaxLoops == 0 && aliases.MaxLoop > 0 {
			file.MaxLoops = aliases.MaxLoop
		}
		if strings.TrimSpace(file.Workdir) == "" && strings.TrimSpace(aliases.WorkspaceRoot) != "" {
			file.Workdir = aliases.WorkspaceRoot
		}
	}

	cfg := &Config{
		SelectedProvider: strings.TrimSpace(file.SelectedProvider),
		CurrentModel:     strings.TrimSpace(file.CurrentModel),
		Workdir:          strings.TrimSpace(file.Workdir),
		Shell:            strings.TrimSpace(file.Shell),
		MaxLoops:         file.MaxLoops,
		ToolTimeoutSec:   file.ToolTimeoutSec,
		Tools:            file.Tools,
	}

	switch {
	case len(file.ProviderOverrides) > 0:
		cfg.Providers = MergeProviderOverrides(defaults.Providers, file.ProviderOverrides)
	case len(file.Providers) > 0:
		cfg.Providers = MergeProviderOverrides(defaults.Providers, DeriveProviderOverrides(file.Providers, defaults.Providers))
	}

	return cfg, nil
}

func parseLegacyConfig(data []byte, defaults Config) (*Config, error) {
	var legacy legacyConfig
	if err := yaml.Unmarshal(data, &legacy); err != nil {
		return nil, err
	}

	return convertLegacyConfig(legacy, defaults), nil
}

func convertLegacyConfig(in legacyConfig, defaults Config) *Config {
	out := &Config{
		SelectedProvider: strings.TrimSpace(in.SelectedProvider),
		CurrentModel:     strings.TrimSpace(in.CurrentModel),
		Workdir:          strings.TrimSpace(in.WorkspaceRoot),
		Shell:            strings.TrimSpace(in.Shell),
		MaxLoops:         in.MaxLoop,
		ToolTimeoutSec:   in.ToolTimeoutSec,
	}

	for name, provider := range in.Providers {
		model := firstNonEmpty(provider.Models...)
		if strings.EqualFold(name, in.SelectedProvider) && strings.TrimSpace(in.CurrentModel) != "" {
			model = strings.TrimSpace(in.CurrentModel)
		}

		out.Providers = append(out.Providers, ProviderConfig{
			Name:      strings.TrimSpace(name),
			Driver:    strings.TrimSpace(provider.Type),
			BaseURL:   strings.TrimSpace(provider.BaseURL),
			Model:     strings.TrimSpace(model),
			APIKeyEnv: strings.TrimSpace(provider.APIKeyEnv),
		})
	}

	out.Providers = MergeProviderOverrides(defaults.Providers, DeriveProviderOverrides(out.Providers, defaults.Providers))
	return out
}

func firstNonEmpty(items ...string) string {
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			return strings.TrimSpace(item)
		}
	}
	return ""
}

func requiresConfigRewrite(data []byte) bool {
	text := strings.TrimSpace(string(data))
	switch {
	case text == "":
		return false
	case strings.Contains(text, "provider_overrides:"):
		return strings.Contains(text, "workspace_root:") || strings.Contains(text, "max_loop:")
	case strings.Contains(text, "\nproviders:") || strings.HasPrefix(text, "providers:"):
		return true
	case strings.Contains(text, "workspace_root:") || strings.Contains(text, "max_loop:"):
		return true
	default:
		return false
	}
}
