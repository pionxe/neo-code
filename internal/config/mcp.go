package config

import (
	"fmt"
	"strings"
)

type MCPConfig struct {
	Servers  []MCPServerConfig `yaml:"servers,omitempty"`
	Exposure MCPExposureConfig `yaml:"exposure,omitempty"`
}

type MCPExposureConfig struct {
	Allowlist []string                 `yaml:"allowlist,omitempty"`
	Denylist  []string                 `yaml:"denylist,omitempty"`
	Agents    []MCPAgentExposureConfig `yaml:"agents,omitempty"`
}

type MCPAgentExposureConfig struct {
	Agent     string   `yaml:"agent"`
	Allowlist []string `yaml:"allowlist,omitempty"`
}

type MCPServerConfig struct {
	ID      string            `yaml:"id"`
	Enabled bool              `yaml:"enabled,omitempty"`
	Source  string            `yaml:"source,omitempty"`
	Version string            `yaml:"version,omitempty"`
	Stdio   MCPStdioConfig    `yaml:"stdio,omitempty"`
	Env     []MCPEnvVarConfig `yaml:"env,omitempty"`
}

type MCPStdioConfig struct {
	Command           string   `yaml:"command,omitempty"`
	Args              []string `yaml:"args,omitempty"`
	Workdir           string   `yaml:"workdir,omitempty"`
	StartTimeoutSec   int      `yaml:"start_timeout_sec,omitempty"`
	CallTimeoutSec    int      `yaml:"call_timeout_sec,omitempty"`
	RestartBackoffSec int      `yaml:"restart_backoff_sec,omitempty"`
}

type MCPEnvVarConfig struct {
	Name     string `yaml:"name"`
	Value    string `yaml:"value,omitempty"`
	ValueEnv string `yaml:"value_env,omitempty"`
}

// defaultMCPConfig 返回 MCP 工具接入配置的默认值（默认无 server）。
func defaultMCPConfig() MCPConfig {
	return MCPConfig{
		Servers: nil,
	}
}

// Clone 返回 MCP 配置的独立副本，避免引用共享造成并发污染。
func (c MCPConfig) Clone() MCPConfig {
	cloned := MCPConfig{
		Exposure: c.Exposure.Clone(),
	}
	if len(c.Servers) == 0 {
		return cloned
	}
	cloned.Servers = make([]MCPServerConfig, 0, len(c.Servers))
	for _, server := range c.Servers {
		cloned.Servers = append(cloned.Servers, server.Clone())
	}
	return cloned
}

// Clone 返回单个 MCP server 配置的独立副本。
func (c MCPServerConfig) Clone() MCPServerConfig {
	cloned := c
	cloned.Stdio.Args = append([]string(nil), c.Stdio.Args...)
	if c.Env != nil {
		cloned.Env = make([]MCPEnvVarConfig, len(c.Env))
		copy(cloned.Env, c.Env)
	} else {
		cloned.Env = nil
	}
	return cloned
}

// ApplyDefaults 为 MCP 配置补齐缺省字段，保证运行时行为可预测。
func (c *MCPConfig) ApplyDefaults(defaults MCPConfig) {
	if c == nil {
		return
	}
	if len(c.Servers) == 0 {
		c.Servers = defaults.Clone().Servers
	}
	c.Exposure.ApplyDefaults()
	for index := range c.Servers {
		c.Servers[index].ApplyDefaults()
	}
}

// Validate 校验 MCP server 列表与字段合法性，防止启动后失败。
func (c MCPConfig) Validate() error {
	if err := c.Exposure.Validate(); err != nil {
		return fmt.Errorf("exposure: %w", err)
	}
	if len(c.Servers) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(c.Servers))
	for index, server := range c.Servers {
		normalizedID := strings.ToLower(strings.TrimSpace(server.ID))
		if normalizedID == "" {
			return fmt.Errorf("servers[%d].id is empty", index)
		}
		if _, exists := seen[normalizedID]; exists {
			return fmt.Errorf("duplicate servers[%d].id %q", index, server.ID)
		}
		seen[normalizedID] = struct{}{}

		source := strings.ToLower(strings.TrimSpace(server.Source))
		if source == "" {
			source = "stdio"
		}
		if source != "stdio" {
			return fmt.Errorf("servers[%d].source %q is not supported", index, server.Source)
		}
		if !server.Enabled {
			continue
		}

		if strings.TrimSpace(server.Stdio.Command) == "" {
			return fmt.Errorf("servers[%d].stdio.command is empty", index)
		}
		for envIndex, env := range server.Env {
			if strings.TrimSpace(env.Name) == "" {
				return fmt.Errorf("servers[%d].env[%d].name is empty", index, envIndex)
			}
			hasValue := strings.TrimSpace(env.Value) != ""
			hasValueEnv := strings.TrimSpace(env.ValueEnv) != ""
			if hasValue == hasValueEnv {
				return fmt.Errorf("servers[%d].env[%d] must set exactly one of value/value_env", index, envIndex)
			}
		}
	}
	return nil
}

// Clone 返回 MCP 工具暴露过滤配置的独立副本。
func (c MCPExposureConfig) Clone() MCPExposureConfig {
	cloned := MCPExposureConfig{
		Allowlist: append([]string(nil), c.Allowlist...),
		Denylist:  append([]string(nil), c.Denylist...),
	}
	if len(c.Agents) > 0 {
		cloned.Agents = make([]MCPAgentExposureConfig, 0, len(c.Agents))
		for _, agent := range c.Agents {
			cloned.Agents = append(cloned.Agents, agent.Clone())
		}
	}
	return cloned
}

// Clone 返回单条 agent 暴露规则的独立副本。
func (c MCPAgentExposureConfig) Clone() MCPAgentExposureConfig {
	cloned := c
	cloned.Allowlist = append([]string(nil), c.Allowlist...)
	return cloned
}

// ApplyDefaults 规范化 MCP 工具暴露过滤配置。
func (c *MCPExposureConfig) ApplyDefaults() {
	if c == nil {
		return
	}
	c.Allowlist = normalizePatternList(c.Allowlist)
	c.Denylist = normalizePatternList(c.Denylist)
	for index := range c.Agents {
		c.Agents[index].ApplyDefaults()
	}
}

// ApplyDefaults 规范化单条 agent 暴露规则。
func (c *MCPAgentExposureConfig) ApplyDefaults() {
	if c == nil {
		return
	}
	c.Agent = strings.TrimSpace(c.Agent)
	c.Allowlist = normalizePatternList(c.Allowlist)
}

// Validate 校验 MCP 暴露过滤配置合法性。
func (c MCPExposureConfig) Validate() error {
	seenAgents := make(map[string]struct{}, len(c.Agents))
	for index, agent := range c.Agents {
		normalizedAgent := strings.ToLower(strings.TrimSpace(agent.Agent))
		if normalizedAgent == "" {
			return fmt.Errorf("agents[%d].agent is empty", index)
		}
		if _, exists := seenAgents[normalizedAgent]; exists {
			return fmt.Errorf("duplicate agents[%d].agent %q", index, agent.Agent)
		}
		seenAgents[normalizedAgent] = struct{}{}
		for allowIndex, pattern := range agent.Allowlist {
			if strings.TrimSpace(pattern) == "" {
				return fmt.Errorf("agents[%d].allowlist[%d] is empty", index, allowIndex)
			}
		}
	}
	for index, pattern := range c.Allowlist {
		if strings.TrimSpace(pattern) == "" {
			return fmt.Errorf("allowlist[%d] is empty", index)
		}
	}
	for index, pattern := range c.Denylist {
		if strings.TrimSpace(pattern) == "" {
			return fmt.Errorf("denylist[%d] is empty", index)
		}
	}
	return nil
}

// ApplyDefaults 为 MCP server stdio 配置补齐默认值。
func (c *MCPServerConfig) ApplyDefaults() {
	if c == nil {
		return
	}
	c.Source = strings.ToLower(strings.TrimSpace(c.Source))
	if c.Source == "" {
		c.Source = "stdio"
	}
}

// normalizePatternList 规范化暴露过滤模式列表并剔除空项。
func normalizePatternList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
