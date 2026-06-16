package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"neo-code/internal/config"
	agentruntime "neo-code/internal/runtime"
	runtimehooks "neo-code/internal/runtime/hooks"
	hookfixture "neo-code/internal/runtime/hooks/fixture"
	agentsession "neo-code/internal/session"
)

const (
	hookExitLintFindings = 1
	hookExitSystemError  = 2
	hookExitHookBlocked  = 3
	hookExitHookFailed   = 4
)

type hookLintDiagnostic struct {
	Path     string
	Line     int
	Severity string
	Message  string
	Hint     string
}

type hookCandidate struct {
	Path   string
	Scope  string
	Source string
	Item   config.RuntimeHookItemConfig
}

type repoHookConfigFile struct {
	Hooks struct {
		Items []config.RuntimeHookItemConfig `yaml:"items"`
	} `yaml:"hooks"`
}

type userHookConfigFile struct {
	Runtime struct {
		Hooks struct {
			Items []config.RuntimeHookItemConfig `yaml:"items"`
		} `yaml:"hooks"`
	} `yaml:"runtime"`
}

type hookLintDocument struct {
	Items []hookLintItem
}

type hookLintItem struct {
	Line int
	Item config.RuntimeHookItemConfig
}

type hookTraceAggregate struct {
	HookID      string
	Count       int
	DurationMS  int64
	MaxDuration int64
}

// newHookCommand 创建 hooks CLI 子命令组。
func newHookCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Inspect and debug runtime hooks",
	}
	cmd.AddCommand(
		newHookLintCommand(),
		newHookDryRunCommand(),
		newHookTraceCommand(),
	)
	return cmd
}

// newHookLintCommand 创建 hook lint 子命令，并负责输出稳定诊断与退出码。
func newHookLintCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "lint [path]",
		Short: "Lint hook configuration files",
		Long:  "Examples:\n  neocode hook lint\n  neocode hook lint .neocode/hooks.yaml\n  neocode hook lint ~/.neocode/config.yaml",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workdir, _ := cmd.Flags().GetString("workdir")
			targets, err := resolveHookLintTargets(strings.TrimSpace(workdir), args)
			if err != nil {
				return newCommandExitError(hookExitSystemError, "%v", err)
			}
			diagnostics, err := lintHookTargets(targets)
			if err != nil {
				return newCommandExitError(hookExitSystemError, "%v", err)
			}
			if len(diagnostics) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "hook lint passed")
				return nil
			}
			for _, diagnostic := range diagnostics {
				_, _ = fmt.Fprintf(
					cmd.OutOrStdout(),
					"%s:%d: %s: %s",
					diagnostic.Path,
					diagnostic.Line,
					diagnostic.Severity,
					diagnostic.Message,
				)
				if strings.TrimSpace(diagnostic.Hint) != "" {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), " (hint: %s)", diagnostic.Hint)
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout())
			}
			return newCommandExitError(hookExitLintFindings, "hook lint found %d issue(s)", len(diagnostics))
		},
	}
}

// newHookDryRunCommand 创建 hook dry-run 子命令，并执行单条 hook 的本地回放。
func newHookDryRunCommand() *cobra.Command {
	var hookID string
	var fixturePath string
	var repoScope bool
	command := &cobra.Command{
		Use:   "dry-run [path]",
		Short: "Execute one hook against a fixture payload",
		Long: "Examples:\n  neocode hook dry-run --hook warn-bash --fixture fixture.yaml\n" +
			"  neocode hook dry-run --hook repo-guard --fixture fixture.json --repo\n" +
			"  neocode hook dry-run .neocode/hooks.yaml --hook repo-guard --fixture fixture.json",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(hookID) == "" {
				return newCommandExitError(hookExitSystemError, "--hook is required")
			}
			if strings.TrimSpace(fixturePath) == "" {
				return newCommandExitError(hookExitSystemError, "--fixture is required")
			}
			workdir, _ := cmd.Flags().GetString("workdir")
			explicitTarget := ""
			if len(args) > 0 {
				explicitTarget = strings.TrimSpace(args[0])
			}
			candidate, err := resolveHookCandidate(strings.TrimSpace(workdir), hookID, repoScope, explicitTarget)
			if err != nil {
				return newCommandExitError(hookExitSystemError, "%v", err)
			}
			parsedFixture, err := hookfixture.ParseFile(fixturePath)
			if err != nil {
				return newCommandExitError(hookExitSystemError, "%v", err)
			}
			spec, err := buildHookSpecForCandidate(candidate, strings.TrimSpace(workdir))
			if err != nil {
				return newCommandExitError(hookExitSystemError, "%v", err)
			}
			if parsedFixture.Point != spec.Point {
				return newCommandExitError(
					hookExitSystemError,
					"fixture point %q does not match hook %q point %q",
					parsedFixture.Point,
					spec.ID,
					spec.Point,
				)
			}
			registry := runtimehooks.NewRegistry()
			if err := registry.Register(spec); err != nil {
				return newCommandExitError(hookExitSystemError, "%v", err)
			}
			executor := runtimehooks.NewExecutor(registry, nil, spec.Timeout)
			startedAt := time.Now()
			output := executor.Run(context.Background(), parsedFixture.Point, parsedFixture.Context)
			duration := time.Since(startedAt).Milliseconds()
			status := "pass"
			if output.Blocked {
				status = "block"
			} else if hasHookFailed(output) {
				status = "failed"
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "status: %s\n", status)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "block: %t\n", output.Blocked)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "duration_ms: %d\n", duration)
			for _, result := range output.Results {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "hook: %s status=%s\n", result.HookID, result.Status)
				if strings.TrimSpace(result.Message) != "" {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "message: %s\n", result.Message)
				}
				if len(result.Metadata.Annotations) > 0 {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "annotations: %s\n", strings.Join(result.Metadata.Annotations, " | "))
				}
			}
			switch status {
			case "pass":
				return nil
			case "block":
				return newCommandExitError(hookExitHookBlocked, "hook blocked")
			default:
				return newCommandExitError(hookExitHookFailed, "hook failed")
			}
		},
	}
	command.Flags().StringVar(&hookID, "hook", "", "hook id to execute")
	command.Flags().StringVar(&fixturePath, "fixture", "", "fixture yaml/json path")
	command.Flags().BoolVar(&repoScope, "repo", false, "resolve the hook from repo hooks instead of user hooks")
	return command
}

// newHookTraceCommand 创建 hook trace 子命令，并回放落盘的 runtime hook 事件。
func newHookTraceCommand() *cobra.Command {
	var runID string
	command := &cobra.Command{
		Use:   "trace",
		Short: "Read persisted hook trace events for one run",
		Long:  "Examples:\n  neocode hook trace --run-id run_123\n  neocode --workdir /repo hook trace --run-id run_456",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(runID) == "" {
				return newCommandExitError(hookExitSystemError, "--run-id is required")
			}
			workdir, _ := cmd.Flags().GetString("workdir")
			tracePath, err := resolveHookTracePath(strings.TrimSpace(workdir), runID)
			if err != nil {
				return newCommandExitError(hookExitSystemError, "%v", err)
			}
			records, err := readHookTraceRecords(tracePath)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return newCommandExitError(hookExitLintFindings, "trace not found for run %s", runID)
				}
				return newCommandExitError(hookExitSystemError, "%v", err)
			}
			for _, record := range records {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), formatHookTraceRecord(record))
			}
			aggregates := aggregateHookTraceRecords(records)
			if len(aggregates) > 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "summary:")
			}
			for _, aggregate := range aggregates {
				_, _ = fmt.Fprintf(
					cmd.OutOrStdout(),
					"%s count=%d duration_ms=%d max=%d %s\n",
					aggregate.HookID,
					aggregate.Count,
					aggregate.DurationMS,
					aggregate.MaxDuration,
					renderHookTraceHistogram(aggregate.DurationMS),
				)
			}
			return nil
		},
	}
	command.Flags().StringVar(&runID, "run-id", "", "runtime run id to replay")
	return command
}

// resolveHookLintTargets 解析 lint 的显式路径或默认扫描路径。
func resolveHookLintTargets(workdir string, args []string) ([]string, error) {
	if len(args) > 0 {
		target, err := filepath.Abs(strings.TrimSpace(args[0]))
		if err != nil {
			return nil, err
		}
		return []string{target}, nil
	}
	defaults := make([]string, 0, 2)
	loader := config.NewLoader("", config.StaticDefaults())
	defaults = append(defaults, loader.ConfigPath())
	resolvedWorkdir, err := resolveHookWorkspace(workdir)
	if err != nil {
		return nil, err
	}
	defaults = append(defaults, filepath.Join(resolvedWorkdir, ".neocode", "hooks.yaml"))
	return defaults, nil
}

// lintHookTargets 逐个校验目标文件中的 hook 配置，并收敛稳定输出顺序。
func lintHookTargets(targets []string) ([]hookLintDiagnostic, error) {
	var diagnostics []hookLintDiagnostic
	for _, target := range targets {
		info, err := os.Stat(target)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if info.IsDir() {
			return nil, fmt.Errorf("hook lint target is a directory: %s", target)
		}
		document, scope, err := parseHookLintDocument(target)
		if err != nil {
			return nil, err
		}
		for _, item := range document.Items {
			if err := validateLintItem(item.Item, scope); err != nil {
				diagnostics = append(diagnostics, hookLintDiagnostic{
					Path:     target,
					Line:     item.Line,
					Severity: "error",
					Message:  err.Error(),
					Hint:     hookLintHint(err.Error()),
				})
			}
		}
	}
	sort.Slice(diagnostics, func(i, j int) bool {
		if diagnostics[i].Path != diagnostics[j].Path {
			return diagnostics[i].Path < diagnostics[j].Path
		}
		return diagnostics[i].Line < diagnostics[j].Line
	})
	return diagnostics, nil
}

// parseHookLintDocument 读取目标 hook 配置，并补齐 item 的起始行号信息。
func parseHookLintDocument(path string) (hookLintDocument, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return hookLintDocument{}, "", err
	}
	var root yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(false)
	if err := decoder.Decode(&root); err != nil {
		return hookLintDocument{}, "", fmt.Errorf("parse hook yaml %s: %w", path, err)
	}
	scope := "repo"
	var items []config.RuntimeHookItemConfig
	if strings.EqualFold(filepath.Base(path), "config.yaml") {
		scope = "user"
		items, err = loadUserHookItems(path)
		if err != nil {
			return hookLintDocument{}, "", err
		}
	} else {
		items, err = loadRepoHookItemsForCLI(path)
		if err != nil {
			return hookLintDocument{}, "", err
		}
	}
	lines := collectHookItemLines(&root, scope)
	document := hookLintDocument{
		Items: make([]hookLintItem, 0, len(items)),
	}
	for index, item := range items {
		line := 1
		if index < len(lines) && lines[index] > 0 {
			line = lines[index]
		}
		document.Items = append(document.Items, hookLintItem{
			Line: line,
			Item: item.Clone(),
		})
	}
	return document, scope, nil
}

// collectHookItemLines 从 YAML AST 中提取 hooks.items 每一项的首行位置。
func collectHookItemLines(root *yaml.Node, scope string) []int {
	if root == nil || len(root.Content) == 0 {
		return nil
	}
	node := root.Content[0]
	switch scope {
	case "user":
		node = findMappingValue(findMappingValue(node, "runtime"), "hooks")
	default:
		node = findMappingValue(node, "hooks")
	}
	itemsNode := findMappingValue(node, "items")
	if itemsNode == nil || itemsNode.Kind != yaml.SequenceNode {
		return nil
	}
	lines := make([]int, 0, len(itemsNode.Content))
	for _, itemNode := range itemsNode.Content {
		lines = append(lines, itemNode.Line)
	}
	return lines
}

// findMappingValue 在 YAML mapping 节点中查找指定 key 对应的 value 节点。
func findMappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		if strings.EqualFold(strings.TrimSpace(node.Content[index].Value), strings.TrimSpace(key)) {
			return node.Content[index+1]
		}
	}
	return nil
}

// validateLintItem 复用现有配置真源，对单条 user/repo hook 执行语义校验。
func validateLintItem(item config.RuntimeHookItemConfig, scope string) error {
	defaults := config.StaticDefaults().Runtime.Hooks
	clone := item.Clone()
	if scope == "repo" {
		agentruntime.ApplyRepoHookItemDefaults(&clone, defaults)
		return agentruntime.ValidateRepoHookItem(clone)
	}
	clone.ApplyDefaults(defaults)
	return clone.Validate(defaults.DefaultFailurePolicy)
}

// hookLintHint 将底层校验错误映射为面向 CLI 的修复提示。
func hookLintHint(message string) string {
	lower := strings.ToLower(strings.TrimSpace(message))
	switch {
	case strings.Contains(lower, "requires match"):
		return "add a supported match section for this hook point"
	case strings.Contains(lower, "unknown field"):
		return "remove unsupported matcher fields and keep only tool_name/tool_name_regex/arguments_contains"
	case strings.Contains(lower, "invalid regex"):
		return "fix the regular expression syntax in match.tool_name_regex"
	case strings.Contains(lower, "loopback only"):
		return "use localhost or a loopback IP for http observe"
	case strings.Contains(lower, "failure_policy"):
		return "change failure_policy to warn_only or fail_open for this hook kind"
	case strings.Contains(lower, "params.command"):
		return "use an argv array or string command with shell=true"
	case strings.Contains(lower, "point"):
		return "pick a supported hook point allowed for this hook scope"
	default:
		return "fix the hook item so it matches current runtime hook schema"
	}
}

// resolveHookCandidate 按默认扫描顺序定位待执行 hook，并处理 user/repo 同名优先级。
func resolveHookCandidate(workdir string, hookID string, repoOnly bool, explicitTarget string) (hookCandidate, error) {
	candidates, err := loadHookCandidates(workdir, explicitTarget)
	if err != nil {
		return hookCandidate{}, err
	}
	var userMatch *hookCandidate
	var repoMatch *hookCandidate
	for index := range candidates {
		candidate := candidates[index]
		if !strings.EqualFold(strings.TrimSpace(candidate.Item.ID), strings.TrimSpace(hookID)) {
			continue
		}
		if candidate.Scope == "user" && userMatch == nil {
			userMatch = &candidate
		}
		if candidate.Scope == "repo" && repoMatch == nil {
			repoMatch = &candidate
		}
	}
	if repoOnly {
		if repoMatch == nil {
			return hookCandidate{}, fmt.Errorf("repo hook %q not found", hookID)
		}
		return *repoMatch, nil
	}
	if userMatch != nil {
		return *userMatch, nil
	}
	if repoMatch != nil {
		return *repoMatch, nil
	}
	return hookCandidate{}, fmt.Errorf("hook %q not found", hookID)
}

// loadHookCandidates 读取默认扫描范围内的 user/repo hook 候选集合。
func loadHookCandidates(workdir string, explicitTarget string) ([]hookCandidate, error) {
	var (
		targets []string
		err     error
	)
	if strings.TrimSpace(explicitTarget) != "" {
		targets, err = resolveHookLintTargets(workdir, []string{explicitTarget})
	} else {
		targets, err = resolveHookLintTargets(workdir, nil)
	}
	if err != nil {
		return nil, err
	}
	var candidates []hookCandidate
	for _, target := range targets {
		if strings.EqualFold(filepath.Base(target), "config.yaml") {
			items, err := loadUserHookItems(target)
			if err != nil {
				if strings.TrimSpace(explicitTarget) == "" && os.IsNotExist(err) {
					continue
				}
				return nil, err
			}
			for _, item := range items {
				candidates = append(candidates, hookCandidate{
					Path:   target,
					Scope:  "user",
					Source: string(runtimehooks.HookSourceUser),
					Item:   item.Clone(),
				})
			}
			continue
		}
		items, err := loadRepoHookItemsForCLI(target)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, item := range items {
			candidates = append(candidates, hookCandidate{
				Path:   target,
				Scope:  "repo",
				Source: string(runtimehooks.HookSourceRepo),
				Item:   item.Clone(),
			})
		}
	}
	return candidates, nil
}

// loadUserHookItems 从 user config 文件中读取 runtime.hooks.items 列表。
func loadUserHookItems(path string) ([]config.RuntimeHookItemConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var parsed userHookConfigFile
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(false)
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return append([]config.RuntimeHookItemConfig(nil), parsed.Runtime.Hooks.Items...), nil
}

// loadRepoHookItemsForCLI 从 repo hooks 文件中读取 hooks.items 列表。
func loadRepoHookItemsForCLI(path string) ([]config.RuntimeHookItemConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var parsed repoHookConfigFile
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	defaults := config.StaticDefaults().Runtime.Hooks
	items := make([]config.RuntimeHookItemConfig, 0, len(parsed.Hooks.Items))
	for _, item := range parsed.Hooks.Items {
		clone := item.Clone()
		agentruntime.ApplyRepoHookItemDefaults(&clone, defaults)
		items = append(items, clone)
	}
	return items, nil
}

// buildHookSpecForCandidate 复用 runtime 的公共构建入口，把配置项编译成可执行 HookSpec。
func buildHookSpecForCandidate(candidate hookCandidate, workdir string) (runtimehooks.HookSpec, error) {
	defaultWorkdir := workdir
	if strings.TrimSpace(defaultWorkdir) == "" {
		defaultWorkdir = filepath.Dir(candidate.Path)
	}
	item := candidate.Item.Clone()
	defaults := config.StaticDefaults().Runtime.Hooks
	if candidate.Scope == "repo" {
		agentruntime.ApplyRepoHookItemDefaults(&item, defaults)
		return agentruntime.BuildRepoHookSpec(item, defaultWorkdir)
	}
	item.ApplyDefaults(defaults)
	return agentruntime.BuildUserHookSpec(item, defaultWorkdir)
}

// hasHookFailed 判断本次 dry-run 是否出现 failed 终态结果。
func hasHookFailed(output runtimehooks.RunOutput) bool {
	for _, result := range output.Results {
		if result.Status == runtimehooks.HookResultFailed {
			return true
		}
	}
	return false
}

// resolveHookTracePath 根据当前 workspace 与 run_id 解析 trace 文件绝对路径。
func resolveHookTracePath(workdir string, runID string) (string, error) {
	resolvedWorkdir, err := resolveHookWorkspace(workdir)
	if err != nil {
		return "", err
	}
	loader := config.NewLoader("", config.StaticDefaults())
	return agentruntime.HookTracePath(loader.BaseDir(), resolvedWorkdir, runID)
}

// resolveHookWorkspace 统一解析 hooks CLI 所使用的 workspace 根目录，优先取显式 --workdir，缺失时回退当前目录。
func resolveHookWorkspace(workdir string) (string, error) {
	candidate := strings.TrimSpace(workdir)
	if candidate == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		candidate = cwd
	}
	return agentsession.ResolveExistingDir(candidate)
}

// readHookTraceRecords 读取并排序 JSONL trace 记录，坏行会返回定位后的解码错误。
func readHookTraceRecords(path string) ([]agentruntime.HookTraceRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	records := make([]agentruntime.HookTraceRecord, 0, 16)
	lineNo := 0
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		lineNo++
		var record agentruntime.HookTraceRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("decode trace line %d: %w", lineNo, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].Timestamp.Before(records[j].Timestamp)
	})
	return records, nil
}

// aggregateHookTraceRecords 汇总每个 hook 的终态次数与耗时，供 trace summary 输出。
func aggregateHookTraceRecords(records []agentruntime.HookTraceRecord) []hookTraceAggregate {
	byHook := make(map[string]*hookTraceAggregate)
	for _, record := range records {
		if !isHookTraceTerminalRecord(record) {
			continue
		}
		hookID := strings.TrimSpace(record.HookID)
		if hookID == "" {
			continue
		}
		aggregate, ok := byHook[hookID]
		if !ok {
			aggregate = &hookTraceAggregate{HookID: hookID}
			byHook[hookID] = aggregate
		}
		aggregate.Count++
		if record.DurationMS > 0 {
			aggregate.DurationMS += record.DurationMS
			if record.DurationMS > aggregate.MaxDuration {
				aggregate.MaxDuration = record.DurationMS
			}
		}
	}
	out := make([]hookTraceAggregate, 0, len(byHook))
	for _, aggregate := range byHook {
		out = append(out, *aggregate)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].HookID < out[j].HookID
	})
	return out
}

// isHookTraceTerminalRecord 判断 trace 记录是否属于一次 hook 执行的终态事件。
func isHookTraceTerminalRecord(record agentruntime.HookTraceRecord) bool {
	switch strings.TrimSpace(record.EventType) {
	case string(agentruntime.EventHookFinished), string(agentruntime.EventHookFailed), string(agentruntime.EventHookBlocked):
		return true
	default:
		return false
	}
}

// formatHookTraceRecord 将单条 trace 记录渲染为稳定且易读的一行文本。
func formatHookTraceRecord(record agentruntime.HookTraceRecord) string {
	line := fmt.Sprintf(
		"%s %-14s hook=%s point=%s status=%s",
		record.Timestamp.Format(time.RFC3339Nano),
		record.EventType,
		strings.TrimSpace(record.HookID),
		strings.TrimSpace(record.Point),
		strings.TrimSpace(record.Status),
	)
	if message := strings.TrimSpace(record.Message); message != "" {
		line += " message=" + message
	}
	if errText := strings.TrimSpace(record.Error); errText != "" {
		line += " error=" + errText
	}
	if record.DurationMS > 0 {
		line += fmt.Sprintf(" duration_ms=%d", record.DurationMS)
	}
	return line
}

// renderHookTraceHistogram 根据累计耗时生成简单文本直方图。
func renderHookTraceHistogram(durationMS int64) string {
	if durationMS <= 0 {
		return ""
	}
	width := int(durationMS / 10)
	if width < 1 {
		width = 1
	}
	if width > 24 {
		width = 24
	}
	return strings.Repeat("#", width)
}
