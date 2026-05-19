package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"neo-code/internal/tuiv2"
	"neo-code/internal/tuiv2/fakegateway"
	"neo-code/internal/tuiv2/gateway"
)

const (
	backendFake    = "fake"
	backendGateway = "gateway"
)

// main 是 TUI v2 独立二进制入口，只负责参数解析、客户端选择和启动 Bubble Tea 程序。
func main() {
	cfg, err := parseStartupConfig(os.Args[1:], os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	client, err := newGatewayClient(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	cfg.Client = client

	if _, err := tea.NewProgram(
		tuiv2.NewApp(cfg),
		tea.WithInput(os.Stdin),
		tea.WithOutput(os.Stdout),
		tea.WithAltScreen(),
	).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "start TUI v2: %v\n", err)
		os.Exit(1)
	}
}

// parseStartupConfig 解析 TUI v2 独立入口参数，保持与 v1 cobra 命令树完全隔离。
func parseStartupConfig(args []string, stderr io.Writer) (tuiv2.StartupConfig, error) {
	cfg := tuiv2.StartupConfig{
		Backend:  backendFake,
		Scenario: fakegateway.ScenarioDefault,
	}

	flags := flag.NewFlagSet("neocode-tuiv2", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&cfg.Backend, "backend", cfg.Backend, "gateway backend: fake or gateway")
	flags.StringVar(&cfg.Scenario, "scenario", cfg.Scenario, "fake gateway scenario")
	flags.BoolVar(&cfg.Debug, "debug", false, "show TUI v2 debug information")

	if err := flags.Parse(args); err != nil {
		return tuiv2.StartupConfig{}, err
	}
	if flags.NArg() > 0 {
		return tuiv2.StartupConfig{}, fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}
	if cfg.Backend == "" {
		return tuiv2.StartupConfig{}, fmt.Errorf("--backend must not be empty")
	}
	if cfg.Scenario == "" {
		return tuiv2.StartupConfig{}, fmt.Errorf("--scenario must not be empty")
	}
	return cfg, nil
}

// newGatewayClient 根据启动参数创建 Gateway 客户端，真实 Gateway 后端保留到后续阶段接入。
func newGatewayClient(cfg tuiv2.StartupConfig) (gateway.Client, error) {
	switch cfg.Backend {
	case backendFake:
		return fakegateway.New(fakegateway.Config{Scenario: cfg.Scenario})
	case backendGateway:
		return nil, fmt.Errorf("--backend=gateway is reserved for Phase 20")
	default:
		return nil, fmt.Errorf("unsupported --backend=%q", cfg.Backend)
	}
}
