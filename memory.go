package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// newUUID 生成 v4 UUID（无外部依赖）
func newUUID() string {
	u := make([]byte, 16)
	rand.Read(u)
	u[6] = (u[6] & 0x0f) | 0x40 // version 4
	u[8] = (u[8] & 0x3f) | 0x80 // variant 10
	return hex.EncodeToString(u[:4]) + "-" +
		hex.EncodeToString(u[4:6]) + "-" +
		hex.EncodeToString(u[6:8]) + "-" +
		hex.EncodeToString(u[8:10]) + "-" +
		hex.EncodeToString(u[10:])
}

// ============================================
// 记忆系统 — 青羽的灵魂载体
// 结构化存储、多维度检索、重要性分级、自动衰减
// ============================================

// 重要性阈值
const (
	ImportanceCore   = 8 // >= 8: 核心记忆，永久保留
	ImportanceNormal = 5 // >= 5: 正常记忆
	ImportanceLow    = 3 // >= 3: 低优先级
	ImportanceMin    = 1 // < 1: 可删除

	DecayDaysLevel1  = 7   // 7 天未访问 → importance -1
	DecayDaysLevel2  = 30  // 30 天未访问 → importance -1
	DecayDaysLevel3  = 90  // 90 天未访问 → importance -1
	DecayDaysArchive = 180 // 180 天未访问 → 强制归档
)

// MemoryEntry 单条记忆条目
type MemoryEntry struct {
	ID          string   `json:"id"`           // UUID
	Topic       string   `json:"topic"`        // 主题分类
	Content     string   `json:"content"`      // 记忆内容
	Importance  int      `json:"importance"`   // 1-10
	Tags        []string `json:"tags"`         // 标签
	Links       []string `json:"links"`        // 关联记忆 ID
	CreatedAt   int64    `json:"created_at"`   // 创建时间戳
	UpdatedAt   int64    `json:"updated_at"`   // 最后更新时间戳
	AccessCount int      `json:"access_count"` // 访问次数
	Version     int      `json:"version"`      // 版本号
}

// IndexEntry 索引条目（轻量级摘要）
type IndexEntry struct {
	ID         string   `json:"id"`
	Topic      string   `json:"topic"`
	Importance int      `json:"importance"`
	Tags       []string `json:"tags"`
	Links      []string `json:"links"`
	CreatedAt  int64    `json:"created_at"`
	UpdatedAt  int64    `json:"updated_at"`
	Version    int      `json:"version"`
	Summary    string   `json:"summary"` // 前 100 字摘要
}

// MemoryIndex 全局索引
type MemoryIndex struct {
	Version    int                 `json:"version"`     // 索引版本号
	UpdatedAt  int64               `json:"updated_at"`  // 最后更新时间
	Entries    []IndexEntry        `json:"entries"`     // 所有记忆条目摘要
	TagIndex   map[string][]string `json:"tag_index"`   // 标签 → ID 列表
	TopicIndex map[string][]string `json:"topic_index"` // 主题 → ID 列表
}

// SearchQuery 检索参数
type SearchQuery struct {
	Topic          string   // 精确主题
	Keyword        string   // 全文关键词
	Tags           []string // 标签过滤
	ImportanceMin  int      // 最低重要性
	ImportanceMax  int      // 最高重要性
	Limit          int      // 返回上限
	IncludeArchive bool     // 是否包含归档
	SortBy         string   // 排序: importance / time / access
}

// MemoryStats 记忆统计
type MemoryStats struct {
	TotalEntries  int            `json:"total_entries"`
	CoreEntries   int            `json:"core_entries"`
	ArchivedCount int            `json:"archived_count"`
	TagCount      int            `json:"tag_count"`
	TotalLinks    int            `json:"total_links"`
	RecentEntries []IndexEntry   `json:"recent_entries"`
	TopAccessed   []IndexEntry   `json:"top_accessed"`
	ByImportance  map[int]int    `json:"by_importance"`
	ByTopic       map[string]int `json:"by_topic"`
}

// MemoryStore 记忆存储引擎
type MemoryStore struct {
	mu      sync.RWMutex
	rootDir string
	index   *MemoryIndex
	dirty   bool // 索引是否有未保存的更改
}

// NewMemoryStore 创建记忆存储引擎
func NewMemoryStore() *MemoryStore {
	store := &MemoryStore{
		rootDir: filepath.Join(RootDir, MemoryDir),
		index: &MemoryIndex{
			Version:    1,
			UpdatedAt:  time.Now().Unix(),
			Entries:    []IndexEntry{},
			TagIndex:   make(map[string][]string),
			TopicIndex: make(map[string][]string),
		},
	}

	// 确保目录存在
	os.MkdirAll(store.rootDir, 0755)
	os.MkdirAll(filepath.Join(store.rootDir, "core"), 0755)
	os.MkdirAll(filepath.Join(store.rootDir, "archive"), 0755)
	os.MkdirAll(filepath.Join(store.rootDir, ".trash"), 0755)

	// 加载索引
	if err := store.loadIndex(); err != nil {
		// 索引不存在或损坏，重建
		store.rebuildIndex()
	}

	return store
}

// ============================================
// 索引管理
// ============================================

func (ms *MemoryStore) indexPath() string {
	return filepath.Join(ms.rootDir, "index.json")
}

func (ms *MemoryStore) entryPath(id string, importance int) string {
	if importance >= ImportanceCore {
		return filepath.Join(ms.rootDir, "core", id+".json")
	}
	return filepath.Join(ms.rootDir, id+".json")
}

func (ms *MemoryStore) archivePath(id string) string {
	return filepath.Join(ms.rootDir, "archive", id+".json")
}

func (ms *MemoryStore) trashPath(id string) string {
	return filepath.Join(ms.rootDir, ".trash", id+".json")
}

// loadIndex 从磁盘加载索引
func (ms *MemoryStore) loadIndex() error {
	data, err := os.ReadFile(ms.indexPath())
	if err != nil {
		return err
	}
	var idx MemoryIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return err
	}
	ms.index = &idx
	ms.dirty = false
	return nil
}

// saveIndex 保存索引到磁盘
func (ms *MemoryStore) saveIndex() error {
	ms.index.UpdatedAt = time.Now().Unix()
	data, err := json.MarshalIndent(ms.index, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(ms.indexPath(), data, 0644); err != nil {
		return err
	}
	ms.dirty = false
	return nil
}

// flush 在需要时写入磁盘
func (ms *MemoryStore) flush() error {
	if ms.dirty {
		return ms.saveIndex()
	}
	return nil
}

// rebuildIndex 从所有记忆文件重建索引
func (ms *MemoryStore) rebuildIndex() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	newIdx := &MemoryIndex{
		Version:    1,
		UpdatedAt:  time.Now().Unix(),
		Entries:    []IndexEntry{},
		TagIndex:   make(map[string][]string),
		TopicIndex: make(map[string][]string),
	}

	// 扫描主目录
	entries, err := os.ReadDir(ms.rootDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			if e.Name() == "index.json" || e.Name() == "creator.json" {
				continue
			}
			id := strings.TrimSuffix(e.Name(), ".json")
			entry, err := ms.loadEntryByID(id)
			if err != nil {
				continue
			}
			ms.addToIndex(newIdx, entry)
		}
	}

	// 扫描 core 目录
	coreEntries, err := os.ReadDir(filepath.Join(ms.rootDir, "core"))
	if err == nil {
		for _, e := range coreEntries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			id := strings.TrimSuffix(e.Name(), ".json")
			entry, err := ms.loadEntryByID(id)
			if err != nil {
				continue
			}
			ms.addToIndex(newIdx, entry)
		}
	}

	ms.index = newIdx
	ms.dirty = true
	return ms.flush()
}

// addToIndex 将条目添加到索引中
func (ms *MemoryStore) addToIndex(idx *MemoryIndex, entry *MemoryEntry) {
	summary := []rune(entry.Content)
	if len(summary) > 100 {
		summary = summary[:100]
	}

	ie := IndexEntry{
		ID:         entry.ID,
		Topic:      entry.Topic,
		Importance: entry.Importance,
		Tags:       entry.Tags,
		Links:      entry.Links,
		CreatedAt:  entry.CreatedAt,
		UpdatedAt:  entry.UpdatedAt,
		Version:    entry.Version,
		Summary:    string(summary),
	}

	idx.Entries = append(idx.Entries, ie)

	// 更新标签索引
	for _, tag := range entry.Tags {
		tagLower := strings.ToLower(tag)
		idx.TagIndex[tagLower] = append(idx.TagIndex[tagLower], entry.ID)
	}

	// 更新主题索引
	topicLower := strings.ToLower(entry.Topic)
	idx.TopicIndex[topicLower] = append(idx.TopicIndex[topicLower], entry.ID)
}

// updateIndex 更新单条索引
func (ms *MemoryStore) updateIndex(entry *MemoryEntry) {
	// 移除旧的索引条目
	for i, e := range ms.index.Entries {
		if e.ID == entry.ID {
			ms.index.Entries = append(ms.index.Entries[:i], ms.index.Entries[i+1:]...)
			break
		}
	}

	// 重建标签索引（全量重建）
	ms.rebuildTagIndex()

	// 添加新的
	ms.addToIndex(ms.index, entry)
	ms.dirty = true
}

// rebuildTagIndex 重建标签索引
func (ms *MemoryStore) rebuildTagIndex() {
	ms.index.TagIndex = make(map[string][]string)
	ms.index.TopicIndex = make(map[string][]string)
	for _, e := range ms.index.Entries {
		for _, tag := range e.Tags {
			tagLower := strings.ToLower(tag)
			ms.index.TagIndex[tagLower] = append(ms.index.TagIndex[tagLower], e.ID)
		}
		topicLower := strings.ToLower(e.Topic)
		ms.index.TopicIndex[topicLower] = append(ms.index.TopicIndex[topicLower], e.ID)
	}
}

// removeFromIndex 从索引中移除条目
func (ms *MemoryStore) removeFromIndex(id string) {
	for i, e := range ms.index.Entries {
		if e.ID == id {
			ms.index.Entries = append(ms.index.Entries[:i], ms.index.Entries[i+1:]...)
			break
		}
	}
	ms.rebuildTagIndex()
	ms.dirty = true
}

// ============================================
// 记忆条目 CRUD
// ============================================

// Save 保存记忆条目
func (ms *MemoryStore) Save(entry *MemoryEntry) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if entry.ID == "" {
		entry.ID = newUUID()
	}
	now := time.Now().Unix()
	if entry.CreatedAt == 0 {
		entry.CreatedAt = now
	}
	entry.UpdatedAt = now

	// 确定存储路径
	var path string
	if entry.Importance >= ImportanceCore {
		path = ms.entryPath(entry.ID, entry.Importance)
	} else {
		path = ms.entryPath(entry.ID, 0)
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化记忆失败: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("写入记忆文件失败: %v", err)
	}

	// 更新索引
	ms.updateIndex(entry)

	return ms.flush()
}

// Load 按 ID 加载记忆条目
func (ms *MemoryStore) Load(id string) (*MemoryEntry, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	return ms.loadEntryByID(id)
}

// loadEntryByID 内部加载（无锁）
func (ms *MemoryStore) loadEntryByID(id string) (*MemoryEntry, error) {
	// 尝试主目录
	path := filepath.Join(ms.rootDir, id+".json")
	if data, err := os.ReadFile(path); err == nil {
		var entry MemoryEntry
		if err := json.Unmarshal(data, &entry); err == nil {
			return &entry, nil
		}
	}

	// 尝试 core 目录
	path = filepath.Join(ms.rootDir, "core", id+".json")
	if data, err := os.ReadFile(path); err == nil {
		var entry MemoryEntry
		if err := json.Unmarshal(data, &entry); err == nil {
			return &entry, nil
		}
	}

	// 尝试 archive 目录
	path = filepath.Join(ms.rootDir, "archive", id+".json")
	if data, err := os.ReadFile(path); err == nil {
		var entry MemoryEntry
		if err := json.Unmarshal(data, &entry); err == nil {
			return &entry, nil
		}
	}

	return nil, fmt.Errorf("记忆 [%s] 不存在", id)
}

// Search 多维度检索记忆
func (ms *MemoryStore) Search(query SearchQuery) ([]*MemoryEntry, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	var candidates []IndexEntry

	for _, ie := range ms.index.Entries {
		// 主题精确匹配
		if query.Topic != "" && !strings.EqualFold(ie.Topic, query.Topic) {
			continue
		}

		// 重要性范围
		if query.ImportanceMin > 0 && ie.Importance < query.ImportanceMin {
			continue
		}
		if query.ImportanceMax > 0 && ie.Importance > query.ImportanceMax {
			continue
		}

		// 标签过滤（任一匹配即可）
		if len(query.Tags) > 0 {
			tagMatched := false
			for _, qt := range query.Tags {
				for _, it := range ie.Tags {
					if strings.EqualFold(qt, it) {
						tagMatched = true
						break
					}
				}
				if tagMatched {
					break
				}
			}
			if !tagMatched {
				continue
			}
		}

		// 关键词全文搜索
		if query.Keyword != "" {
			// 搜索摘要
			if !strings.Contains(strings.ToLower(ie.Summary), strings.ToLower(query.Keyword)) {
				// 如果摘要不匹配，加载完整内容搜索
				entry, err := ms.loadEntryByID(ie.ID)
				if err != nil || !strings.Contains(strings.ToLower(entry.Content), strings.ToLower(query.Keyword)) {
					continue
				}
			}
		}

		candidates = append(candidates, ie)
	}

	// 排序
	switch query.SortBy {
	case "time":
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].UpdatedAt > candidates[j].UpdatedAt
		})
	case "access":
		// 需要加载完整条目获取 AccessCount
		// 这里简单按更新时间排序
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].UpdatedAt > candidates[j].UpdatedAt
		})
	default: // importance
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].Importance != candidates[j].Importance {
				return candidates[i].Importance > candidates[j].Importance
			}
			return candidates[i].UpdatedAt > candidates[j].UpdatedAt
		})
	}

	// 限制数量
	limit := query.Limit
	if limit <= 0 {
		limit = 20
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	// 加载完整条目
	result := make([]*MemoryEntry, 0, len(candidates))
	for _, ie := range candidates {
		entry, err := ms.loadEntryByID(ie.ID)
		if err != nil {
			continue
		}
		// 增加访问计数
		entry.AccessCount++
		result = append(result, entry)
	}

	return result, nil
}

// Delete 删除记忆条目
func (ms *MemoryStore) Delete(id string, soft bool) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	// 查找条目
	entry, err := ms.loadEntryByID(id)
	if err != nil {
		return err
	}

	// 确定源路径
	srcPath := ms.entryPath(id, entry.Importance)
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		srcPath = ms.archivePath(id)
	}

	if soft {
		// 软删除：移入回收站
		dstPath := ms.trashPath(id)
		if err := os.Rename(srcPath, dstPath); err != nil {
			// 如果跨设备，用 copy+delete
			data, _ := os.ReadFile(srcPath)
			os.WriteFile(dstPath, data, 0644)
			os.Remove(srcPath)
		}
	} else {
		// 硬删除
		os.Remove(srcPath)
		// 也检查 archive
		os.Remove(ms.archivePath(id))
	}

	ms.removeFromIndex(id)
	return ms.flush()
}

// DeleteByTopic 按主题删除
func (ms *MemoryStore) DeleteByTopic(topic string, soft bool) (int, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	var ids []string
	for _, ie := range ms.index.Entries {
		if strings.EqualFold(ie.Topic, topic) {
			ids = append(ids, ie.ID)
		}
	}

	count := 0
	for _, id := range ids {
		entry, err := ms.loadEntryByID(id)
		if err != nil {
			continue
		}
		srcPath := ms.entryPath(id, entry.Importance)
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			srcPath = ms.archivePath(id)
		}

		if soft {
			dstPath := ms.trashPath(id)
			data, _ := os.ReadFile(srcPath)
			os.WriteFile(dstPath, data, 0644)
			os.Remove(srcPath)
		} else {
			os.Remove(srcPath)
			os.Remove(ms.archivePath(id))
		}

		ms.removeFromIndex(id)
		count++
	}

	if count > 0 {
		ms.dirty = true
		ms.flush()
	}

	return count, nil
}

// ============================================
// 记忆衰减
// ============================================

// Decay 执行记忆衰减，返回归档数和删除数
func (ms *MemoryStore) Decay() (archived int, deleted int, err error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	now := time.Now().Unix()
	daySeconds := int64(24 * 60 * 60)

	var toArchive []IndexEntry
	var toDelete []IndexEntry

	for _, ie := range ms.index.Entries {
		if ie.Importance >= ImportanceCore {
			continue // 核心记忆不衰减
		}

		daysSinceAccess := (now - ie.UpdatedAt) / daySeconds

		// 计算新的重要性
		newImportance := ie.Importance
		if daysSinceAccess > DecayDaysLevel3 {
			newImportance -= 3
		} else if daysSinceAccess > DecayDaysLevel2 {
			newImportance -= 2
		} else if daysSinceAccess > DecayDaysLevel1 {
			newImportance -= 1
		}

		if newImportance < ImportanceMin || daysSinceAccess > DecayDaysArchive {
			toDelete = append(toDelete, ie)
		} else if newImportance < ie.Importance {
			// 需要降级
			entry, loadErr := ms.loadEntryByID(ie.ID)
			if loadErr != nil {
				continue
			}
			entry.Importance = newImportance
			entry.UpdatedAt = now

			// 如果降到 ImportanceLow 以下，移入归档
			if newImportance < ImportanceLow {
				toArchive = append(toArchive, ie)
			} else {
				// 更新文件
				data, _ := json.MarshalIndent(entry, "", "  ")
				path := ms.entryPath(entry.ID, entry.Importance)
				os.WriteFile(path, data, 0644)
				ms.updateIndex(entry)
			}
		}
	}

	// 处理归档
	for _, ie := range toArchive {
		entry, loadErr := ms.loadEntryByID(ie.ID)
		if loadErr != nil {
			continue
		}
		// 移入归档
		srcPath := ms.entryPath(ie.ID, entry.Importance)
		dstPath := ms.archivePath(ie.ID)
		data, _ := os.ReadFile(srcPath)
		os.WriteFile(dstPath, data, 0644)
		os.Remove(srcPath)

		entry.Importance = 1
		ms.updateIndex(entry)
		archived++
	}

	// 处理删除
	for _, ie := range toDelete {
		entry, loadErr := ms.loadEntryByID(ie.ID)
		if loadErr != nil {
			// 文件可能已丢失，直接从索引移除
			ms.removeFromIndex(ie.ID)
			deleted++
			continue
		}
		srcPath := ms.entryPath(ie.ID, entry.Importance)
		os.Remove(srcPath)
		os.Remove(ms.archivePath(ie.ID))
		ms.removeFromIndex(ie.ID)
		deleted++
	}

	if archived > 0 || deleted > 0 {
		ms.dirty = true
		ms.flush()
	}

	return
}

// ============================================
// 关联管理
// ============================================

// Link 在两个记忆之间建立关联
func (ms *MemoryStore) Link(sourceID, targetID, relation string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	source, err := ms.loadEntryByID(sourceID)
	if err != nil {
		return fmt.Errorf("源记忆不存在: %v", err)
	}
	target, err := ms.loadEntryByID(targetID)
	if err != nil {
		return fmt.Errorf("目标记忆不存在: %v", err)
	}

	// 检查是否已关联
	for _, link := range source.Links {
		if link == targetID {
			return nil // 已关联
		}
	}

	source.Links = append(source.Links, targetID)
	target.Links = append(target.Links, sourceID)

	// 保存双方
	data, _ := json.MarshalIndent(source, "", "  ")
	os.WriteFile(ms.entryPath(sourceID, source.Importance), data, 0644)
	ms.updateIndex(source)

	data, _ = json.MarshalIndent(target, "", "  ")
	os.WriteFile(ms.entryPath(targetID, target.Importance), data, 0644)
	ms.updateIndex(target)

	return ms.flush()
}

// Unlink 解除关联
func (ms *MemoryStore) Unlink(sourceID, targetID string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	source, err := ms.loadEntryByID(sourceID)
	if err == nil {
		for i, link := range source.Links {
			if link == targetID {
				source.Links = append(source.Links[:i], source.Links[i+1:]...)
				data, _ := json.MarshalIndent(source, "", "  ")
				os.WriteFile(ms.entryPath(sourceID, source.Importance), data, 0644)
				ms.updateIndex(source)
				break
			}
		}
	}

	target, err := ms.loadEntryByID(targetID)
	if err == nil {
		for i, link := range target.Links {
			if link == sourceID {
				target.Links = append(target.Links[:i], target.Links[i+1:]...)
				data, _ := json.MarshalIndent(target, "", "  ")
				os.WriteFile(ms.entryPath(targetID, target.Importance), data, 0644)
				ms.updateIndex(target)
				break
			}
		}
	}

	return ms.flush()
}

// ============================================
// 统计
// ============================================

// Stats 获取记忆统计信息
func (ms *MemoryStore) Stats() *MemoryStats {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	stats := &MemoryStats{
		TotalEntries: len(ms.index.Entries),
		ByImportance: make(map[int]int),
		ByTopic:      make(map[string]int),
	}

	// 按重要性分组
	for _, ie := range ms.index.Entries {
		stats.ByImportance[ie.Importance]++
		stats.ByTopic[ie.Topic]++
		if ie.Importance >= ImportanceCore {
			stats.CoreEntries++
		}
		if len(ie.Links) > 0 {
			stats.TotalLinks += len(ie.Links)
		}
	}

	stats.TagCount = len(ms.index.TagIndex)

	// 统计归档
	archiveDir := filepath.Join(ms.rootDir, "archive")
	if entries, err := os.ReadDir(archiveDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				stats.ArchivedCount++
			}
		}
	}

	// 最近记忆（按更新时间排序）
	sorted := make([]IndexEntry, len(ms.index.Entries))
	copy(sorted, ms.index.Entries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].UpdatedAt > sorted[j].UpdatedAt
	})
	if len(sorted) > 5 {
		sorted = sorted[:5]
	}
	stats.RecentEntries = sorted

	// 最常访问（按版本号+更新时间估算）
	topSorted := make([]IndexEntry, len(ms.index.Entries))
	copy(topSorted, ms.index.Entries)
	sort.Slice(topSorted, func(i, j int) bool {
		return topSorted[i].Version > topSorted[j].Version
	})
	if len(topSorted) > 5 {
		topSorted = topSorted[:5]
	}
	stats.TopAccessed = topSorted

	return stats
}

// ============================================
// 旧数据迁移
// ============================================

// MigrateOldFormat 扫描并导入旧格式的 .md 记忆文件
func (ms *MemoryStore) MigrateOldFormat() (int, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	entries, err := os.ReadDir(ms.rootDir)
	if err != nil {
		return 0, err
	}

	migrated := 0
	migratedDir := filepath.Join(ms.rootDir, ".migrated")
	os.MkdirAll(migratedDir, 0755)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if e.Name() == "creator.json" {
			continue
		}

		mdPath := filepath.Join(ms.rootDir, e.Name())
		data, err := os.ReadFile(mdPath)
		if err != nil {
			continue
		}

		topic := strings.TrimSuffix(e.Name(), ".md")
		content := string(data)

		entry := &MemoryEntry{
			ID:         newUUID(),
			Topic:      topic,
			Content:    content,
			Importance: ImportanceNormal,
			Tags:       []string{},
			Links:      []string{},
			CreatedAt:  time.Now().Unix(),
			UpdatedAt:  time.Now().Unix(),
			Version:    1,
		}

		// 保存为新格式
		path := ms.entryPath(entry.ID, 0)
		jsonData, _ := json.MarshalIndent(entry, "", "  ")
		os.WriteFile(path, jsonData, 0644)
		ms.addToIndex(ms.index, entry)

		// 移走旧文件
		os.Rename(mdPath, filepath.Join(migratedDir, e.Name()))
		migrated++
	}

	if migrated > 0 {
		ms.dirty = true
		ms.flush()
	}

	return migrated, nil
}

// ============================================
// 全局单例
// ============================================

var globalMemoryStore *MemoryStore
var memoryStoreOnce sync.Once

// GetMemoryStore 获取全局记忆存储引擎
func GetMemoryStore() *MemoryStore {
	memoryStoreOnce.Do(func() {
		globalMemoryStore = NewMemoryStore()
	})
	return globalMemoryStore
}
