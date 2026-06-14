package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"neo-code/internal/cli"
)

func main() {
	if err := cli.Execute(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "neocode: %v\n", err)
		exitCode := 1
		var exitCoder interface{ ExitCode() int }
		if errors.As(err, &exitCoder) {
			exitCode = exitCoder.ExitCode()
		}
		os.Exit(exitCode)
	}
	if notice := cli.ConsumeUpdateNotice(); notice != "" {
		fmt.Fprintln(os.Stdout, notice)
	}
}
