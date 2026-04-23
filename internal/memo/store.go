package memo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	agentsession "neo-code/internal/session"
)

const (
	memoDirName   = "memo"
	topicsDirName = "topics"
	memoFileName  = "MEMO.md"
)

// cachedIndex 缓存已解析的索引及其文件修改时间，用于减少高频读取时的磁盘 I/O。
type cachedIndex struct {
	content *Index
	modTime time.Time
}

// FileStore 基于文件系统实现 Store 接口，采用工作区隔离的双层目录布局。
type FileStore struct {
	mu            sync.RWMutex
	baseDir       string
	workspaceRoot string

	cacheMu    sync.RWMutex
	indexCache map[Scope]*cachedIndex
}

// NewFileStore 创建 FileStore 实例，目录基于 baseDir 和 workspaceRoot 计算工作区隔离路径。
func NewFileStore(baseDir string, workspaceRoot string) *FileStore {
	return &FileStore{
		baseDir:       baseDir,
		workspaceRoot: workspaceRoot,
		indexCache:    make(map[Scope]*cachedIndex),
	}
}

// LoadIndex 加载指定分层下的 MEMO.md 索引文件并解析为 Index 结构，优先复用内存缓存。
func (s *FileStore) LoadIndex(ctx context.Context, scope Scope) (*Index, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateStorageScope(scope); err != nil {
		return nil, err
	}

	indexPath := filepath.Join(s.scopeDir(scope), memoFileName)
	var cachedModTime time.Time
	if info, statErr := os.Stat(indexPath); statErr == nil {
		cachedModTime = info.ModTime()
		s.cacheMu.RLock()
		if cached, ok := s.indexCache[scope]; ok && cachedModTime.Equal(cached.modTime) {
			result := cloneIndex(cached.content)
			s.cacheMu.RUnlock()
			return result, nil
		}
		s.cacheMu.RUnlock()
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(indexPath)
	if errors.Is(err, os.ErrNotExist) {
		return &Index{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("memo: read index: %w", err)
	}
	index, err := ParseIndex(string(data))
	if err != nil {
		return nil, err
	}
	cachedContent := cloneIndex(index)

	if !cachedModTime.IsZero() {
		s.cacheMu.Lock()
		s.indexCache[scope] = &cachedIndex{
			content: cachedContent,
			modTime: cachedModTime,
		}
		s.cacheMu.Unlock()
	} else if info, statErr := os.Stat(indexPath); statErr == nil {
		s.cacheMu.Lock()
		s.indexCache[scope] = &cachedIndex{
			content: cachedContent,
			modTime: info.ModTime(),
		}
		s.cacheMu.Unlock()
	}

	return cloneIndex(cachedContent), nil
}

// SaveIndex 将索引写入指定分层下的 MEMO.md 文件，采用临时文件加原子替换策略。
func (s *FileStore) SaveIndex(ctx context.Context, scope Scope, index *Index) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateStorageScope(scope); err != nil {
		return err
	}
	if index == nil {
		return errors.New("memo: index is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.scopeDir(scope)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("memo: create memo dir: %w", err)
	}

	target := filepath.Join(dir, memoFileName)
	temp := target + ".tmp"
	content := RenderIndex(index)

	if err := os.WriteFile(temp, []byte(content), 0o644); err != nil {
		return fmt.Errorf("memo: write temp index: %w", err)
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("memo: remove old index: %w", err)
	}
	if err := os.Rename(temp, target); err != nil {
		return fmt.Errorf("memo: commit index: %w", err)
	}

	s.cacheMu.Lock()
	s.indexCache[scope] = &cachedIndex{
		content: cloneIndex(index),
		modTime: cacheModTimeAfterSave(target),
	}
	s.cacheMu.Unlock()

	return nil
}

// cacheModTimeAfterSave 在原子写入后读取文件的真实 mtime，确保缓存与后续 LoadIndex 的 stat 比较一致。
func cacheModTimeAfterSave(target string) time.Time {
	if info, err := os.Stat(target); err == nil {
		return info.ModTime()
	}
	return time.Now()
}

// LoadTopic 读取指定分层下的 topic 文件完整内容。
func (s *FileStore) LoadTopic(ctx context.Context, scope Scope, filename string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := validateStorageScope(scope); err != nil {
		return "", err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.topicPath(scope, filename))
	if err != nil {
		return "", fmt.Errorf("memo: read topic %s: %w", filename, err)
	}
	return string(data), nil
}

// SaveTopic 将内容写入指定分层下的 topic 文件，采用临时文件加原子替换策略。
func (s *FileStore) SaveTopic(ctx context.Context, scope Scope, filename string, content string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateStorageScope(scope); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.topicsDir(scope)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("memo: create topics dir: %w", err)
	}

	path := s.topicPath(scope, filename)
	temp := path + ".tmp"
	if err := os.WriteFile(temp, []byte(content), 0o644); err != nil {
		return fmt.Errorf("memo: write temp topic: %w", err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("memo: remove old topic: %w", err)
	}
	if err := os.Rename(temp, path); err != nil {
		return fmt.Errorf("memo: commit topic: %w", err)
	}
	return nil
}

// DeleteTopic 删除指定分层下的 topic 文件。
func (s *FileStore) DeleteTopic(ctx context.Context, scope Scope, filename string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateStorageScope(scope); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.topicPath(scope, filename)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("memo: delete topic %s: %w", filename, err)
	}
	return nil
}

// ListTopics 列出指定分层 topics 目录中的全部 .md 文件名。
func (s *FileStore) ListTopics(ctx context.Context, scope Scope) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateStorageScope(scope); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.topicsDir(scope))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("memo: list topics: %w", err)
	}

	seen := make(map[string]struct{})
	for _, name := range collectTopicNames(entries) {
		seen[name] = struct{}{}
	}
	if len(seen) == 0 {
		return nil, nil
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// collectTopicNames 将目录项过滤为 topic 文件名列表。
func collectTopicNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		names = append(names, entry.Name())
	}
	return names
}

// scopeDir 返回指定 memo 分层的根目录。
func (s *FileStore) scopeDir(scope Scope) string {
	if scope == ScopeUser {
		return filepath.Join(globalMemoDirectory(s.baseDir), string(scope))
	}
	return filepath.Join(projectMemoDirectory(s.baseDir, s.workspaceRoot), string(scope))
}

// topicsDir 返回指定 memo 分层的 topics 目录。
func (s *FileStore) topicsDir(scope Scope) string {
	return filepath.Join(s.scopeDir(scope), topicsDirName)
}

// topicPath 生成指定分层中 topic 文件的安全路径，防止目录穿越。
func (s *FileStore) topicPath(scope Scope, filename string) string {
	return filepath.Join(s.topicsDir(scope), filepath.Base(filename))
}

// globalMemoDirectory 返回全局 memo 根目录，用于存放 user 层记忆。
func globalMemoDirectory(baseDir string) string {
	return filepath.Join(baseDir, memoDirName)
}

// projectMemoDirectory 根据 workspace 根目录计算 project 层 memo 根目录。
func projectMemoDirectory(baseDir string, workspaceRoot string) string {
	return filepath.Join(baseDir, "projects", agentsession.HashWorkspaceRoot(workspaceRoot), memoDirName)
}

// validateStorageScope 校验当前 scope 是否允许落盘。
func validateStorageScope(scope Scope) error {
	switch scope {
	case ScopeUser, ScopeProject:
		return nil
	default:
		return fmt.Errorf("memo: unsupported storage scope %q", scope)
	}
}
