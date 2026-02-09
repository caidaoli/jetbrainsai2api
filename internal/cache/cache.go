package cache

import (
	"context"
	"crypto/sha1" //nolint:gosec // G505: sha1 for cache keys, not security
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/util"
)

// LRUCache is a thread-safe LRU cache with expiration
type LRUCache struct {
	capacity int
	items    map[string]*CacheItem
	mu       sync.RWMutex
	head     *CacheItem
	tail     *CacheItem
	ctx      context.Context
	cancel   context.CancelFunc
}

// CacheItem represents an item in the cache with LRU links
type CacheItem struct {
	Value      any
	Expiration int64
	key        string
	prev       *CacheItem
	next       *CacheItem
}

// NewCache creates a new LRU Cache
func NewCache() *LRUCache {
	ctx, cancel := context.WithCancel(context.Background())
	c := &LRUCache{
		capacity: core.CacheDefaultCapacity,
		items:    make(map[string]*CacheItem),
		ctx:      ctx,
		cancel:   cancel,
	}

	c.head = &CacheItem{}
	c.tail = &CacheItem{}
	c.head.next = c.tail
	c.tail.prev = c.head

	go c.startCleanupWorker()
	return c
}

func (c *LRUCache) startCleanupWorker() {
	ticker := time.NewTicker(core.CacheCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanupExpired()
		case <-c.ctx.Done():
			return
		}
	}
}

// Stop terminates the cache cleanup worker goroutine.
func (c *LRUCache) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

// Set stores a value in the cache with the given TTL.
func (c *LRUCache) Set(key string, value any, duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if item, exists := c.items[key]; exists {
		item.Value = value
		item.Expiration = time.Now().Add(duration).UnixNano()
		c.moveToFront(item)
		return
	}

	item := &CacheItem{
		Value:      value,
		Expiration: time.Now().Add(duration).UnixNano(),
		key:        key,
	}

	c.addToFront(item)
	c.items[key] = item

	if len(c.items) > c.capacity {
		c.evict()
	}
}

// Get retrieves a value from the cache, returning false if not found or expired.
func (c *LRUCache) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, found := c.items[key]
	if !found {
		return nil, false
	}

	if time.Now().UnixNano() > item.Expiration {
		c.remove(item)
		delete(c.items, key)
		return nil, false
	}

	c.moveToFront(item)
	return item.Value, true
}

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

// Clear clears all cache items
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.head.next = c.tail
	c.tail.prev = c.head
	c.items = make(map[string]*CacheItem)
}

// CacheService unified cache service
type CacheService struct {
	general *LRUCache
	quota   *LRUCache
}

// NewCacheService creates a new CacheService with general and quota caches.
func NewCacheService() *CacheService {
	return &CacheService{
		general: NewCache(),
		quota:   NewCache(),
	}
}

// GetQuotaCache retrieves quota data from the quota-specific cache.
func (cs *CacheService) GetQuotaCache(key string) (*core.JetbrainsQuotaResponse, bool) {
	cached, found := cs.quota.Get(key)
	if !found {
		return nil, false
	}

	quotaData, ok := cached.(*core.JetbrainsQuotaResponse)
	if !ok {
		return nil, false
	}

	return quotaData.Clone(), true
}

// SetQuotaCache stores quota data in the quota-specific cache.
func (cs *CacheService) SetQuotaCache(key string, value *core.JetbrainsQuotaResponse, duration time.Duration) {
	cs.quota.Set(key, value.Clone(), duration)
}

// DeleteQuotaCache removes quota data from the quota-specific cache.
func (cs *CacheService) DeleteQuotaCache(key string) {
	cs.quota.mu.Lock()
	defer cs.quota.mu.Unlock()

	if item, found := cs.quota.items[key]; found {
		cs.quota.remove(item)
		delete(cs.quota.items, key)
	}
}

// ClearQuotaCache removes all items from the quota cache.
func (cs *CacheService) ClearQuotaCache() {
	cs.quota.Clear()
}

// Get retrieves a value from the general cache.
func (cs *CacheService) Get(key string) (any, bool) {
	return cs.general.Get(key)
}

// Set stores a value in the general cache.
func (cs *CacheService) Set(key string, value any, duration time.Duration) {
	cs.general.Set(key, value, duration)
}

// Stop terminates both general and quota cache cleanup workers.
func (cs *CacheService) Stop() {
	cs.general.Stop()
	cs.quota.Stop()
}

// Close stops the cache service and releases resources.
func (cs *CacheService) Close() error {
	cs.Stop()
	return nil
}

// GenerateMessagesCacheKey creates a cache key from chat messages
func GenerateMessagesCacheKey(messages []core.ChatMessage) string {
	h := sha1.New() //nolint:gosec // G401: sha1 for cache keys, not security
	for _, msg := range messages {
		msgBytes, err := util.MarshalJSON(msg)
		if err != nil {
			h.Write([]byte(msg.Role))
			continue
		}
		h.Write(msgBytes)
	}
	return fmt.Sprintf("msg:%s:%s", core.CacheKeyVersion, hex.EncodeToString(h.Sum(nil)))
}

// GenerateToolsCacheKey creates a cache key from tools
func GenerateToolsCacheKey(tools []core.Tool) string {
	h := sha1.New() //nolint:gosec // G401: sha1 for cache keys, not security
	for _, t := range tools {
		toolBytes, err := util.MarshalJSON(t)
		if err != nil {
			h.Write([]byte(t.Type))
			h.Write([]byte(t.Function.Name))
			continue
		}
		h.Write(toolBytes)
	}
	return fmt.Sprintf("tools:%s:%s", core.CacheKeyVersion, hex.EncodeToString(h.Sum(nil)))
}

// GenerateQuotaCacheKey creates a cache key for quota data
func GenerateQuotaCacheKey(account *core.JetbrainsAccount) string {
	cacheKey := account.LicenseID
	if cacheKey == "" {
		if len(account.JWT) > 8 {
			cacheKey = account.JWT[:8]
		} else {
			cacheKey = account.JWT
		}
	}
	return fmt.Sprintf("quota:%s:%s", core.CacheKeyVersion, cacheKey)
}

// TruncateCacheKey safely truncates cache key for log display
func TruncateCacheKey(key string, maxLen int) string {
	if len(key) <= maxLen {
		return key
	}
	return key[:maxLen]
}
