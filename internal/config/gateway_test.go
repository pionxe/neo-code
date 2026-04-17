package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGatewayConfigDefaultsAndClone(t *testing.T) {
	t.Parallel()

	defaults := defaultGatewayConfig()
	if defaults.Security.ACLMode != DefaultGatewayACLMode {
		t.Fatalf("acl_mode = %q, want %q", defaults.Security.ACLMode, DefaultGatewayACLMode)
	}
	if defaults.Limits.MaxFrameBytes != DefaultGatewayMaxFrameBytes {
		t.Fatalf("max_frame_bytes = %d, want %d", defaults.Limits.MaxFrameBytes, DefaultGatewayMaxFrameBytes)
	}
	if !defaults.Observability.Enabled() {
		t.Fatal("metrics should be enabled by default")
	}

	cloned := defaults.Clone()
	cloned.Security.AllowOrigins[0] = "http://changed"
	if defaults.Security.AllowOrigins[0] == "http://changed" {
		t.Fatal("clone should not share allow_origins slice")
	}
}

func TestGatewayConfigApplyDefaultsAndValidate(t *testing.T) {
	t.Parallel()

	cfg := GatewayConfig{}
	cfg.ApplyDefaults(defaultGatewayConfig())
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate defaulted gateway config: %v", err)
	}

	cfg.Observability.MetricsEnabled = boolPtr(false)
	cfg.ApplyDefaults(defaultGatewayConfig())
	if cfg.Observability.Enabled() {
		t.Fatal("explicit metrics_enabled=false should be preserved")
	}

	invalid := cfg.Clone()
	invalid.Security.ACLMode = "allow-all"
	if err := invalid.Validate(); err == nil || !strings.Contains(err.Error(), "acl_mode") {
		t.Fatalf("expected acl_mode error, got %v", err)
	}
}

func TestLoadGatewayConfig(t *testing.T) {
	t.Run("cancelled context returns error", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := LoadGatewayConfig(ctx, t.TempDir()); err == nil {
			t.Fatal("expected canceled context error")
		}
	})

	t.Run("missing file uses defaults", func(t *testing.T) {
		t.Parallel()
		cfg, err := LoadGatewayConfig(context.Background(), t.TempDir())
		if err != nil {
			t.Fatalf("load gateway config: %v", err)
		}
		if !cfg.Observability.Enabled() {
			t.Fatal("metrics should default to enabled")
		}
	})

	t.Run("empty basedir falls back to user home", func(t *testing.T) {
		cfg, err := LoadGatewayConfig(context.Background(), "")
		if err != nil {
			t.Fatalf("load gateway config with empty base dir: %v", err)
		}
		_ = cfg
	})

	t.Run("reads gateway section", func(t *testing.T) {
		t.Parallel()

		baseDir := t.TempDir()
		configPath := filepath.Join(baseDir, configName)
		content := `
selected_provider: openai
current_model: gpt-5.4
shell: bash
gateway:
  security:
    acl_mode: strict
    token_file: /tmp/neocode-auth.json
    allow_origins:
      - http://localhost
      - app://
  limits:
    max_frame_bytes: 2048
    ipc_max_connections: 32
    http_max_request_bytes: 4096
    http_max_stream_connections: 16
  timeouts:
    ipc_read_sec: 20
    ipc_write_sec: 21
    http_read_sec: 9
    http_write_sec: 10
    http_shutdown_sec: 4
  observability:
    metrics_enabled: false
`
		if err := os.WriteFile(configPath, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		cfg, err := LoadGatewayConfig(context.Background(), baseDir)
		if err != nil {
			t.Fatalf("load gateway config: %v", err)
		}
		if cfg.Limits.MaxFrameBytes != 2048 {
			t.Fatalf("max_frame_bytes = %d, want %d", cfg.Limits.MaxFrameBytes, 2048)
		}
		if cfg.Observability.Enabled() {
			t.Fatal("metrics_enabled should be false")
		}
		if cfg.Security.TokenFile != "/tmp/neocode-auth.json" {
			t.Fatalf("token_file = %q, want %q", cfg.Security.TokenFile, "/tmp/neocode-auth.json")
		}
	})

	t.Run("invalid gateway section returns error", func(t *testing.T) {
		t.Parallel()

		baseDir := t.TempDir()
		configPath := filepath.Join(baseDir, configName)
		content := `
selected_provider: openai
current_model: gpt-5.4
shell: bash
gateway:
  limits:
    max_frame_bytes: 0
`
		if err := os.WriteFile(configPath, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		_, err := LoadGatewayConfig(context.Background(), baseDir)
		if err == nil {
			t.Fatal("expected invalid gateway config error")
		}
		if !strings.Contains(err.Error(), "max_frame_bytes") {
			t.Fatalf("error = %v, want max_frame_bytes validation", err)
		}
	})

	t.Run("invalid yaml returns parse error", func(t *testing.T) {
		t.Parallel()
		baseDir := t.TempDir()
		configPath := filepath.Join(baseDir, configName)
		if err := os.WriteFile(configPath, []byte("gateway: ["), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		_, err := LoadGatewayConfig(context.Background(), baseDir)
		if err == nil || !strings.Contains(err.Error(), "parse gateway config file") {
			t.Fatalf("expected parse gateway config error, got %v", err)
		}
	})

	t.Run("unknown gateway field returns parse error", func(t *testing.T) {
		t.Parallel()
		baseDir := t.TempDir()
		configPath := filepath.Join(baseDir, configName)
		content := `
selected_provider: openai
current_model: gpt-5.4
shell: bash
gateway:
  security:
    acl_mode: strict
    token_fiel: /tmp/typo-auth.json
`
		if err := os.WriteFile(configPath, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		_, err := LoadGatewayConfig(context.Background(), baseDir)
		if err == nil {
			t.Fatal("expected unknown gateway field parse error")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "field") {
			t.Fatalf("error = %v, want contains unknown field diagnostic", err)
		}
	})
}

func TestGatewaySecurityConfigApplyDefaultsAndValidateBranches(t *testing.T) {
	t.Parallel()

	defaults := GatewaySecurityConfig{
		ACLMode:      DefaultGatewayACLMode,
		TokenFile:    "/tmp/default-auth.json",
		AllowOrigins: []string{"http://localhost"},
	}

	cfg := GatewaySecurityConfig{}
	cfg.ApplyDefaults(defaults)
	if cfg.ACLMode != defaults.ACLMode {
		t.Fatalf("acl_mode = %q, want %q", cfg.ACLMode, defaults.ACLMode)
	}
	if cfg.TokenFile != defaults.TokenFile {
		t.Fatalf("token_file = %q, want %q", cfg.TokenFile, defaults.TokenFile)
	}
	if len(cfg.AllowOrigins) != 1 || cfg.AllowOrigins[0] != "http://localhost" {
		t.Fatalf("allow_origins = %#v, want default allow list", cfg.AllowOrigins)
	}

	cfg = GatewaySecurityConfig{
		AllowOrigins: []string{"  http://localhost:3000  ", " ", "app://desktop"},
	}
	cfg.ApplyDefaults(defaults)
	if len(cfg.AllowOrigins) != 2 {
		t.Fatalf("allow_origins len = %d, want %d", len(cfg.AllowOrigins), 2)
	}
	if cfg.AllowOrigins[0] != "http://localhost:3000" || cfg.AllowOrigins[1] != "app://desktop" {
		t.Fatalf("allow_origins = %#v, want normalized values", cfg.AllowOrigins)
	}

	invalidACL := GatewaySecurityConfig{ACLMode: "allow-all"}
	if err := invalidACL.Validate(); err == nil || !strings.Contains(err.Error(), "acl_mode") {
		t.Fatalf("expected acl_mode validation error, got %v", err)
	}

	invalidTokenPath := GatewaySecurityConfig{ACLMode: DefaultGatewayACLMode, TokenFile: "."}
	if err := invalidTokenPath.Validate(); err == nil || !strings.Contains(err.Error(), "token_file") {
		t.Fatalf("expected token_file validation error, got %v", err)
	}

	invalidAllowOrigins := GatewaySecurityConfig{
		ACLMode:      DefaultGatewayACLMode,
		AllowOrigins: []string{"http://localhost", " "},
	}
	if err := invalidAllowOrigins.Validate(); err == nil || !strings.Contains(err.Error(), "allow_origins") {
		t.Fatalf("expected allow_origins validation error, got %v", err)
	}
}

func TestGatewayLimitsConfigApplyDefaultsAndValidateBranches(t *testing.T) {
	t.Parallel()

	defaults := GatewayLimitsConfig{
		MaxFrameBytes:            1,
		IPCMaxConnections:        2,
		HTTPMaxRequestBytes:      3,
		HTTPMaxStreamConnections: 4,
	}
	limits := GatewayLimitsConfig{}
	limits.ApplyDefaults(defaults)
	if limits != defaults {
		t.Fatalf("limits defaults = %#v, want %#v", limits, defaults)
	}

	cases := []GatewayLimitsConfig{
		{MaxFrameBytes: 0, IPCMaxConnections: 1, HTTPMaxRequestBytes: 1, HTTPMaxStreamConnections: 1},
		{MaxFrameBytes: 1, IPCMaxConnections: 0, HTTPMaxRequestBytes: 1, HTTPMaxStreamConnections: 1},
		{MaxFrameBytes: 1, IPCMaxConnections: 1, HTTPMaxRequestBytes: 0, HTTPMaxStreamConnections: 1},
		{MaxFrameBytes: 1, IPCMaxConnections: 1, HTTPMaxRequestBytes: 1, HTTPMaxStreamConnections: 0},
	}
	for _, tc := range cases {
		if err := tc.Validate(); err == nil {
			t.Fatalf("expected validate error for limits %#v", tc)
		}
	}

	if err := (GatewayLimitsConfig{
		MaxFrameBytes:            1,
		IPCMaxConnections:        1,
		HTTPMaxRequestBytes:      1,
		HTTPMaxStreamConnections: 1,
	}).Validate(); err != nil {
		t.Fatalf("expected valid limits, got %v", err)
	}
}

func TestGatewayTimeoutsConfigApplyDefaultsAndValidateBranches(t *testing.T) {
	t.Parallel()

	defaults := GatewayTimeoutsConfig{
		IPCReadSec:      1,
		IPCWriteSec:     2,
		HTTPReadSec:     3,
		HTTPWriteSec:    4,
		HTTPShutdownSec: 5,
	}
	timeouts := GatewayTimeoutsConfig{}
	timeouts.ApplyDefaults(defaults)
	if timeouts != defaults {
		t.Fatalf("timeouts defaults = %#v, want %#v", timeouts, defaults)
	}

	cases := []GatewayTimeoutsConfig{
		{IPCReadSec: 0, IPCWriteSec: 1, HTTPReadSec: 1, HTTPWriteSec: 1, HTTPShutdownSec: 1},
		{IPCReadSec: 1, IPCWriteSec: 0, HTTPReadSec: 1, HTTPWriteSec: 1, HTTPShutdownSec: 1},
		{IPCReadSec: 1, IPCWriteSec: 1, HTTPReadSec: 0, HTTPWriteSec: 1, HTTPShutdownSec: 1},
		{IPCReadSec: 1, IPCWriteSec: 1, HTTPReadSec: 1, HTTPWriteSec: 0, HTTPShutdownSec: 1},
		{IPCReadSec: 1, IPCWriteSec: 1, HTTPReadSec: 1, HTTPWriteSec: 1, HTTPShutdownSec: 0},
	}
	for _, tc := range cases {
		if err := tc.Validate(); err == nil {
			t.Fatalf("expected validate error for timeouts %#v", tc)
		}
	}

	if err := (GatewayTimeoutsConfig{
		IPCReadSec:      1,
		IPCWriteSec:     1,
		HTTPReadSec:     1,
		HTTPWriteSec:    1,
		HTTPShutdownSec: 1,
	}).Validate(); err != nil {
		t.Fatalf("expected valid timeouts, got %v", err)
	}
}

func TestGatewayObservabilityBranches(t *testing.T) {
	t.Parallel()

	var nilDefaults GatewayObservabilityConfig
	cfg := GatewayObservabilityConfig{}
	cfg.ApplyDefaults(nilDefaults)
	if !cfg.Enabled() {
		t.Fatal("metrics should be enabled by fallback default")
	}

	defaultDisabled := GatewayObservabilityConfig{MetricsEnabled: boolPtr(false)}
	cfg = GatewayObservabilityConfig{}
	cfg.ApplyDefaults(defaultDisabled)
	if cfg.Enabled() {
		t.Fatal("metrics should follow defaults when explicitly disabled")
	}

	cloned := cfg.Clone()
	*cfg.MetricsEnabled = true
	if *cloned.MetricsEnabled {
		t.Fatal("clone should deep copy metrics_enabled pointer")
	}
}

func TestGatewayConfigValidateWrapsSubErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  GatewayConfig
		want string
	}{
		{
			name: "security",
			cfg: GatewayConfig{
				Security: GatewaySecurityConfig{ACLMode: "bad"},
				Limits: GatewayLimitsConfig{
					MaxFrameBytes:            1,
					IPCMaxConnections:        1,
					HTTPMaxRequestBytes:      1,
					HTTPMaxStreamConnections: 1,
				},
				Timeouts: GatewayTimeoutsConfig{
					IPCReadSec: 1, IPCWriteSec: 1, HTTPReadSec: 1, HTTPWriteSec: 1, HTTPShutdownSec: 1,
				},
			},
			want: "security:",
		},
		{
			name: "limits",
			cfg: GatewayConfig{
				Security: GatewaySecurityConfig{ACLMode: DefaultGatewayACLMode},
				Limits: GatewayLimitsConfig{
					MaxFrameBytes:            0,
					IPCMaxConnections:        1,
					HTTPMaxRequestBytes:      1,
					HTTPMaxStreamConnections: 1,
				},
				Timeouts: GatewayTimeoutsConfig{
					IPCReadSec: 1, IPCWriteSec: 1, HTTPReadSec: 1, HTTPWriteSec: 1, HTTPShutdownSec: 1,
				},
			},
			want: "limits:",
		},
		{
			name: "timeouts",
			cfg: GatewayConfig{
				Security: GatewaySecurityConfig{ACLMode: DefaultGatewayACLMode},
				Limits: GatewayLimitsConfig{
					MaxFrameBytes:            1,
					IPCMaxConnections:        1,
					HTTPMaxRequestBytes:      1,
					HTTPMaxStreamConnections: 1,
				},
				Timeouts: GatewayTimeoutsConfig{
					IPCReadSec: 0, IPCWriteSec: 1, HTTPReadSec: 1, HTTPWriteSec: 1, HTTPShutdownSec: 1,
				},
			},
			want: "timeouts:",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected wrapped error contains %q, got %v", tc.want, err)
			}
		})
	}
}

func TestGatewayNormalizeAllowOrigins(t *testing.T) {
	t.Parallel()

	normalized := normalizeGatewayAllowOrigins([]string{" http://localhost ", "", "  ", "app://desktop"})
	if len(normalized) != 2 {
		t.Fatalf("normalized len = %d, want %d", len(normalized), 2)
	}
	if normalized[0] != "http://localhost" || normalized[1] != "app://desktop" {
		t.Fatalf("normalized = %#v, want trimmed values", normalized)
	}
}

func TestGatewayApplyDefaultsNilReceivers(t *testing.T) {
	t.Parallel()

	var gatewayCfg *GatewayConfig
	gatewayCfg.ApplyDefaults(defaultGatewayConfig())

	var securityCfg *GatewaySecurityConfig
	securityCfg.ApplyDefaults(GatewaySecurityConfig{ACLMode: DefaultGatewayACLMode})

	var limitsCfg *GatewayLimitsConfig
	limitsCfg.ApplyDefaults(GatewayLimitsConfig{MaxFrameBytes: 1})

	var timeoutsCfg *GatewayTimeoutsConfig
	timeoutsCfg.ApplyDefaults(GatewayTimeoutsConfig{IPCReadSec: 1})

	var observabilityCfg *GatewayObservabilityConfig
	observabilityCfg.ApplyDefaults(GatewayObservabilityConfig{MetricsEnabled: boolPtr(false)})
}
