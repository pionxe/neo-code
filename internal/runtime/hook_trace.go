package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	agentsession "neo-code/internal/session"
)

const hookTraceSafeRunIDAlphabet = "0123456789abcdef"

var hookTraceEventTypes = map[EventType]struct{}{
	EventHookStarted:  {},
	EventHookFinished: {},
	EventHookFailed:   {},
	EventHookBlocked:  {},
}

// HookTraceRecord 描述 hook trace JSONL 的单条固定结构记录。
type HookTraceRecord struct {
	EventType  string    `json:"event_type"`
	Timestamp  time.Time `json:"timestamp"`
	RunID      string    `json:"run_id"`
	SessionID  string    `json:"session_id,omitempty"`
	Turn       int       `json:"turn"`
	Phase      string    `json:"phase,omitempty"`
	HookID     string    `json:"hook_id,omitempty"`
	Point      string    `json:"point,omitempty"`
	Source     string    `json:"source,omitempty"`
	Kind       string    `json:"kind,omitempty"`
	Mode       string    `json:"mode,omitempty"`
	Status     string    `json:"status,omitempty"`
	Message    string    `json:"message,omitempty"`
	Error      string    `json:"error,omitempty"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	DurationMS int64     `json:"duration_ms,omitempty"`
}

// HookTraceRecorder 将 hook 相关 runtime 事件旁路持久化为 JSONL。
type HookTraceRecorder struct {
	baseDir       string
	workspaceRoot string

	mu      sync.Mutex
	writers map[string]*hookTraceWriter
}

type hookTraceWriter struct {
	file   *os.File
	writer *bufio.Writer
}

// NewHookTraceRecorder 创建 workspace 级 hook trace 记录器。
func NewHookTraceRecorder(baseDir string, workspaceRoot string) *HookTraceRecorder {
	return &HookTraceRecorder{
		baseDir:       strings.TrimSpace(baseDir),
		workspaceRoot: strings.TrimSpace(workspaceRoot),
		writers:       make(map[string]*hookTraceWriter),
	}
}

// RecordRuntimeEvent 将 hook 生命周期事件写入当前工作区的 trace 文件。
func (r *HookTraceRecorder) RecordRuntimeEvent(_ context.Context, event RuntimeEvent) {
	if r == nil {
		return
	}
	if _, ok := hookTraceEventTypes[event.Type]; !ok {
		return
	}
	record, ok := buildHookTraceRecord(event)
	if !ok {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	writer, err := r.ensureWriter(record.RunID)
	if err != nil {
		return
	}
	encoder := json.NewEncoder(writer.writer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(record); err != nil {
		return
	}
	_ = writer.writer.Flush()
}

// Close 关闭 recorder 持有的全部打开文件。
func (r *HookTraceRecorder) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	var firstErr error
	for runID, writer := range r.writers {
		if writer == nil {
			delete(r.writers, runID)
			continue
		}
		if writer.writer != nil {
			if err := writer.writer.Flush(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		if writer.file != nil {
			if err := writer.file.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		delete(r.writers, runID)
	}
	return firstErr
}

// HookTracePath 返回当前 workspace/run 对应的 trace 文件绝对路径。
func HookTracePath(baseDir string, workspaceRoot string, runID string) (string, error) {
	trimmedBaseDir := strings.TrimSpace(baseDir)
	trimmedWorkspace := strings.TrimSpace(workspaceRoot)
	trimmedRunID := strings.TrimSpace(runID)
	if trimmedBaseDir == "" {
		return "", fmt.Errorf("hook trace baseDir is empty")
	}
	if trimmedWorkspace == "" {
		return "", fmt.Errorf("hook trace workspace is empty")
	}
	if trimmedRunID == "" {
		return "", fmt.Errorf("hook trace run_id is empty")
	}
	return filepath.Join(
		trimmedBaseDir,
		"projects",
		agentsession.HashWorkspaceRoot(trimmedWorkspace),
		"hook-traces",
		escapeHookTraceRunID(trimmedRunID)+".jsonl",
	), nil
}

// ensureWriter 按 run_id 懒创建并缓存 JSONL writer，避免每条事件重复打开文件。
func (r *HookTraceRecorder) ensureWriter(runID string) (*hookTraceWriter, error) {
	if existing, ok := r.writers[runID]; ok && existing != nil {
		return existing, nil
	}
	path, err := HookTracePath(r.baseDir, r.workspaceRoot, runID)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	writer := &hookTraceWriter{
		file:   file,
		writer: bufio.NewWriter(file),
	}
	r.writers[runID] = writer
	return writer, nil
}

// buildHookTraceRecord 将 runtime 事件映射为固定结构的 hook trace 记录。
func buildHookTraceRecord(event RuntimeEvent) (HookTraceRecord, bool) {
	record := HookTraceRecord{
		EventType: string(event.Type),
		Timestamp: event.Timestamp.UTC(),
		RunID:     strings.TrimSpace(event.RunID),
		SessionID: strings.TrimSpace(event.SessionID),
		Turn:      event.Turn,
		Phase:     strings.TrimSpace(event.Phase),
	}
	if record.RunID == "" {
		return HookTraceRecord{}, false
	}
	switch payload := event.Payload.(type) {
	case HookEventPayload:
		record.HookID = strings.TrimSpace(payload.HookID)
		record.Point = strings.TrimSpace(payload.Point)
		record.Source = strings.TrimSpace(payload.Source)
		record.Kind = strings.TrimSpace(payload.Kind)
		record.Mode = strings.TrimSpace(payload.Mode)
		record.Status = strings.TrimSpace(payload.Status)
		record.Message = strings.TrimSpace(payload.Message)
		record.Error = strings.TrimSpace(payload.Error)
		record.StartedAt = payload.StartedAt.UTC()
		record.DurationMS = payload.DurationMS
	case HookBlockedPayload:
		record.HookID = strings.TrimSpace(payload.HookID)
		record.Point = strings.TrimSpace(payload.Point)
		record.Source = strings.TrimSpace(payload.Source)
		record.Status = "block"
		record.Message = strings.TrimSpace(payload.Reason)
	default:
		return HookTraceRecord{}, false
	}
	return record, true
}

// escapeHookTraceRunID 将任意 run_id 编码为仅包含安全文件名字节的稳定 token。
func escapeHookTraceRunID(runID string) string {
	trimmed := strings.TrimSpace(runID)
	if trimmed == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(trimmed))
	for index := 0; index < len(trimmed); index++ {
		value := trimmed[index]
		if isHookTraceSafeRunIDByte(value) {
			builder.WriteByte(value)
			continue
		}
		builder.WriteByte('~')
		builder.WriteByte(hookTraceSafeRunIDAlphabet[value>>4])
		builder.WriteByte(hookTraceSafeRunIDAlphabet[value&0x0f])
	}
	return builder.String()
}

// isHookTraceSafeRunIDByte 判断单个字节能否直接作为 trace 文件名的一部分。
func isHookTraceSafeRunIDByte(value byte) bool {
	switch {
	case value >= 'a' && value <= 'z':
		return true
	case value >= 'A' && value <= 'Z':
		return true
	case value >= '0' && value <= '9':
		return true
	case value == '-', value == '_':
		return true
	default:
		return false
	}
}
