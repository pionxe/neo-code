package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"neo-code/internal/cli"
)

func main() {
	if err := cli.ExecuteGatewayServer(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "neocode-gateway: %v\n", err)
		exitCode := 1
		var exitCoder interface{ ExitCode() int }
		if errors.As(err, &exitCoder) {
			exitCode = exitCoder.ExitCode()
		}
		os.Exit(exitCode)
	}
}
