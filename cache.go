package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"sync"
	"time"

	"github.com/bytedance/sonic"
)

// LRUCache is a thread-safe LRU cache with expiration
type LRUCache struct {
	capacity int
	items    map[string]*CacheItem
	mu       sync.RWMutex
	head     *CacheItem
	tail     *CacheItem
	// 优雅关闭支持
	ctx    context.Context
	cancel context.CancelFunc
}

// CacheItem represents an item in the cache with LRU links
type CacheItem struct {
	Value      any
	Expiration int64
	key        string
	prev       *CacheItem
	next       *CacheItem
}

// NewCache creates a new LRU Cache with optimized capacity.
func NewCache() *LRUCache {
	ctx, cancel := context.WithCancel(context.Background())
	cache := &LRUCache{
		capacity: 1000, // 优化缓存容量
		items:    make(map[string]*CacheItem),
		ctx:      ctx,
		cancel:   cancel,
	}

	// Initialize sentinel nodes
	cache.head = &CacheItem{}
	cache.tail = &CacheItem{}
	cache.head.next = cache.tail
	cache.tail.prev = cache.head

	// 启动后台清理 goroutine，支持优雅关闭
	go cache.startCleanupWorker()
	return cache
}

// startCleanupWorker 后台清理过期缓存项，支持优雅关闭
func (c *LRUCache) startCleanupWorker() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanupExpired()
		case <-c.ctx.Done():
			// 收到关闭信号，优雅退出
			return
		}
	}
}

// Stop 停止后台清理 goroutine
func (c *LRUCache) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

// Set adds an item to the cache, replacing any existing item.
func (c *LRUCache) Set(key string, value any, duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If item exists, update it and move to front
	if item, exists := c.items[key]; exists {
		item.Value = value
		item.Expiration = time.Now().Add(duration).UnixNano()
		c.moveToFront(item)
		return
	}

	// Create new item
	item := &CacheItem{
		Value:      value,
		Expiration: time.Now().Add(duration).UnixNano(),
		key:        key,
	}

	// Add to front
	c.addToFront(item)
	c.items[key] = item

	// Evict if over capacity
	if len(c.items) > c.capacity {
		c.evict()
	}
}

// Get gets an item from the cache. It returns the item or nil, and a bool indicating whether the key was found.
func (c *LRUCache) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, found := c.items[key]
	if !found {
		return nil, false
	}

	// 检查是否过期
	if time.Now().UnixNano() > item.Expiration {
		// 立即删除过期项，避免缓存污染
		c.remove(item)
		delete(c.items, key)
		return nil, false
	}

	// Move to front for LRU
	c.moveToFront(item)
	return item.Value, true
}

// 全局 cache 实例（向后兼容，逐步迁移到依赖注入）
// 新代码应该使用注入的 Cache 接口
var (
	messageConversionCache = NewCache()
	toolsValidationCache   = NewCache()
)

// generateMessagesCacheKey creates a cache key from chat messages.
func generateMessagesCacheKey(messages []ChatMessage) string {
	// 优化: 使用流式hash，避免大量内存分配
	h := sha1.New()
	for _, msg := range messages {
		h.Write([]byte(msg.Role))
		if content, ok := msg.Content.(string); ok {
			h.Write([]byte(content))
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// generateToolsCacheKey creates a cache key from a slice of tools.
func generateToolsCacheKey(tools []Tool) string {
	// 优化: 使用流式hash，避免大量内存分配
	h := sha1.New()
	for _, t := range tools {
		h.Write([]byte(t.Type))
		h.Write([]byte(t.Function.Name))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// generateParamsCacheKey creates a cache key from parameter schemas
func generateParamsCacheKey(params map[string]any) string {
	// 使用 Sonic 快速序列化
	data, _ := marshalJSON(params)
	hash := sha1.Sum(data)
	return hex.EncodeToString(hash[:])
}

// Helper function to marshal JSON, using Sonic for performance
func marshalJSON(v any) ([]byte, error) {
	return sonic.Marshal(v)
}

// LRU cache helper methods
func (c *LRUCache) addToFront(item *CacheItem) {
	item.next = c.head.next
	item.prev = c.head
	c.head.next.prev = item
	c.head.next = item
}

func (c *LRUCache) moveToFront(item *CacheItem) {
	c.remove(item)
	c.addToFront(item)
}

func (c *LRUCache) remove(item *CacheItem) {
	item.prev.next = item.next
	item.next.prev = item.prev
}

func (c *LRUCache) evict() {
	if c.tail.prev == c.head {
		return
	}

	item := c.tail.prev
	c.remove(item)
	delete(c.items, item.key)
}

func (c *LRUCache) cleanupExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().UnixNano()
	for key, item := range c.items {
		if now > item.Expiration {
			c.remove(item)
			delete(c.items, key)
		}
	}
}
