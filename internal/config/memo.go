package config

import "errors"

const DefaultMemoMaxIndexLines = 200

// MemoConfig 控制跨会话持久记忆的行为配置。
type MemoConfig struct {
	Enabled       bool `yaml:"enabled,omitempty"`
	AutoExtract   bool `yaml:"auto_extract,omitempty"`
	MaxIndexLines int  `yaml:"max_index_lines,omitempty"`
}

// defaultMemoConfig 返回跨会话记忆的默认配置。
func defaultMemoConfig() MemoConfig {
	return MemoConfig{
		Enabled:       true,
		AutoExtract:   true,
		MaxIndexLines: DefaultMemoMaxIndexLines,
	}
}

// Clone 返回 memo 配置的值副本。
func (c MemoConfig) Clone() MemoConfig {
	return c
}

// ApplyDefaults 为 memo 配置补齐缺省参数。
func (c *MemoConfig) ApplyDefaults(defaults MemoConfig) {
	if c == nil {
		return
	}
	if c.MaxIndexLines <= 0 {
		c.MaxIndexLines = defaults.MaxIndexLines
	}
}

// Validate 校验 memo 配置是否合法。
func (c MemoConfig) Validate() error {
	if c.MaxIndexLines < 0 {
		return errors.New("max_index_lines must be non-negative")
	}
	return nil
}
