package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ============================================
// 全局缓存引擎 — 内存 + 磁盘双层缓存
// 减少重复网络请求和 LLM 调用
// ============================================

// CacheEntry 缓存条目
type CacheEntry struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	TTL       int64     `json:"ttl"` // 有效期秒数
}

// CacheEngine 缓存引擎
type CacheEngine struct {
	mu         sync.RWMutex
	mem        map[string]*CacheEntry
	diskDir    string
	defaultTTL int64 // 默认有效期秒数
}

var (
	globalCache *CacheEngine
	cacheOnce   sync.Once
)

// GetCache 获取全局缓存引擎
func GetCache() *CacheEngine {
	cacheOnce.Do(func() {
		globalCache = NewCacheEngine(600) // 默认 10 分钟
	})
	return globalCache
}

// NewCacheEngine 创建缓存引擎
func NewCacheEngine(defaultTTLSeconds int64) *CacheEngine {
	ce := &CacheEngine{
		mem:        make(map[string]*CacheEntry),
		diskDir:    filepath.Join(RootDir, ".cache"),
		defaultTTL: defaultTTLSeconds,
	}
	os.MkdirAll(ce.diskDir, 0700)
	ce.loadFromDisk()
	return ce
}

// cacheKey 生成缓存键（SHA256 哈希）
func cacheKey(prefix string, parts ...string) string {
	h := sha256.New()
	h.Write([]byte(prefix))
	for _, p := range parts {
		h.Write([]byte("|"))
		h.Write([]byte(p))
	}
	return prefix + ":" + hex.EncodeToString(h.Sum(nil)[:16])
}

// Get 获取缓存
func (ce *CacheEngine) Get(key string) (string, bool) {
	ce.mu.RLock()
	entry, ok := ce.mem[key]
	ce.mu.RUnlock()

	if ok {
		// 检查是否过期
		if time.Since(entry.CreatedAt).Seconds() < float64(entry.TTL) {
			return entry.Value, true
		}
		// 过期，惰性删除
		ce.Delete(key)
	}

	// 尝试从磁盘加载
	diskPath := filepath.Join(ce.diskDir, key+".json")
	if data, err := os.ReadFile(diskPath); err == nil {
		var diskEntry CacheEntry
		if json.Unmarshal(data, &diskEntry) == nil {
			if time.Since(diskEntry.CreatedAt).Seconds() < float64(diskEntry.TTL) {
				// 加载到内存
				ce.mu.Lock()
				ce.mem[key] = &diskEntry
				ce.mu.Unlock()
				return diskEntry.Value, true
			}
			// 过期删除
			os.Remove(diskPath)
		}
	}

	return "", false
}

// Set 设置缓存
func (ce *CacheEngine) Set(key, value string, ttlSeconds ...int64) {
	ttl := ce.defaultTTL
	if len(ttlSeconds) > 0 && ttlSeconds[0] > 0 {
		ttl = ttlSeconds[0]
	}

	entry := &CacheEntry{
		Key:       key,
		Value:     value,
		CreatedAt: time.Now(),
		TTL:       ttl,
	}

	ce.mu.Lock()
	ce.mem[key] = entry
	ce.mu.Unlock()

	// 异步写入磁盘
	go func() {
		diskPath := filepath.Join(ce.diskDir, key+".json")
		data, _ := json.Marshal(entry)
		os.WriteFile(diskPath, data, 0600)
	}()
}

// Delete 删除缓存
func (ce *CacheEngine) Delete(key string) {
	ce.mu.Lock()
	delete(ce.mem, key)
	ce.mu.Unlock()

	diskPath := filepath.Join(ce.diskDir, key+".json")
	os.Remove(diskPath)
}

// Clear 清空所有缓存
func (ce *CacheEngine) Clear() {
	ce.mu.Lock()
	ce.mem = make(map[string]*CacheEntry)
	ce.mu.Unlock()

	os.RemoveAll(ce.diskDir)
	os.MkdirAll(ce.diskDir, 0700)
}

// Stats 缓存统计
func (ce *CacheEngine) Stats() string {
	ce.mu.RLock()
	count := len(ce.mem)
	ce.mu.RUnlock()

	diskCount := 0
	if entries, err := os.ReadDir(ce.diskDir); err == nil {
		diskCount = len(entries)
	}

	return fmt.Sprintf("📦 缓存: 内存 %d 条, 磁盘 %d 条, TTL %ds", count, diskCount, ce.defaultTTL)
}

// loadFromDisk 启动时从磁盘加载缓存
func (ce *CacheEngine) loadFromDisk() {
	entries, err := os.ReadDir(ce.diskDir)
	if err != nil {
		return
	}

	now := time.Now()
	for _, e := range entries {
		if e.IsDir() || !e.IsDir() {
			path := filepath.Join(ce.diskDir, e.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var entry CacheEntry
			if json.Unmarshal(data, &entry) != nil {
				continue
			}
			// 跳过过期条目
			if now.Sub(entry.CreatedAt).Seconds() > float64(entry.TTL) {
				os.Remove(path)
				continue
			}
			ce.mem[entry.Key] = &entry
		}
	}
}

// ============================================
// 便捷缓存包装函数
// ============================================

// CachedNetworkCall 带缓存的网络请求
// 对 fetch_url, web_search, get_ip, get_weather 等工具使用
func CachedNetworkCall(cachePrefix, url string, fetchFn func() string) string {
	key := cacheKey("net", cachePrefix, url)

	cache := GetCache()
	if cached, ok := cache.Get(key); ok {
		return cached
	}

	result := fetchFn()
	if result != "" && !stringsHasPrefix(result, "错误") && !stringsHasPrefix(result, "无法") {
		cache.Set(key, result, 300) // 网络请求缓存 5 分钟
	}
	return result
}

// stringsHasPrefix 辅助函数
func stringsHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
