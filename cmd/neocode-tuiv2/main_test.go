package main

import (
	"io"
	"testing"

	"neo-code/internal/tuiv2"
	"neo-code/internal/tuiv2/fakegateway"
)

func TestParseStartupConfigDefaults(t *testing.T) {
	cfg, err := parseStartupConfig(nil, io.Discard)
	if err != nil {
		t.Fatalf("parseStartupConfig() error = %v", err)
	}

	if cfg.Backend != backendFake {
		t.Fatalf("Backend = %q, want %q", cfg.Backend, backendFake)
	}
	if cfg.Scenario != fakegateway.ScenarioDefault {
		t.Fatalf("Scenario = %q, want %q", cfg.Scenario, fakegateway.ScenarioDefault)
	}
	if cfg.Debug {
		t.Fatal("Debug = true, want false")
	}
}

func TestParseStartupConfigExplicitValues(t *testing.T) {
	cfg, err := parseStartupConfig([]string{
		"--backend=fake",
		"--scenario=tool_approval",
		"--debug",
	}, io.Discard)
	if err != nil {
		t.Fatalf("parseStartupConfig() error = %v", err)
	}

	if cfg.Backend != backendFake {
		t.Fatalf("Backend = %q, want %q", cfg.Backend, backendFake)
	}
	if cfg.Scenario != fakegateway.ScenarioToolApproval {
		t.Fatalf("Scenario = %q, want %q", cfg.Scenario, fakegateway.ScenarioToolApproval)
	}
	if !cfg.Debug {
		t.Fatal("Debug = false, want true")
	}
}

func TestParseStartupConfigRejectsInvalidShape(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "positional", args: []string{"extra"}},
		{name: "empty backend", args: []string{"--backend="}},
		{name: "empty scenario", args: []string{"--scenario="}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseStartupConfig(tt.args, io.Discard); err == nil {
				t.Fatal("parseStartupConfig() error = nil, want error")
			}
		})
	}
}

func TestNewGatewayClient(t *testing.T) {
	cfg, err := parseStartupConfig([]string{"--scenario=gateway_offline"}, io.Discard)
	if err != nil {
		t.Fatalf("parseStartupConfig() error = %v", err)
	}

	client, err := newGatewayClient(cfg)
	if err != nil {
		t.Fatalf("newGatewayClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("newGatewayClient() client = nil")
	}
}

func TestNewGatewayClientRejectsReservedAndUnknownBackends(t *testing.T) {
	tests := []string{backendGateway, "other"}
	for _, backend := range tests {
		t.Run(backend, func(t *testing.T) {
			_, err := newGatewayClient(mustConfig(t, []string{"--backend=" + backend}))
			if err == nil {
				t.Fatal("newGatewayClient() error = nil, want error")
			}
		})
	}
}

func mustConfig(t *testing.T, args []string) tuiv2.StartupConfig {
	t.Helper()
	cfg, err := parseStartupConfig(args, io.Discard)
	if err != nil {
		t.Fatalf("parseStartupConfig() error = %v", err)
	}
	return cfg
}
