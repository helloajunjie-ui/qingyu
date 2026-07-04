package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ============================================
// 全局缓存引擎 — 内存 + 磁盘双层缓存
// 减少重复网络请求和 LLM 调用
// 设计要点：
//   - 内存优先：读操作先查内存，miss 后查磁盘
//   - 磁盘持久化：异步写入，重启后缓存不丢失
//   - TTL 过期：惰性删除 + 启动时清理
//   - 线程安全：RWMutex 支持并发读取
//   - 缓存键：SHA256 哈希，避免键冲突
// ============================================

// CacheEntry 缓存条目
// TTL 为有效期秒数，过期后惰性删除
type CacheEntry struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	TTL       int64     `json:"ttl"`
}

// CacheEngine 缓存引擎
// mem: 内存缓存（最快）
// diskDir: 磁盘缓存目录（.cache/）
// defaultTTL: 默认有效期秒数
type CacheEngine struct {
	mu         sync.RWMutex
	mem        map[string]*CacheEntry
	diskDir    string
	defaultTTL int64
}

var (
	globalCache *CacheEngine
	cacheOnce   sync.Once
)

// GetCache 获取全局缓存引擎（单例）
// 默认 TTL 600 秒（10 分钟）
// 首次调用时创建，之后返回同一实例
func GetCache() *CacheEngine {
	cacheOnce.Do(func() {
		globalCache = NewCacheEngine(600)
	})
	return globalCache
}

// NewCacheEngine 创建新的缓存引擎实例
// 自动创建 .cache 目录（权限 0700）
// 启动时从磁盘加载未过期的缓存条目
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

// cacheKey 生成缓存键
// 格式: {prefix}:{SHA256(prefix|part1|part2|...)[:16]}
// 使用 SHA256 前 16 字节作为哈希，平衡唯一性和长度
func cacheKey(prefix string, parts ...string) string {
	h := sha256.New()
	h.Write([]byte(prefix))
	for _, p := range parts {
		h.Write([]byte("|"))
		h.Write([]byte(p))
	}
	return prefix + ":" + hex.EncodeToString(h.Sum(nil)[:16])
}

// Get 获取缓存值
// 查找顺序：内存 → 磁盘
// 过期条目自动删除（惰性删除策略）
// 磁盘命中后自动加载到内存，加速后续访问
func (ce *CacheEngine) Get(key string) (string, bool) {
	ce.mu.RLock()
	entry, ok := ce.mem[key]
	ce.mu.RUnlock()

	if ok {
		if time.Since(entry.CreatedAt).Seconds() < float64(entry.TTL) {
			return entry.Value, true
		}
		// 内存条目过期，惰性删除
		ce.Delete(key)
	}

	// 内存未命中，尝试从磁盘加载
	diskPath := filepath.Join(ce.diskDir, key+".json")
	if data, err := os.ReadFile(diskPath); err == nil {
		var diskEntry CacheEntry
		if json.Unmarshal(data, &diskEntry) == nil {
			if time.Since(diskEntry.CreatedAt).Seconds() < float64(diskEntry.TTL) {
				// 磁盘命中，加载到内存
				ce.mu.Lock()
				ce.mem[key] = &diskEntry
				ce.mu.Unlock()
				return diskEntry.Value, true
			}
			// 磁盘条目过期，删除
			os.Remove(diskPath)
		}
	}

	return "", false
}

// Set 设置缓存值
// 同时写入内存和磁盘（异步）
// 可选参数 ttlSeconds 覆盖默认 TTL
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

	// 异步写入磁盘（带 recover 防止 panic 导致整个进程崩溃）
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("[cache] 异步写入 panic: %v\n", r)
			}
		}()
		diskPath := filepath.Join(ce.diskDir, key+".json")
		data, _ := json.Marshal(entry)
		os.WriteFile(diskPath, data, 0600)
	}()
}

// Delete 删除指定键的缓存
// 同时从内存和磁盘删除
func (ce *CacheEngine) Delete(key string) {
	ce.mu.Lock()
	delete(ce.mem, key)
	ce.mu.Unlock()

	diskPath := filepath.Join(ce.diskDir, key+".json")
	os.Remove(diskPath)
}

// Clear 清空所有缓存
// 重置内存缓存，删除整个磁盘缓存目录后重建
func (ce *CacheEngine) Clear() {
	ce.mu.Lock()
	ce.mem = make(map[string]*CacheEntry)
	ce.mu.Unlock()

	os.RemoveAll(ce.diskDir)
	os.MkdirAll(ce.diskDir, 0700)
}

// Stats 返回缓存统计信息
// 包括内存条目数、磁盘条目数和默认 TTL
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

// loadFromDisk 启动时从磁盘加载未过期的缓存条目到内存
// 跳过过期条目并自动清理磁盘文件
func (ce *CacheEngine) loadFromDisk() {
	entries, err := os.ReadDir(ce.diskDir)
	if err != nil {
		return
	}

	now := time.Now()
	for _, e := range entries {
		if e.IsDir() {
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

// CachedNetworkCall 带缓存的网络请求包装器
// 用于 fetch_url, web_search, get_ip, get_weather 等网络工具
// 缓存键 = "net:{cachePrefix}:{SHA256(url)}"
// 缓存 TTL = 5 分钟（300 秒）
// 仅在结果非空且非错误时缓存
func CachedNetworkCall(cachePrefix, url string, fetchFn func() string) string {
	key := cacheKey("net", cachePrefix, url)

	cache := GetCache()
	if cached, ok := cache.Get(key); ok {
		return cached
	}

	result := fetchFn()
	if result != "" && !strings.HasPrefix(result, "错误") && !strings.HasPrefix(result, "无法") {
		cache.Set(key, result, 300) // 网络请求缓存 5 分钟
	}
	return result
}
