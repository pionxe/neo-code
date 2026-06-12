package main

import (
	"fmt"
	"os"
	"path/filepath"

	runtimehooks "neo-code/internal/runtime/hooks"
)

func main() {
	content, err := runtimehooks.MarshalPayloadJSONSchema()
	if err != nil {
		fail(err)
	}
	targetPath := filepath.Clean(filepath.Join("..", "..", "..", "docs", "reference", schemaFileName()))
	if err := os.WriteFile(targetPath, content, 0o644); err != nil {
		fail(err)
	}
}

func schemaFileName() string {
	return fmt.Sprintf("hook-payload.v%s.json", runtimehooks.PayloadVersion)
}

func fail(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "generate hook payload schema: %v\n", err)
	os.Exit(1)
}
