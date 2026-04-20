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

	data, err := readFirstExistingFile(s.indexPaths(scope))
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

// SaveIndex 将索引写入指定分层下的 MEMO.md 文件，采用临时文件 + 原子替换策略。
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

	if err := s.migrateLegacyProjectScopeLocked(scope); err != nil {
		return err
	}

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

	data, err := readFirstExistingFile(s.topicPaths(scope, filename))
	if err != nil {
		return "", fmt.Errorf("memo: read topic %s: %w", filename, err)
	}
	return string(data), nil
}

// SaveTopic 将内容写入指定分层下的 topic 文件，采用临时文件 + 原子替换策略。
func (s *FileStore) SaveTopic(ctx context.Context, scope Scope, filename string, content string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateStorageScope(scope); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.migrateLegacyProjectScopeLocked(scope); err != nil {
		return err
	}

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

	if err := s.migrateLegacyProjectScopeLocked(scope); err != nil {
		return err
	}

	path := s.topicPath(scope, filename)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("memo: delete topic %s: %w", filename, err)
	}
	return nil
}

// ListTopics 列出指定分层下 topics 目录中的所有 .md 文件名。
func (s *FileStore) ListTopics(ctx context.Context, scope Scope) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateStorageScope(scope); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	seen := make(map[string]struct{})
	for _, dir := range s.topicsDirs(scope) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("memo: list topics: %w", err)
		}
		for _, name := range collectTopicNames(entries) {
			seen[name] = struct{}{}
		}
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

// readFirstExistingFile 按顺序读取候选路径，返回首个存在文件内容；若均不存在则返回 os.ErrNotExist。
func readFirstExistingFile(paths []string) ([]byte, error) {
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	return nil, os.ErrNotExist
}

// scopeDir 返回指定 memo 分层的根目录。
func (s *FileStore) scopeDir(scope Scope) string {
	if scope == ScopeUser {
		return filepath.Join(globalMemoDirectory(s.baseDir), string(scope))
	}
	return filepath.Join(projectMemoDirectory(s.baseDir, s.workspaceRoot), string(scope))
}

// scopeDirLegacy 返回旧版本 project scope 的根目录，仅用于兼容迁移。
func (s *FileStore) scopeDirLegacy(scope Scope) string {
	if scope == ScopeProject {
		return projectMemoDirectory(s.baseDir, s.workspaceRoot)
	}
	return ""
}

// indexPaths 返回读取索引时的候选路径，顺序为新路径优先、旧路径兜底。
func (s *FileStore) indexPaths(scope Scope) []string {
	paths := []string{filepath.Join(s.scopeDir(scope), memoFileName)}
	if legacy := s.scopeDirLegacy(scope); legacy != "" {
		paths = append(paths, filepath.Join(legacy, memoFileName))
	}
	return paths
}

// topicsDir 返回指定 memo 分层的 topics 目录。
func (s *FileStore) topicsDir(scope Scope) string {
	return filepath.Join(s.scopeDir(scope), topicsDirName)
}

// topicsDirs 返回读取 topics 时的候选目录，顺序为新路径优先、旧路径兜底。
func (s *FileStore) topicsDirs(scope Scope) []string {
	dirs := []string{s.topicsDir(scope)}
	if legacy := s.scopeDirLegacy(scope); legacy != "" {
		dirs = append(dirs, filepath.Join(legacy, topicsDirName))
	}
	return dirs
}

// topicPath 生成指定分层下 topic 文件的安全路径，防止目录穿越。
func (s *FileStore) topicPath(scope Scope, filename string) string {
	return filepath.Join(s.topicsDir(scope), filepath.Base(filename))
}

// topicPaths 返回读取 topic 时的候选路径，顺序为新路径优先、旧路径兜底。
func (s *FileStore) topicPaths(scope Scope, filename string) []string {
	base := filepath.Base(filename)
	paths := []string{filepath.Join(s.topicsDir(scope), base)}
	if legacy := s.scopeDirLegacy(scope); legacy != "" {
		paths = append(paths, filepath.Join(legacy, topicsDirName, base))
	}
	return paths
}

// migrateLegacyProjectScopeLocked 在首次写入前把旧版 project 目录迁移到新目录，避免历史数据不可见。
func (s *FileStore) migrateLegacyProjectScopeLocked(scope Scope) error {
	if scope != ScopeProject {
		return nil
	}

	legacyDir := s.scopeDirLegacy(scope)
	if legacyDir == "" {
		return nil
	}

	if err := os.MkdirAll(s.scopeDir(scope), 0o755); err != nil {
		return fmt.Errorf("memo: create scoped dir for migration: %w", err)
	}

	legacyMemo := filepath.Join(legacyDir, memoFileName)
	targetMemo := filepath.Join(s.scopeDir(scope), memoFileName)
	if err := moveFileIfDstMissing(legacyMemo, targetMemo); err != nil {
		return fmt.Errorf("memo: migrate legacy index: %w", err)
	}

	legacyTopics := filepath.Join(legacyDir, topicsDirName)
	legacyEntries, err := os.ReadDir(legacyTopics)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("memo: list legacy topics: %w", err)
	}
	if len(legacyEntries) == 0 {
		return nil
	}

	newTopics := s.topicsDir(scope)
	if err := os.MkdirAll(newTopics, 0o755); err != nil {
		return fmt.Errorf("memo: create scoped topics dir for migration: %w", err)
	}
	for _, entry := range legacyEntries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		oldPath := filepath.Join(legacyTopics, entry.Name())
		newPath := filepath.Join(newTopics, entry.Name())
		if err := moveFileIfDstMissing(oldPath, newPath); err != nil {
			return fmt.Errorf("memo: migrate legacy topic %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// moveFileIfDstMissing 在源文件存在且目标文件不存在时执行迁移重命名。
func moveFileIfDstMissing(src string, dst string) error {
	if _, err := os.Stat(src); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(src, dst)
}

// memoDirectory 根据工作区根目录计算记忆分桶目录，复用 session 包的工作区哈希。
// globalMemoDirectory 返回全局 memo 根目录，用于存放 user 层记忆。
func globalMemoDirectory(baseDir string) string {
	return filepath.Join(baseDir, memoDirName)
}

// projectMemoDirectory 根据 workspace 根目录计算 project 层 memo 根目录。
func projectMemoDirectory(baseDir string, workspaceRoot string) string {
	return filepath.Join(baseDir, "projects", agentsession.HashWorkspaceRoot(workspaceRoot), memoDirName)
}

// validateStorageScope 校验当前 scope 是否是允许落盘的 memo 分层。
func validateStorageScope(scope Scope) error {
	switch scope {
	case ScopeUser, ScopeProject:
		return nil
	default:
		return fmt.Errorf("memo: unsupported storage scope %q", scope)
	}
}
