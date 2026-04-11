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

const (
	dirName    = ".neocode"
	configName = "config.yaml"
)

type Loader struct {
	baseDir  string
	defaults Config
}

type persistedConfig struct {
	SelectedProvider     string                 `yaml:"selected_provider"`
	CurrentModel         string                 `yaml:"current_model"`
	LegacyDefaultWorkdir *string                `yaml:"default_workdir,omitempty"`
	LegacyWorkdir        *string                `yaml:"workdir,omitempty"`
	Shell                string                 `yaml:"shell"`
	MaxLoops             int                    `yaml:"max_loops,omitempty"`
	ToolTimeoutSec       int                    `yaml:"tool_timeout_sec,omitempty"`
	Context              persistedContextConfig `yaml:"context,omitempty"`
	Tools                ToolsConfig            `yaml:"tools,omitempty"`
	Memo                 persistedMemoConfig    `yaml:"memo,omitempty"`
}

type persistedContextConfig struct {
	Compact     persistedCompactConfig     `yaml:"compact,omitempty"`
	AutoCompact persistedAutoCompactConfig `yaml:"auto_compact,omitempty"`
}

type persistedCompactConfig struct {
	ManualStrategy           string `yaml:"manual_strategy,omitempty"`
	ManualKeepRecentMessages int    `yaml:"manual_keep_recent_messages,omitempty"`
	MaxSummaryChars          int    `yaml:"max_summary_chars,omitempty"`
	MicroCompactDisabled     bool   `yaml:"micro_compact_disabled,omitempty"`
}

type persistedAutoCompactConfig struct {
	Enabled             bool `yaml:"enabled"`
	InputTokenThreshold int  `yaml:"input_token_threshold,omitempty"`
}

type persistedMemoConfig struct {
	Enabled       bool `yaml:"enabled,omitempty"`
	AutoExtract   bool `yaml:"auto_extract,omitempty"`
	MaxIndexLines int  `yaml:"max_index_lines,omitempty"`
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

func (l *Loader) DefaultConfig() Config {
	return l.defaults.Clone()
}

func (l *Loader) Load(ctx context.Context) (*Config, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

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

	cfg, err := parseConfigWithContextDefaults(data, l.defaults.Context)
	if err != nil {
		return nil, fmt.Errorf("config: parse config file: %w", err)
	}
	customProviders, err := loadCustomProviders(l.baseDir)
	if err != nil {
		return nil, err
	}
	cfg.Providers = customProviders
	cfg.ApplyDefaultsFrom(l.defaults)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	needsRewrite, err := persistedConfigDiffers(data, *cfg)
	if err != nil {
		return nil, err
	}
	if needsRewrite {
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
	providersSnapshot := cloneProviders(snapshot.Providers)
	snapshot.ApplyDefaultsFrom(l.defaults)
	if len(providersSnapshot) > 0 {
		snapshot.Providers = providersSnapshot
	}
	if err := snapshot.Validate(); err != nil {
		return err
	}

	data, err := marshalPersistedConfig(snapshot)
	if err != nil {
		return err
	}

	if err := os.WriteFile(l.ConfigPath(), data, 0o644); err != nil {
		return fmt.Errorf("config: write config file: %w", err)
	}

	return nil
}

func defaultBaseDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return dirName
	}
	return filepath.Join(home, dirName)
}

func parseConfig(data []byte) (*Config, error) {
	return parseConfigWithContextDefaults(data, Default().Context)
}

// parseConfigWithContextDefaults 负责在解析配置时注入上下文压缩相关默认值。
func parseConfigWithContextDefaults(data []byte, contextDefaults ContextConfig) (*Config, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return &Config{}, nil
	}

	return parseCurrentConfig(data, contextDefaults)
}

func parseCurrentConfig(data []byte, contextDefaults ContextConfig) (*Config, error) {
	var file persistedConfig
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&file); err != nil {
		return nil, err
	}
	cfg := &Config{
		SelectedProvider: strings.TrimSpace(file.SelectedProvider),
		CurrentModel:     strings.TrimSpace(file.CurrentModel),
		Shell:            strings.TrimSpace(file.Shell),
		MaxLoops:         file.MaxLoops,
		ToolTimeoutSec:   file.ToolTimeoutSec,
		Context:          fromPersistedContextConfig(file.Context, contextDefaults),
		Tools:            file.Tools,
		Memo:             fromPersistedMemoConfig(file.Memo),
	}

	return cfg, nil
}

func marshalPersistedConfig(snapshot Config) ([]byte, error) {
	file := persistedConfig{
		SelectedProvider: snapshot.SelectedProvider,
		CurrentModel:     snapshot.CurrentModel,
		Shell:            snapshot.Shell,
		MaxLoops:         snapshot.MaxLoops,
		ToolTimeoutSec:   snapshot.ToolTimeoutSec,
		Context:          newPersistedContextConfig(snapshot.Context),
		Tools:            snapshot.Tools,
		Memo:             newPersistedMemoConfig(snapshot.Memo),
	}

	data, err := yaml.Marshal(&file)
	if err != nil {
		return nil, fmt.Errorf("config: marshal config: %w", err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	return data, nil
}

// newPersistedContextConfig 将运行时上下文配置收敛为 YAML 持久化结构。
func newPersistedContextConfig(cfg ContextConfig) persistedContextConfig {
	return persistedContextConfig{
		Compact: persistedCompactConfig{
			ManualStrategy:           cfg.Compact.ManualStrategy,
			ManualKeepRecentMessages: cfg.Compact.ManualKeepRecentMessages,
			MaxSummaryChars:          cfg.Compact.MaxSummaryChars,
			MicroCompactDisabled:     cfg.Compact.MicroCompactDisabled,
		},
		AutoCompact: persistedAutoCompactConfig{
			Enabled:             cfg.AutoCompact.Enabled,
			InputTokenThreshold: cfg.AutoCompact.InputTokenThreshold,
		},
	}
}

// fromPersistedContextConfig 将持久化配置恢复为运行时上下文配置并补齐默认值。
func fromPersistedContextConfig(file persistedContextConfig, defaults ContextConfig) ContextConfig {
	out := ContextConfig{
		Compact: CompactConfig{
			ManualStrategy:           strings.TrimSpace(file.Compact.ManualStrategy),
			ManualKeepRecentMessages: file.Compact.ManualKeepRecentMessages,
			MaxSummaryChars:          file.Compact.MaxSummaryChars,
			MicroCompactDisabled:     file.Compact.MicroCompactDisabled,
		},
		AutoCompact: AutoCompactConfig{
			Enabled:             file.AutoCompact.Enabled,
			InputTokenThreshold: file.AutoCompact.InputTokenThreshold,
		},
	}
	out.Compact.ApplyDefaults(defaults.Compact)
	out.AutoCompact.ApplyDefaults(defaults.AutoCompact)
	return out
}

func persistedConfigDiffers(data []byte, cfg Config) (bool, error) {
	canonical, err := marshalPersistedConfig(cfg)
	if err != nil {
		return false, err
	}
	return !bytes.Equal(bytes.TrimSpace(data), bytes.TrimSpace(canonical)), nil
}

// newPersistedMemoConfig 将运行时 memo 配置收敛为 YAML 持久化结构。
func newPersistedMemoConfig(cfg MemoConfig) persistedMemoConfig {
	return persistedMemoConfig{
		Enabled:       cfg.Enabled,
		AutoExtract:   cfg.AutoExtract,
		MaxIndexLines: cfg.MaxIndexLines,
	}
}

// fromPersistedMemoConfig 将持久化配置恢复为运行时 memo 配置。
func fromPersistedMemoConfig(file persistedMemoConfig) MemoConfig {
	return MemoConfig{
		Enabled:       file.Enabled,
		AutoExtract:   file.AutoExtract,
		MaxIndexLines: file.MaxIndexLines,
	}
}
