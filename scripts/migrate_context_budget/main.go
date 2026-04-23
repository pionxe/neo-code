package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"neo-code/internal/config"
)

const defaultNeoCodeDirName = ".neocode"

// main 解析命令行参数并调用正式配置迁移实现。
func main() {
	baseDir := flag.String("base-dir", defaultBaseDir(), "NeoCode 配置根目录，默认为 ~/.neocode")
	target := flag.String("target", "", "指定要迁移的 config.yaml；为空时使用 <base-dir>/config.yaml")
	dryRun := flag.Bool("dry-run", false, "只检查是否需要迁移，不写入文件")
	flag.Parse()

	path := strings.TrimSpace(*target)
	if path == "" {
		path = filepath.Join(strings.TrimSpace(*baseDir), "config.yaml")
	}
	result, err := config.MigrateContextBudgetConfigFile(path, *dryRun)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "迁移失败: %v\n", err)
		os.Exit(1)
	}
	printMigrationResult(result, *dryRun)
}

// printMigrationResult 输出迁移结果，保持脚本与打包 CLI 的用户提示一致。
func printMigrationResult(result config.ContextBudgetMigrationResult, dryRun bool) {
	for _, note := range result.Notes {
		fmt.Printf("说明: %s\n", strings.TrimSpace(note))
	}
	if !result.Changed {
		fmt.Printf("跳过: %s (%s)\n", result.Path, result.Reason)
		return
	}
	if dryRun {
		fmt.Printf("[DRY-RUN] 将迁移 %s\n", result.Path)
		return
	}
	fmt.Printf("已迁移 %s (备份: %s)\n", result.Path, result.Backup)
}

// defaultBaseDir 返回当前用户目录下的默认 NeoCode 配置目录。
func defaultBaseDir() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Join("~", defaultNeoCodeDirName)
	}
	return filepath.Join(home, defaultNeoCodeDirName)
}
