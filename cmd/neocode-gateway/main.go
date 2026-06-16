package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"neo-code/internal/cli"
)

var executeGatewayServer = cli.ExecuteGatewayServer
var exitGatewayProcess = os.Exit

func main() {
	exitGatewayProcess(runMain(context.Background(), os.Args[1:], os.Stderr))
}

// runMain 执行 gateway-only 主流程并返回进程退出码，方便测试覆盖错误分支。
func runMain(ctx context.Context, args []string, stderr io.Writer) int {
	if err := executeGatewayServer(ctx, args); err != nil {
		_, _ = fmt.Fprintf(stderr, "neocode-gateway: %v\n", err)
		exitCode := 1
		var exitCoder interface{ ExitCode() int }
		if errors.As(err, &exitCoder) {
			exitCode = exitCoder.ExitCode()
		}
		return exitCode
	}
	return 0
}
