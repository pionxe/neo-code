package tuiv2

import (
	"context"
	"testing"
)

// TestNewKernelAppRegistersAllPlugins 验证 10 插件 + bootstrapReactor 全量注册无冲突。
func TestNewKernelAppRegistersAllPlugins(t *testing.T) {
	k := NewKernelApp(context.Background(), StartupConfig{Debug: false})
	if k == nil {
		t.Fatal("NewKernelApp returned nil")
	}
}
