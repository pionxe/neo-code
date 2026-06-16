package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"neo-code/internal/cli"
)

var executeCLI = cli.Execute
var consumeCLIUpdateNotice = cli.ConsumeUpdateNotice
var exitProcess = os.Exit

func main() {
	exitProcess(runMain(context.Background(), os.Stdout, os.Stderr))
}

// runMain 执行 CLI 主流程并返回最终进程退出码，便于主入口与测试共享同一套分支逻辑。
func runMain(ctx context.Context, stdout io.Writer, stderr io.Writer) int {
	if err := executeCLI(ctx); err != nil {
		_, _ = fmt.Fprintf(stderr, "neocode: %v\n", err)
		exitCode := 1
		var exitCoder interface{ ExitCode() int }
		if errors.As(err, &exitCoder) {
			exitCode = exitCoder.ExitCode()
		}
		return exitCode
	}
	if notice := consumeCLIUpdateNotice(); notice != "" {
		_, _ = fmt.Fprintln(stdout, notice)
	}
	return 0
}
