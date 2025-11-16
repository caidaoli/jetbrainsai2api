package main

import (
	"sync"
	"testing"
	"time"
)

// TestLRUCache_BasicSetGet 测试基本的设置和获取
func TestLRUCache_BasicSetGet(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()

	cache.Set("key1", "value1", 1*time.Hour)

	value, found := cache.Get("key1")
	if !found {
		t.Error("Expected to find key1")
	}

	if value != "value1" {
		t.Errorf("Expected 'value1', got '%v'", value)
	}
}

// TestLRUCache_GetNonExistent 测试获取不存在的键
func TestLRUCache_GetNonExistent(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()

	_, found := cache.Get("nonexistent")
	if found {
		t.Error("Should not find nonexistent key")
	}
}

// TestLRUCache_Expiration 测试过期机制
func TestLRUCache_Expiration(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()

	cache.Set("key", "value", 100*time.Millisecond)

	// 立即获取应该成功
	_, found := cache.Get("key")
	if !found {
		t.Error("Key should be found immediately after set")
	}

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	// 现在应该找不到
	_, found = cache.Get("key")
	if found {
		t.Error("Key should be expired")
	}
}

// TestLRUCache_Eviction 测试LRU驱逐机制
func TestLRUCache_Eviction(t *testing.T) {
	// 使用 NewCache 创建缓存，然后修改容量
	cache := NewCache()
	defer cache.Stop()

	// 修改容量为2用于测试
	cache.mu.Lock()
	cache.capacity = 2
	cache.mu.Unlock()

	// 添加3个项
	cache.Set("key1", "value1", 1*time.Hour)
	cache.Set("key2", "value2", 1*time.Hour)
	cache.Set("key3", "value3", 1*time.Hour) // 应该驱逐 key1

	// key1 应该被驱逐
	_, found := cache.Get("key1")
	if found {
		t.Error("key1 should be evicted")
	}

	// key2 和 key3 应该存在
	_, found = cache.Get("key2")
	if !found {
		t.Error("key2 should exist")
	}

	_, found = cache.Get("key3")
	if !found {
		t.Error("key3 should exist")
	}
}

// TestLRUCache_LRUOrder 测试LRU顺序
func TestLRUCache_LRUOrder(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()

	// 修改容量为2用于测试
	cache.mu.Lock()
	cache.capacity = 2
	cache.mu.Unlock()

	cache.Set("key1", "value1", 1*time.Hour)
	cache.Set("key2", "value2", 1*time.Hour)

	// 访问 key1，使其成为最近使用
	cache.Get("key1")

	// 添加 key3，应该驱逐 key2（最久未使用）
	cache.Set("key3", "value3", 1*time.Hour)

	// key2 应该被驱逐
	_, found := cache.Get("key2")
	if found {
		t.Error("key2 should be evicted (least recently used)")
	}

	// key1 和 key3 应该存在
	_, found = cache.Get("key1")
	if !found {
		t.Error("key1 should exist")
	}

	_, found = cache.Get("key3")
	if !found {
		t.Error("key3 should exist")
	}
}

// TestLRUCache_ConcurrentAccess 测试并发访问
func TestLRUCache_ConcurrentAccess(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()

	const numGoroutines = 100
	const numOperations = 100

	var wg sync.WaitGroup

	// 并发写入
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := string(rune('a' + (id+j)%26))
				cache.Set(key, id*numOperations+j, 1*time.Hour)
			}
		}(i)
	}

	// 并发读取
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := string(rune('a' + (id+j)%26))
				cache.Get(key)
			}
		}(i)
	}

	wg.Wait()
	// 如果没有 panic，测试通过
}

// TestLRUCache_UpdateExisting 测试更新已存在的键
func TestLRUCache_UpdateExisting(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()

	cache.Set("key", "value1", 1*time.Hour)

	value, _ := cache.Get("key")
	if value != "value1" {
		t.Errorf("Expected 'value1', got '%v'", value)
	}

	// 更新值
	cache.Set("key", "value2", 1*time.Hour)

	value, _ = cache.Get("key")
	if value != "value2" {
		t.Errorf("Expected 'value2', got '%v'", value)
	}
}

// TestLRUCache_ExpiredItemCleanup 测试过期项的立即清理
func TestLRUCache_ExpiredItemCleanup(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()

	cache.Set("key1", "value1", 50*time.Millisecond)
	cache.Set("key2", "value2", 1*time.Hour)

	// 等待 key1 过期
	time.Sleep(100 * time.Millisecond)

	// 获取 key1 应该触发立即清理
	_, found := cache.Get("key1")
	if found {
		t.Error("key1 should be expired and cleaned up")
	}

	// key2 应该仍然存在
	_, found = cache.Get("key2")
	if !found {
		t.Error("key2 should still exist")
	}

	// 验证 key1 已从缓存中删除
	cache.mu.Lock()
	_, exists := cache.items["key1"]
	cache.mu.Unlock()

	if exists {
		t.Error("key1 should be removed from cache items")
	}
}

// TestLRUCache_ZeroTTL 测试零TTL
func TestLRUCache_ZeroTTL(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()

	cache.Set("key", "value", 0)

	// 零TTL应该立即过期
	_, found := cache.Get("key")
	if found {
		t.Error("Key with zero TTL should be immediately expired")
	}
}

// TestLRUCache_NegativeTTL 测试负TTL
func TestLRUCache_NegativeTTL(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()

	cache.Set("key", "value", -1*time.Second)

	// 负TTL应该立即过期
	_, found := cache.Get("key")
	if found {
		t.Error("Key with negative TTL should be immediately expired")
	}
}

// TestLRUCache_PeriodicCleanup 测试定期清理
// 注意：定期清理间隔是5分钟，这个测试验证清理机制存在即可
// 实际的过期项清理由 TestLRUCache_ExpiredItemCleanup 测试（立即清理）
func TestLRUCache_PeriodicCleanup(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()

	// 添加一些项
	for i := 0; i < 5; i++ {
		cache.Set(string(rune('a'+i)), i, 1*time.Hour)
	}

	// 验证项被添加
	cache.mu.Lock()
	itemCount := len(cache.items)
	cache.mu.Unlock()

	if itemCount != 5 {
		t.Errorf("Expected 5 items, got %d", itemCount)
	}

	// 注意：我们不测试实际的定期清理，因为间隔太长（5分钟）
	// 过期清理的功能由 TestLRUCache_ExpiredItemCleanup 覆盖
}

// TestLRUCache_TypeSafety 测试类型安全
func TestLRUCache_TypeSafety(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()

	// 存储不同类型的值
	cache.Set("string", "value", 1*time.Hour)
	cache.Set("int", 42, 1*time.Hour)
	cache.Set("struct", struct{ Name string }{"test"}, 1*time.Hour)

	// 获取并验证类型
	strVal, _ := cache.Get("string")
	if _, ok := strVal.(string); !ok {
		t.Error("Expected string type")
	}

	intVal, _ := cache.Get("int")
	if _, ok := intVal.(int); !ok {
		t.Error("Expected int type")
	}

	structVal, _ := cache.Get("struct")
	if _, ok := structVal.(struct{ Name string }); !ok {
		t.Error("Expected struct type")
	}
}
