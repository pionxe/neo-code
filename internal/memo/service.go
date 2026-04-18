package memo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"neo-code/internal/config"
)

// Service 编排记忆的存储、检索、删除和索引维护，是 memo 子系统对外的统一入口。
type Service struct {
	store                  Store
	config                 config.MemoConfig
	mu                     sync.Mutex
	sourceInvl             func()
	autoExtractIndexMu     sync.Mutex
	autoExtractIndexReady  bool
	autoExtractKeysByTopic map[string]string
	autoExtractKeyRefs     map[string]int
}

// NewService 创建 memo Service 实例。
func NewService(store Store, cfg config.MemoConfig, sourceInvl func()) *Service {
	return &Service{
		store:      store,
		config:     cfg,
		sourceInvl: sourceInvl,
	}
}

// Add 添加一条记忆并持久化到对应分层的索引与 topic 文件。
func (s *Service) Add(ctx context.Context, entry Entry) error {
	entry, err := normalizeEntryForPersist(entry)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.saveEntryLocked(ctx, entry)
}

// addAutoExtractIfAbsent 在同一把锁内完成自动提取条目的查重与写入。
func (s *Service) addAutoExtractIfAbsent(ctx context.Context, entry Entry) (bool, error) {
	entry, err := normalizeEntryForPersist(entry)
	if err != nil {
		return false, err
	}
	if err := s.ensureAutoExtractIndex(ctx); err != nil {
		return false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.hasExactAutoExtractLocked(entry) {
		return false, nil
	}
	if err := s.saveEntryLocked(ctx, entry); err != nil {
		return false, err
	}
	return true, nil
}

// normalizeKeyword 统一关键词的空格与大小写处理。
func normalizeKeyword(keyword string) string {
	return strings.ToLower(strings.TrimSpace(keyword))
}

// Remove 按关键词删除匹配的记忆条目，支持按 scope 过滤。
func (s *Service) Remove(ctx context.Context, keyword string, scope Scope) (int, error) {
	keyword = normalizeKeyword(keyword)
	if keyword == "" {
		return 0, fmt.Errorf("memo: keyword is empty")
	}
	if err := validateQueryScope(scope); err != nil {
		return 0, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	removed := 0
	for _, bucket := range scopesForQuery(scope) {
		index, err := s.loadIndexLocked(ctx, bucket)
		if err != nil {
			return 0, err
		}

		remaining := make([]Entry, 0, len(index.Entries))
		removedEntries := make([]Entry, 0, len(index.Entries))
		for _, entry := range index.Entries {
			if matchesKeyword(entry, keyword) {
				removedEntries = append(removedEntries, entry)
				continue
			}
			remaining = append(remaining, entry)
		}
		if len(removedEntries) == 0 {
			continue
		}

		index.Entries = remaining
		index.UpdatedAt = time.Now()
		if err := s.store.SaveIndex(ctx, bucket, index); err != nil {
			return removed, fmt.Errorf("memo: save index: %w", err)
		}
		for _, entry := range removedEntries {
			if topicFile := strings.TrimSpace(entry.TopicFile); topicFile != "" {
				_ = s.store.DeleteTopic(ctx, bucket, topicFile)
				s.removeAutoExtractTopicLocked(bucket, topicFile)
			}
		}
		removed += len(removedEntries)
	}

	if removed > 0 {
		s.invalidateCache()
	}
	return removed, nil
}

// List 返回 scope 范围内的记忆条目浅拷贝。
func (s *Service) List(ctx context.Context, scope Scope) ([]Entry, error) {
	if err := validateQueryScope(scope); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.listLocked(ctx, scope)
}

// Search 按关键词搜索记忆条目，支持按 scope 过滤。
func (s *Service) Search(ctx context.Context, keyword string, scope Scope) ([]Entry, error) {
	if err := validateQueryScope(scope); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.listLocked(ctx, scope)
	if err != nil {
		return nil, err
	}
	keyword = normalizeKeyword(keyword)
	if keyword == "" {
		return entries, nil
	}

	results := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if matchesKeyword(entry, keyword) {
			results = append(results, entry)
		}
	}
	return results, nil
}

// Recall 加载匹配关键词的 topic 文件内容，支持按 scope 过滤。
func (s *Service) Recall(ctx context.Context, keyword string, scope Scope) ([]RecalledEntry, error) {
	if err := validateQueryScope(scope); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	keyword = normalizeKeyword(keyword)
	results := make([]RecalledEntry, 0)
	for _, bucket := range scopesForQuery(scope) {
		index, err := s.loadIndexLocked(ctx, bucket)
		if err != nil {
			return nil, err
		}
		for _, entry := range index.Entries {
			if !matchesKeyword(entry, keyword) {
				continue
			}
			if strings.TrimSpace(entry.TopicFile) == "" {
				continue
			}
			content, err := s.store.LoadTopic(ctx, bucket, entry.TopicFile)
			if err != nil {
				continue
			}
			results = append(results, RecalledEntry{
				Scope:   bucket,
				Entry:   entry,
				Content: content,
			})
		}
	}
	return results, nil
}

// saveEntryLocked 在持有 Service 锁的前提下持久化单条记忆及索引。
func (s *Service) saveEntryLocked(ctx context.Context, entry Entry) error {
	scope := ScopeForType(entry.Type)
	now := time.Now()
	if entry.ID == "" {
		entry.ID = newEntryID(entry.Type)
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	entry.UpdatedAt = now
	if entry.TopicFile == "" {
		entry.TopicFile = fmt.Sprintf("%s_%s.md", entry.Type, entry.ID)
	}

	index, err := s.loadIndexLocked(ctx, scope)
	if err != nil {
		return err
	}
	working := cloneIndex(index)

	replaced := false
	var previous Entry
	for i, existing := range working.Entries {
		if existing.ID == entry.ID {
			previous = existing
			working.Entries[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		working.Entries = append(working.Entries, entry)
	}
	working.UpdatedAt = now

	removedEntries := trimIndexEntries(working, s.config.MaxEntries, s.config.MaxIndexBytes)
	if err := s.store.SaveTopic(ctx, scope, entry.TopicFile, RenderTopic(&entry)); err != nil {
		return fmt.Errorf("memo: save topic: %w", err)
	}
	if err := s.store.SaveIndex(ctx, scope, working); err != nil {
		if !replaced {
			_ = s.store.DeleteTopic(ctx, scope, entry.TopicFile)
		}
		return fmt.Errorf("memo: save index: %w", err)
	}

	if replaced && previous.TopicFile != "" && previous.TopicFile != entry.TopicFile {
		_ = s.store.DeleteTopic(ctx, scope, previous.TopicFile)
	}
	for _, removed := range removedEntries {
		if topicFile := strings.TrimSpace(removed.TopicFile); topicFile != "" {
			_ = s.store.DeleteTopic(ctx, scope, topicFile)
		}
	}

	if s.autoExtractIndexReady {
		if replaced {
			s.removeAutoExtractTopicLocked(scope, previous.TopicFile)
		}
		for _, removed := range removedEntries {
			s.removeAutoExtractTopicLocked(scope, removed.TopicFile)
		}
		if indexContainsEntryID(working, entry.ID) {
			s.trackAutoExtractEntryLocked(scope, entry)
		}
	}

	s.invalidateCache()
	return nil
}

// ensureAutoExtractIndex 在锁外预加载自动提取的精确去重索引，避免重复在主锁内扫 topic 文件。
func (s *Service) ensureAutoExtractIndex(ctx context.Context) error {
	s.mu.Lock()
	if s.autoExtractIndexReady {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	s.autoExtractIndexMu.Lock()
	defer s.autoExtractIndexMu.Unlock()

	s.mu.Lock()
	if s.autoExtractIndexReady {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	keysByTopic := make(map[string]string)
	keyRefs := make(map[string]int)
	for _, scope := range supportedStorageScopes() {
		index, err := s.store.LoadIndex(ctx, scope)
		if err != nil {
			return fmt.Errorf("memo: load index: %w", err)
		}
		for _, entry := range index.Entries {
			topicFile := strings.TrimSpace(entry.TopicFile)
			if topicFile == "" {
				continue
			}
			topicContent, err := s.store.LoadTopic(ctx, scope, topicFile)
			if err != nil {
				continue
			}
			source, content := parseTopicSourceAndContent(topicContent)
			if source != SourceAutoExtract {
				continue
			}
			entry.Source = source
			entry.Content = content
			key := autoExtractDedupKey(entry)
			if key == "" {
				continue
			}
			topicKey := scopedTopicKey(scope, topicFile)
			keysByTopic[topicKey] = key
			keyRefs[key]++
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.autoExtractIndexReady {
		return nil
	}
	s.autoExtractKeysByTopic = keysByTopic
	s.autoExtractKeyRefs = keyRefs
	s.autoExtractIndexReady = true
	return nil
}

// hasExactAutoExtractLocked 检查是否已存在完全相同的自动提取记忆。
func (s *Service) hasExactAutoExtractLocked(target Entry) bool {
	targetKey := autoExtractDedupKey(target)
	if targetKey == "" {
		return false
	}
	return s.autoExtractKeyRefs[targetKey] > 0
}

// trackAutoExtractEntryLocked 在自动提取索引已就绪时维护单条条目的去重键。
func (s *Service) trackAutoExtractEntryLocked(scope Scope, entry Entry) {
	if !s.autoExtractIndexReady {
		return
	}
	topicFile := strings.TrimSpace(entry.TopicFile)
	if topicFile == "" {
		return
	}
	s.removeAutoExtractTopicLocked(scope, topicFile)
	if entry.Source != SourceAutoExtract {
		return
	}
	key := autoExtractDedupKey(entry)
	if key == "" {
		return
	}
	s.autoExtractKeysByTopic[scopedTopicKey(scope, topicFile)] = key
	s.autoExtractKeyRefs[key]++
}

// removeAutoExtractTopicLocked 从精确去重索引中移除指定 topic 的记录。
func (s *Service) removeAutoExtractTopicLocked(scope Scope, topicFile string) {
	if !s.autoExtractIndexReady {
		return
	}
	topicFile = strings.TrimSpace(topicFile)
	if topicFile == "" {
		return
	}

	topicKey := scopedTopicKey(scope, topicFile)
	key, ok := s.autoExtractKeysByTopic[topicKey]
	if !ok {
		return
	}
	delete(s.autoExtractKeysByTopic, topicKey)
	if refs := s.autoExtractKeyRefs[key]; refs > 1 {
		s.autoExtractKeyRefs[key] = refs - 1
		return
	}
	delete(s.autoExtractKeyRefs, key)
}

// loadIndexLocked 在持有锁的状态下加载指定分层索引。
func (s *Service) loadIndexLocked(ctx context.Context, scope Scope) (*Index, error) {
	index, err := s.store.LoadIndex(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("memo: load index: %w", err)
	}
	return index, nil
}

// listLocked 返回 scope 范围内的所有条目。
func (s *Service) listLocked(ctx context.Context, scope Scope) ([]Entry, error) {
	results := make([]Entry, 0)
	for _, bucket := range scopesForQuery(scope) {
		index, err := s.loadIndexLocked(ctx, bucket)
		if err != nil {
			return nil, err
		}
		results = append(results, index.Entries...)
	}
	return append([]Entry(nil), results...), nil
}

// invalidateCache 触发上下文源缓存失效回调。
func (s *Service) invalidateCache() {
	if s.sourceInvl != nil {
		s.sourceInvl()
	}
}

// matchesKeyword 检查条目是否匹配关键词。
func matchesKeyword(entry Entry, keyword string) bool {
	if keyword == "" {
		return true
	}
	if strings.Contains(strings.ToLower(entry.Title), keyword) {
		return true
	}
	if strings.Contains(strings.ToLower(string(entry.Type)), keyword) {
		return true
	}
	if strings.Contains(strings.ToLower(entry.Content), keyword) {
		return true
	}
	for _, kw := range entry.Keywords {
		if strings.Contains(strings.ToLower(kw), keyword) {
			return true
		}
	}
	return false
}

// normalizeEntryForPersist 统一校验并标准化写入前的记忆条目。
func normalizeEntryForPersist(entry Entry) (Entry, error) {
	if !IsValidType(entry.Type) {
		return Entry{}, fmt.Errorf("memo: invalid type %q", entry.Type)
	}
	entry.Title = NormalizeTitle(entry.Title)
	if entry.Title == "" {
		return Entry{}, fmt.Errorf("memo: title is empty")
	}
	entry.Content = strings.TrimSpace(entry.Content)
	if entry.Content == "" {
		return Entry{}, fmt.Errorf("memo: content is empty")
	}
	entry.Keywords = normalizeKeywords(entry.Keywords)
	return entry, nil
}

// newEntryID 生成格式为 <type>_<timestamp_hex>_<random_hex> 的唯一 ID。
func newEntryID(t Type) string {
	ts := fmt.Sprintf("%x", time.Now().Unix())
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	return fmt.Sprintf("%s_%s_%s", t, ts, hex.EncodeToString(buf))
}

// cloneIndex 复制索引结构，避免持久化失败时污染原始数据引用。
func cloneIndex(index *Index) *Index {
	if index == nil {
		return &Index{}
	}
	cloned := &Index{
		Entries:   make([]Entry, len(index.Entries)),
		UpdatedAt: index.UpdatedAt,
	}
	copy(cloned.Entries, index.Entries)
	return cloned
}

// trimIndexEntries 先按条目数、再按索引字节数裁剪最旧条目，并返回被删除的记录。
func trimIndexEntries(index *Index, maxEntries int, maxIndexBytes int) []Entry {
	if index == nil {
		return nil
	}
	removed := make([]Entry, 0)
	for maxEntries > 0 && len(index.Entries) > maxEntries {
		removed = append(removed, index.Entries[0])
		index.Entries = index.Entries[1:]
	}
	if maxIndexBytes > 0 && len(index.Entries) > 0 {
		removed = append(removed, trimIndexEntriesByBytes(index, maxIndexBytes)...)
	}
	return removed
}

// trimIndexEntriesByBytes 在索引超过字节阈值时，通过二分定位最小移除数量并返回被删除条目。
func trimIndexEntriesByBytes(index *Index, maxIndexBytes int) []Entry {
	if index == nil || len(index.Entries) == 0 || maxIndexBytes <= 0 {
		return nil
	}
	if len(RenderIndex(index)) <= maxIndexBytes {
		return nil
	}

	entries := index.Entries
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := lo + (hi-lo)/2
		candidate := &Index{Entries: entries[mid:], UpdatedAt: index.UpdatedAt}
		if len(RenderIndex(candidate)) > maxIndexBytes {
			lo = mid + 1
			continue
		}
		hi = mid
	}

	removed := append([]Entry(nil), entries[:lo]...)
	index.Entries = entries[lo:]
	return removed
}

// indexContainsEntryID 判断索引中是否仍保留目标 ID，用于避免为已裁剪条目建立去重索引。
func indexContainsEntryID(index *Index, entryID string) bool {
	if index == nil || strings.TrimSpace(entryID) == "" {
		return false
	}
	for _, item := range index.Entries {
		if item.ID == entryID {
			return true
		}
	}
	return false
}

// scopesForQuery 将查询范围展开为实际存储分层列表。
func scopesForQuery(scope Scope) []Scope {
	switch NormalizeScope(scope) {
	case ScopeUser:
		return []Scope{ScopeUser}
	case ScopeProject:
		return []Scope{ScopeProject}
	default:
		return supportedStorageScopes()
	}
}

// validateQueryScope 校验外部查询/删除接口允许的 scope 取值。
func validateQueryScope(scope Scope) error {
	normalized := strings.ToLower(strings.TrimSpace(string(scope)))
	switch Scope(normalized) {
	case "", ScopeAll, ScopeUser, ScopeProject:
		return nil
	default:
		return fmt.Errorf("memo: unsupported scope %q", scope)
	}
}

// scopedTopicKey 为自动提取去重索引生成稳定的 topic 维度键。
func scopedTopicKey(scope Scope, topicFile string) string {
	return string(scope) + ":" + strings.TrimSpace(topicFile)
}
