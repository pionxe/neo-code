package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	gatewayclient "neo-code/internal/gateway/client"
	"neo-code/internal/tuiv2"
	"neo-code/internal/tuiv2/fakegateway"
	"neo-code/internal/tuiv2/gateway"
)

const (
	backendFake    = "fake"
	backendGateway = "gateway"
)

// 网关后端可选连接参数（--gateway-address/--gateway-token-file）：
// 空=默认地址解析与默认 token 位置（v1 行为）。
var (
	gatewayAddress   string
	gatewayTokenFile string
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
		// 内核接线切换（issue #41，S3-4）：kernel + 插件装配路径取代旧
		// app 层路由。旧路由文件在后续删除 commit 前保持并存（绞杀者模式）。
		tuiv2.NewKernelApp(context.Background(), cfg),
		tea.WithInput(os.Stdin),
		tea.WithOutput(os.Stdout),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
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
	flags.StringVar(&gatewayAddress, "gateway-address", "", "gateway IPC socket path (empty = default ~/.neocode/run/gateway.sock; v1 RPC 客户端为 IPC-only 传输)")
	flags.StringVar(&gatewayTokenFile, "gateway-token-file", "", "gateway auth token file (empty = default location)")

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

// newGatewayClient 根据启动参数创建 Gateway 客户端。
// fake 后端走场景模拟器；gateway 后端走 RealClient 薄翻译层
// （ADR-004：复用 v1 RPC 客户端的认证/心跳/重试/地址解析，自动拉起
// 强制关闭——v1 自我重执行在 tuiv2 二进制下不可用，网关需外部先行启动）。
func newGatewayClient(cfg tuiv2.StartupConfig) (gateway.Client, error) {
	switch cfg.Backend {
	case backendFake:
		return fakegateway.New(fakegateway.Config{Scenario: cfg.Scenario})
	case backendGateway:
		return gateway.NewRealClient(gateway.RealClientOptions{
			RPC: gatewayclient.GatewayRPCClientOptions{
				ListenAddress: gatewayAddress,
				TokenFile:     gatewayTokenFile,
			},
		})
	default:
		return nil, fmt.Errorf("unsupported --backend=%q", cfg.Backend)
	}
}
