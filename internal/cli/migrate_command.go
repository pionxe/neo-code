package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"neo-code/internal/config"
)

type migrateContextBudgetOptions struct {
	ConfigPath string
	DryRun     bool
}

// newMigrateCommand 构建一次性迁移命令集合，命令可手动触发，启动 preflight 也会自动执行迁移。
func newMigrateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "migrate",
		Short:        "Run one-time local data migrations",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
	}
	cmd.AddCommand(newMigrateContextBudgetCommand())
	return cmd
}

// newMigrateContextBudgetCommand 构建 context.auto_compact 到 context.budget 的显式迁移命令。
func newMigrateContextBudgetCommand() *cobra.Command {
	options := &migrateContextBudgetOptions{}
	cmd := &cobra.Command{
		Use:          "context-budget",
		Short:        "Migrate context.auto_compact to context.budget",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		Annotations: map[string]string{
			commandAnnotationSkipGlobalPreload:     "true",
			commandAnnotationSkipSilentUpdateCheck: "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := config.MigrateContextBudgetConfigFile(strings.TrimSpace(options.ConfigPath), options.DryRun)
			if err != nil {
				return err
			}
			printContextBudgetMigrationResult(cmd, result, options.DryRun)
			return nil
		},
	}
	cmd.Flags().StringVar(&options.ConfigPath, "config", "", "config.yaml path (default ~/.neocode/config.yaml)")
	cmd.Flags().BoolVar(&options.DryRun, "dry-run", false, "check migration without writing files")
	return cmd
}

// printContextBudgetMigrationResult 输出迁移结果，确保 dry-run 和真实写入提示保持一致。
func printContextBudgetMigrationResult(cmd *cobra.Command, result config.ContextBudgetMigrationResult, dryRun bool) {
	writer := cmd.OutOrStdout()
	for _, note := range result.Notes {
		_, _ = fmt.Fprintf(writer, "说明: %s\n", strings.TrimSpace(note))
	}
	if !result.Changed {
		_, _ = fmt.Fprintf(writer, "跳过: %s (%s)\n", result.Path, result.Reason)
		return
	}
	if dryRun {
		_, _ = fmt.Fprintf(writer, "[DRY-RUN] 将迁移 %s\n", result.Path)
		return
	}
	_, _ = fmt.Fprintf(writer, "已迁移 %s (备份: %s)\n", result.Path, result.Backup)
}
