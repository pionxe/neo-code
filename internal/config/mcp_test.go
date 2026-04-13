package config

import (
	"strings"
	"testing"
)

func TestMCPServerConfigApplyDefaultsNormalizesSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
	}{
		{name: "trimmed", source: " STDIO "},
		{name: "empty", source: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := MCPServerConfig{
				ID:      "test-server",
				Enabled: true,
				Source:  tt.source,
				Stdio:   MCPStdioConfig{Command: "cmd"},
			}
			cfg.ApplyDefaults()
			if cfg.Source != "stdio" {
				t.Fatalf("expected normalized source 'stdio', got %q", cfg.Source)
			}
		})
	}
}

func TestMCPConfigValidateSkipsDisabledServerWithoutCommand(t *testing.T) {
	t.Parallel()

	cfg := MCPConfig{
		Servers: []MCPServerConfig{
			{ID: "disabled-no-cmd", Enabled: false, Source: "stdio"},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected disabled server without command to pass validation, got %v", err)
	}
}

func TestMCPConfigValidateRejectsServerWithEmptyEnvBothValues(t *testing.T) {
	t.Parallel()

	cfg := MCPConfig{
		Servers: []MCPServerConfig{
			{
				ID:      "test",
				Enabled: true,
				Source:  "stdio",
				Stdio:   MCPStdioConfig{Command: "cmd"},
				Env: []MCPEnvVarConfig{
					{Name: "VAR", Value: "", ValueEnv: ""},
					{Name: "VAR2", Value: "a", ValueEnv: "b"},
				},
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for invalid env bindings")
	}
}

func TestMCPConfigValidateRejectsServerWithMissingEnvName(t *testing.T) {
	t.Parallel()

	cfg := MCPConfig{
		Servers: []MCPServerConfig{
			{
				ID:      "test",
				Enabled: true,
				Source:  "stdio",
				Stdio:   MCPStdioConfig{Command: "cmd"},
				Env: []MCPEnvVarConfig{
					{Name: "  ", Value: "a"},
				},
			},
		},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "name is empty") {
		t.Fatalf("expected empty env name error, got %v", err)
	}
}

func TestMCPServerConfigCloneWithoutEnv(t *testing.T) {
	t.Parallel()

	original := MCPServerConfig{
		ID:      "test",
		Enabled: true,
		Source:  "stdio",
		Stdio:   MCPStdioConfig{Command: "cmd", Args: []string{"--help"}},
		Env:     nil,
	}
	cloned := original.Clone()
	cloned.Stdio.Args[0] = "--version"
	if original.Stdio.Args[0] == cloned.Stdio.Args[0] {
		t.Fatal("expected args clone to be independent")
	}
}

func TestMCPServerConfigCloneWithEmptyEnvSlice(t *testing.T) {
	t.Parallel()

	original := MCPServerConfig{
		ID:    "test",
		Stdio: MCPStdioConfig{Command: "cmd"},
		Env:   []MCPEnvVarConfig{},
	}
	cloned := original.Clone()
	if cloned.Env == nil {
		t.Fatal("expected cloned env to preserve empty slice")
	}
	if len(cloned.Env) != 0 {
		t.Fatalf("expected cloned env length to be 0, got %d", len(cloned.Env))
	}
}

func TestMCPServerConfigCloneEnvIndependence(t *testing.T) {
	t.Parallel()

	original := MCPServerConfig{
		ID:    "test",
		Stdio: MCPStdioConfig{Command: "cmd"},
		Env: []MCPEnvVarConfig{
			{Name: "TOKEN", Value: "secret"},
		},
	}
	cloned := original.Clone()
	cloned.Env[0].Value = "modified"
	if original.Env[0].Value == cloned.Env[0].Value {
		t.Fatal("expected Env clone to be independent")
	}
}

func TestMCPExposureConfigCloneIndependence(t *testing.T) {
	t.Parallel()

	original := MCPExposureConfig{
		Allowlist: []string{"docs"},
		Denylist:  []string{"admin.secret"},
		Agents: []MCPAgentExposureConfig{
			{Agent: "coder", Allowlist: []string{"docs.search"}},
		},
	}

	cloned := original.Clone()
	cloned.Allowlist[0] = "changed"
	cloned.Denylist[0] = "changed"
	cloned.Agents[0].Agent = "planner"
	cloned.Agents[0].Allowlist[0] = "changed"

	if original.Allowlist[0] != "docs" || original.Denylist[0] != "admin.secret" {
		t.Fatalf("expected top-level slices to remain unchanged, got %+v", original)
	}
	if original.Agents[0].Agent != "coder" || original.Agents[0].Allowlist[0] != "docs.search" {
		t.Fatalf("expected agent clone independence, got %+v", original.Agents[0])
	}
}

func TestMCPExposureConfigApplyDefaultsNormalizesValues(t *testing.T) {
	t.Parallel()

	cfg := MCPExposureConfig{
		Allowlist: []string{" Docs ", "", "MCP.Search.Live"},
		Denylist:  []string{" Admin.Secret "},
		Agents: []MCPAgentExposureConfig{
			{Agent: " Planner ", Allowlist: []string{" Docs.Search ", ""}},
		},
	}

	cfg.ApplyDefaults()
	if strings.Join(cfg.Allowlist, ",") != "docs,mcp.search.live" {
		t.Fatalf("unexpected allowlist normalization: %+v", cfg.Allowlist)
	}
	if strings.Join(cfg.Denylist, ",") != "admin.secret" {
		t.Fatalf("unexpected denylist normalization: %+v", cfg.Denylist)
	}
	if cfg.Agents[0].Agent != "Planner" {
		t.Fatalf("expected agent to keep trimmed original casing, got %q", cfg.Agents[0].Agent)
	}
	if strings.Join(cfg.Agents[0].Allowlist, ",") != "docs.search" {
		t.Fatalf("unexpected agent allowlist normalization: %+v", cfg.Agents[0].Allowlist)
	}
}

func TestMCPExposureConfigValidateRejectsInvalidRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  MCPExposureConfig
		want string
	}{
		{
			name: "empty agent",
			cfg: MCPExposureConfig{
				Agents: []MCPAgentExposureConfig{{Agent: " ", Allowlist: []string{"docs"}}},
			},
			want: "agents[0].agent is empty",
		},
		{
			name: "duplicate agent",
			cfg: MCPExposureConfig{
				Agents: []MCPAgentExposureConfig{
					{Agent: "coder", Allowlist: []string{"docs"}},
					{Agent: "CoDeR", Allowlist: []string{"search"}},
				},
			},
			want: "duplicate agents[1].agent",
		},
		{
			name: "empty allowlist item",
			cfg:  MCPExposureConfig{Allowlist: []string{"docs", " "}},
			want: "allowlist[1] is empty",
		},
		{
			name: "empty denylist item",
			cfg:  MCPExposureConfig{Denylist: []string{" "}},
			want: "denylist[0] is empty",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q, got %v", tt.want, err)
			}
		})
	}
}

func TestMCPConfigClonePreservesExposureWithoutServers(t *testing.T) {
	t.Parallel()

	cfg := MCPConfig{
		Exposure: MCPExposureConfig{
			Allowlist: []string{"docs"},
			Agents: []MCPAgentExposureConfig{
				{Agent: "coder", Allowlist: []string{"docs.search"}},
			},
		},
	}

	cloned := cfg.Clone()
	if strings.Join(cloned.Exposure.Allowlist, ",") != "docs" {
		t.Fatalf("expected exposure allowlist cloned, got %+v", cloned.Exposure)
	}
	if len(cloned.Servers) != 0 {
		t.Fatalf("expected no servers, got %+v", cloned.Servers)
	}
}

func TestMCPConfigValidateWrapsExposureErrors(t *testing.T) {
	t.Parallel()

	cfg := MCPConfig{
		Exposure: MCPExposureConfig{
			Allowlist: []string{" "},
		},
	}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "exposure: allowlist[0] is empty") {
		t.Fatalf("expected wrapped exposure error, got %v", err)
	}
}

func TestMCPApplyDefaultsNilReceivers(t *testing.T) {
	t.Parallel()

	var mcpCfg *MCPConfig
	mcpCfg.ApplyDefaults(MCPConfig{})

	var serverCfg *MCPServerConfig
	serverCfg.ApplyDefaults()

	var expCfg *MCPExposureConfig
	expCfg.ApplyDefaults()

	var agExpCfg *MCPAgentExposureConfig
	agExpCfg.ApplyDefaults()
}
