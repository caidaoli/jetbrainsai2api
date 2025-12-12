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
		capacity: CacheDefaultCapacity, // 优化缓存容量
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
	ticker := time.NewTicker(CacheCleanupInterval)
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

// CacheService 统一缓存服务
// SRP: 单一职责 - 只负责缓存管理
type CacheService struct {
	messages *LRUCache // 消息转换缓存
	tools    *LRUCache // 工具验证缓存
	quota    *LRUCache // 配额缓存 (修复竞态条件)
	params   *LRUCache // 参数转换缓存
}

// NewCacheService 创建新的缓存服务
func NewCacheService() *CacheService {
	return &CacheService{
		messages: NewCache(),
		tools:    NewCache(),
		quota:    NewCache(),
		params:   NewCache(),
	}
}

// GetMessageCache 获取消息转换缓存
func (cs *CacheService) GetMessageCache(key string) (any, bool) {
	return cs.messages.Get(key)
}

// SetMessageCache 设置消息转换缓存
func (cs *CacheService) SetMessageCache(key string, value any, duration time.Duration) {
	cs.messages.Set(key, value, duration)
}

// GetToolCache 获取工具验证缓存
func (cs *CacheService) GetToolCache(key string) (any, bool) {
	return cs.tools.Get(key)
}

// SetToolCache 设置工具验证缓存
func (cs *CacheService) SetToolCache(key string, value any, duration time.Duration) {
	cs.tools.Set(key, value, duration)
}

// GetQuotaCache 获取配额缓存 (修复 TOCTOU 竞态条件 - 返回深拷贝)
func (cs *CacheService) GetQuotaCache(key string) (*JetbrainsQuotaResponse, bool) {
	cached, found := cs.quota.Get(key)
	if !found {
		return nil, false
	}

	// 深拷贝：防止返回的数据被外部修改
	quotaData, ok := cached.(*JetbrainsQuotaResponse)
	if !ok {
		return nil, false
	}

	// 深拷贝 JetbrainsQuotaResponse
	copied := &JetbrainsQuotaResponse{
		Current: struct {
			Current struct {
				Amount string `json:"amount"`
			} `json:"current"`
			Maximum struct {
				Amount string `json:"amount"`
			} `json:"maximum"`
		}{
			Current: struct {
				Amount string `json:"amount"`
			}{
				Amount: quotaData.Current.Current.Amount,
			},
			Maximum: struct {
				Amount string `json:"amount"`
			}{
				Amount: quotaData.Current.Maximum.Amount,
			},
		},
		Until: quotaData.Until,
	}

	return copied, true
}

// SetQuotaCache 设置配额缓存
func (cs *CacheService) SetQuotaCache(key string, value *JetbrainsQuotaResponse, duration time.Duration) {
	// 存储时也进行深拷贝，确保完全隔离
	copied := &JetbrainsQuotaResponse{
		Current: struct {
			Current struct {
				Amount string `json:"amount"`
			} `json:"current"`
			Maximum struct {
				Amount string `json:"amount"`
			} `json:"maximum"`
		}{
			Current: struct {
				Amount string `json:"amount"`
			}{
				Amount: value.Current.Current.Amount,
			},
			Maximum: struct {
				Amount string `json:"amount"`
			}{
				Amount: value.Current.Maximum.Amount,
			},
		},
		Until: value.Until,
	}
	cs.quota.Set(key, copied, duration)
}

// DeleteQuotaCache 删除配额缓存
func (cs *CacheService) DeleteQuotaCache(key string) {
	cs.quota.mu.Lock()
	defer cs.quota.mu.Unlock()

	if item, found := cs.quota.items[key]; found {
		cs.quota.remove(item)
		delete(cs.quota.items, key)
	}
}

// ClearQuotaCache 清空所有配额缓存
func (cs *CacheService) ClearQuotaCache() {
	cs.quota.mu.Lock()
	defer cs.quota.mu.Unlock()

	// 重置链表
	cs.quota.head.next = cs.quota.tail
	cs.quota.tail.prev = cs.quota.head

	// 清空 map
	cs.quota.items = make(map[string]*CacheItem)
}

// GetParamCache 获取参数转换缓存
func (cs *CacheService) GetParamCache(key string) (any, bool) {
	return cs.params.Get(key)
}

// SetParamCache 设置参数转换缓存
func (cs *CacheService) SetParamCache(key string, value any, duration time.Duration) {
	cs.params.Set(key, value, duration)
}

// Get 实现 Cache 接口（统一获取方法，默认使用 messages 缓存）
func (cs *CacheService) Get(key string) (any, bool) {
	return cs.messages.Get(key)
}

// Set 实现 Cache 接口（统一设置方法，默认使用 messages 缓存）
func (cs *CacheService) Set(key string, value any, duration time.Duration) {
	cs.messages.Set(key, value, duration)
}

// Stop 实现 Cache 接口
func (cs *CacheService) Stop() {
	cs.Close()
}

// Close 优雅关闭所有缓存
func (cs *CacheService) Close() error {
	cs.messages.Stop()
	cs.tools.Stop()
	cs.quota.Stop()
	cs.params.Stop()
	return nil
}

// 缓存键生成函数

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

// generateQuotaCacheKey creates a cache key for quota data
func generateQuotaCacheKey(account *JetbrainsAccount) string {
	// 修复: 使用 licenseID 作为缓存键而非 JWT，避免敏感信息泄露
	cacheKey := account.LicenseID
	if cacheKey == "" {
		// 如果没有 licenseID，使用 JWT 的前8个字符作为标识（仅用于缓存键）
		if len(account.JWT) > 8 {
			cacheKey = account.JWT[:8]
		} else {
			cacheKey = account.JWT
		}
	}
	return cacheKey
}

// Helper function to marshal JSON, using Sonic for performance
func marshalJSON(v any) ([]byte, error) {
	return sonic.Marshal(v)
}

// 全局 CacheService 实例
// 新代码应该使用依赖注入的 CacheService，此全局实例仅用于向后兼容
var globalCacheService = NewCacheService()
