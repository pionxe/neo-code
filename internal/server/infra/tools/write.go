package tools

import (
	"fmt"
)

type WriteTool struct{}

func (w *WriteTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "write",
		Description: "Write entire file content in the workspace. Automatically creates parent directories if they do not exist.",
		Parameters: []ToolParamSpec{
			{Name: "filePath", Type: "string", Required: true, Description: "Target file path within the workspace."},
			{Name: "content", Type: "string", Required: true, Description: "The complete new content to be written to the file."},
		},
	}
}

func (w *WriteTool) Run(params map[string]interface{}) *ToolResult {
	filePath, errRes := requiredString(params, "filePath")
	if errRes != nil {
		errRes.ToolName = w.Definition().Name
		return errRes
	}
	filePath, pathErr := ensureWorkspacePath(filePath)
	if pathErr != nil {
		pathErr.ToolName = w.Definition().Name
		return pathErr
	}
	content, errRes := requiredString(params, "content")
	if errRes != nil {
		errRes.ToolName = w.Definition().Name
		return errRes
	}

	if err := AtomicWrite(filePath, []byte(content)); err != nil {
		return &ToolResult{ToolName: w.Definition().Name, Success: false, Error: fmt.Sprintf("failed to write file: %v", err)}
	}
	return &ToolResult{ToolName: w.Definition().Name, Success: true, Output: fmt.Sprintf("Successfully wrote to %s", filePath), Metadata: map[string]interface{}{"filePath": filePath, "bytesWritten": len(content)}}
}
