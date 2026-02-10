package cache

import (
	"sync"
	"testing"
	"time"

	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/util"
)

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

func TestLRUCache_GetNonExistent(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()
	_, found := cache.Get("nonexistent")
	if found {
		t.Error("Should not find nonexistent key")
	}
}

func TestLRUCache_Expiration(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()
	cache.Set("key", "value", 100*time.Millisecond)
	_, found := cache.Get("key")
	if !found {
		t.Error("Key should be found immediately after set")
	}
	time.Sleep(150 * time.Millisecond)
	_, found = cache.Get("key")
	if found {
		t.Error("Key should be expired")
	}
}

func TestLRUCache_Eviction(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()
	cache.mu.Lock()
	cache.capacity = 2
	cache.mu.Unlock()
	cache.Set("key1", "value1", 1*time.Hour)
	cache.Set("key2", "value2", 1*time.Hour)
	cache.Set("key3", "value3", 1*time.Hour)
	_, found := cache.Get("key1")
	if found {
		t.Error("key1 should be evicted")
	}
	_, found = cache.Get("key2")
	if !found {
		t.Error("key2 should exist")
	}
	_, found = cache.Get("key3")
	if !found {
		t.Error("key3 should exist")
	}
}

func TestLRUCache_LRUOrder(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()
	cache.mu.Lock()
	cache.capacity = 2
	cache.mu.Unlock()
	cache.Set("key1", "value1", 1*time.Hour)
	cache.Set("key2", "value2", 1*time.Hour)
	cache.Get("key1")
	cache.Set("key3", "value3", 1*time.Hour)
	_, found := cache.Get("key2")
	if found {
		t.Error("key2 should be evicted (least recently used)")
	}
	_, found = cache.Get("key1")
	if !found {
		t.Error("key1 should exist")
	}
}

func TestLRUCache_ConcurrentAccess(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()
	const numGoroutines = 100
	const numOperations = 100
	var wg sync.WaitGroup
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
}

func TestLRUCache_UpdateExisting(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()
	cache.Set("key", "value1", 1*time.Hour)
	v, _ := cache.Get("key")
	if v != "value1" {
		t.Errorf("Expected 'value1'")
	}
	cache.Set("key", "value2", 1*time.Hour)
	v, _ = cache.Get("key")
	if v != "value2" {
		t.Errorf("Expected 'value2'")
	}
}

func TestLRUCache_ExpiredItemCleanup(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()
	cache.Set("key1", "value1", 50*time.Millisecond)
	cache.Set("key2", "value2", 1*time.Hour)
	time.Sleep(100 * time.Millisecond)
	_, found := cache.Get("key1")
	if found {
		t.Error("key1 should be expired")
	}
	_, found = cache.Get("key2")
	if !found {
		t.Error("key2 should still exist")
	}
	cache.mu.Lock()
	_, exists := cache.items["key1"]
	cache.mu.Unlock()
	if exists {
		t.Error("key1 should be removed")
	}
}

func TestLRUCache_ZeroTTL(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()
	cache.Set("key", "value", 0)
	_, found := cache.Get("key")
	if found {
		t.Error("Key with zero TTL should be immediately expired")
	}
}

func TestLRUCache_NegativeTTL(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()
	cache.Set("key", "value", -1*time.Second)
	_, found := cache.Get("key")
	if found {
		t.Error("Key with negative TTL should be immediately expired")
	}
}

func TestLRUCache_PeriodicCleanup(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()
	for i := 0; i < 5; i++ {
		cache.Set(string(rune('a'+i)), i, 1*time.Hour)
	}
	cache.mu.Lock()
	itemCount := len(cache.items)
	cache.mu.Unlock()
	if itemCount != 5 {
		t.Errorf("Expected 5 items, got %d", itemCount)
	}
}

func TestLRUCache_TypeSafety(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()
	cache.Set("string", "value", 1*time.Hour)
	cache.Set("int", 42, 1*time.Hour)
	cache.Set("struct", struct{ Name string }{"test"}, 1*time.Hour)
	strVal, _ := cache.Get("string")
	if _, ok := strVal.(string); !ok {
		t.Error("Expected string type")
	}
	intVal, _ := cache.Get("int")
	if _, ok := intVal.(int); !ok {
		t.Error("Expected int type")
	}
}

func TestNewCacheService(t *testing.T) {
	service := NewCacheService()
	if service == nil {
		t.Fatal("NewCacheService should not return nil")
	}
	defer func() { _ = service.Close() }()
	if service.general == nil {
		t.Error("general cache should be initialized")
	}
	if service.quota == nil {
		t.Error("quota cache should be initialized")
	}
}

func TestCacheService_QuotaCache(t *testing.T) {
	service := NewCacheService()
	defer func() { _ = service.Close() }()
	quotaResponse := &core.JetbrainsQuotaResponse{Until: "1000"}
	quotaResponse.Current.Current.Amount = "100"
	quotaResponse.Current.Maximum.Amount = "1000"
	cacheKey := "quota:v1:test-license"
	service.SetQuotaCache(cacheKey, quotaResponse)
	result, found := service.GetQuotaCache(cacheKey)
	if !found {
		t.Error("配额缓存应该被找到")
	}
	if result == nil {
		t.Fatal("返回的配额数据不应该为nil")
	}
	if result.Current.Current.Amount != "100" {
		t.Errorf("配额当前值错误")
	}
	service.DeleteQuotaCache(cacheKey)
	_, found = service.GetQuotaCache(cacheKey)
	if found {
		t.Error("删除后不应该找到配额缓存")
	}
}

func TestCacheService_QuotaCacheDeepCopy(t *testing.T) {
	service := NewCacheService()
	defer func() { _ = service.Close() }()
	quotaResponse := &core.JetbrainsQuotaResponse{Until: "1000"}
	quotaResponse.Current.Current.Amount = "100"
	cacheKey := "quota:v1:test-deep-copy"
	service.SetQuotaCache(cacheKey, quotaResponse)
	result1, _ := service.GetQuotaCache(cacheKey)
	result1.Current.Current.Amount = "modified"
	result2, _ := service.GetQuotaCache(cacheKey)
	if result2.Current.Current.Amount == "modified" {
		t.Error("深拷贝失败")
	}
}

func TestCacheService_ClearQuotaCache(t *testing.T) {
	service := NewCacheService()
	defer func() { _ = service.Close() }()
	keys := []string{"quota:v1:license-1", "quota:v1:license-2", "quota:v1:license-3"}
	quotaResponse := &core.JetbrainsQuotaResponse{Until: "1000"}
	quotaResponse.Current.Current.Amount = "100"
	for _, key := range keys {
		service.SetQuotaCache(key, quotaResponse)
	}
	service.ClearQuotaCache()
	for _, key := range keys {
		_, found := service.GetQuotaCache(key)
		if found {
			t.Errorf("配额缓存 %s 应该被清除", key)
		}
	}
}

func TestCacheService_GetSet(t *testing.T) {
	service := NewCacheService()
	defer func() { _ = service.Close() }()
	service.Set("test-key", "test-value", 1*time.Hour)
	value, found := service.Get("test-key")
	if !found {
		t.Error("应该找到缓存值")
	}
	if value != "test-value" {
		t.Errorf("期望 'test-value'，实际 '%v'", value)
	}
}

func TestCacheService_Stop(t *testing.T) {
	service := NewCacheService()
	service.Set("key1", "value1", 1*time.Hour)
	service.Stop()
}

func TestCacheService_Close(t *testing.T) {
	service := NewCacheService()
	service.Set("key1", "value1", 1*time.Hour)
	_ = service.Close()
}

func TestGenerateMessagesCacheKey(t *testing.T) {
	messages1 := []core.ChatMessage{{Role: core.RoleUser, Content: "Hello"}}
	messages2 := []core.ChatMessage{{Role: core.RoleUser, Content: "Hello"}}
	messages3 := []core.ChatMessage{{Role: core.RoleUser, Content: "Different"}}
	key1 := GenerateMessagesCacheKey(messages1)
	key2 := GenerateMessagesCacheKey(messages2)
	key3 := GenerateMessagesCacheKey(messages3)
	if key1 != key2 {
		t.Error("相同消息应该生成相同的缓存键")
	}
	if key1 == key3 {
		t.Error("不同消息应该生成不同的缓存键")
	}
	if len(key1) == 0 {
		t.Error("缓存键不应该为空")
	}
}

func TestGenerateToolsCacheKey(t *testing.T) {
	tools1 := []core.Tool{{Type: core.ToolTypeFunction, Function: core.ToolFunction{Name: "func1"}}}
	tools2 := []core.Tool{{Type: core.ToolTypeFunction, Function: core.ToolFunction{Name: "func1"}}}
	tools3 := []core.Tool{{Type: core.ToolTypeFunction, Function: core.ToolFunction{Name: "func2"}}}
	key1 := GenerateToolsCacheKey(tools1)
	key2 := GenerateToolsCacheKey(tools2)
	key3 := GenerateToolsCacheKey(tools3)
	if key1 != key2 {
		t.Error("相同工具应该生成相同的缓存键")
	}
	if key1 == key3 {
		t.Error("不同工具应该生成不同的缓存键")
	}
}

func TestGenerateQuotaCacheKey(t *testing.T) {
	cs := NewCacheService()
	defer func() { _ = cs.Close() }()
	tests := []struct {
		name      string
		jwt       string
		licenseID string
	}{
		{"有LicenseID", "test-jwt", "test-license"},
		{"无LicenseID使用JWT", "test-jwt-1234567890", ""},
		{"短JWT", "short", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := cs.GenerateQuotaCacheKey(tt.jwt, tt.licenseID)
			if key == "" {
				t.Error("缓存键不应该为空")
			}
		})
	}
}

func TestMarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		wantErr bool
	}{
		{"简单字符串", "test", false},
		{"数组", []int{1, 2, 3}, false},
		{"结构体", struct{ Name string }{"test"}, false},
		{"Map", map[string]any{"key": "value"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := util.MarshalJSON(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("期望有错误")
				}
			} else {
				if err != nil {
					t.Errorf("不期望错误: %v", err)
				}
				if len(result) == 0 {
					t.Error("结果不应该为空")
				}
			}
		})
	}
}

func TestLRUCache_Clear(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()
	cache.Set("key1", "value1", 1*time.Hour)
	cache.Set("key2", "value2", 1*time.Hour)
	cache.Clear()
	_, found := cache.Get("key1")
	if found {
		t.Error("key1 应该被清除")
	}
}

func TestLRUCache_CleanupExpired(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()
	cache.Set("short", "value", 50*time.Millisecond)
	cache.Set("long", "value", 1*time.Hour)
	time.Sleep(100 * time.Millisecond)
	cache.cleanupExpired()
	_, found := cache.Get("short")
	if found {
		t.Error("short 应该被清理")
	}
	_, found = cache.Get("long")
	if !found {
		t.Error("long 应该仍然存在")
	}
}

func TestLRUCache_CleanupExpired_Empty(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()
	cache.cleanupExpired()
}

func TestLRUCache_Evict_EmptyCache(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()
	cache.mu.Lock()
	cache.evict()
	cache.mu.Unlock()
}

func TestGenerateMessagesCacheKey_WithToolCalls(t *testing.T) {
	messages1 := []core.ChatMessage{
		{Role: core.RoleAssistant, ToolCalls: []core.ToolCall{{ID: "call_1", Type: "function", Function: core.Function{Name: "get_weather", Arguments: `{"city":"Beijing"}`}}}},
	}
	messages2 := []core.ChatMessage{
		{Role: core.RoleAssistant, ToolCalls: []core.ToolCall{{ID: "call_1", Type: "function", Function: core.Function{Name: "get_weather", Arguments: `{"city":"Beijing"}`}}}},
	}
	messages3 := []core.ChatMessage{
		{Role: core.RoleAssistant, ToolCalls: []core.ToolCall{{ID: "call_2", Type: "function", Function: core.Function{Name: "get_weather", Arguments: `{"city":"Shanghai"}`}}}},
	}
	key1 := GenerateMessagesCacheKey(messages1)
	key2 := GenerateMessagesCacheKey(messages2)
	key3 := GenerateMessagesCacheKey(messages3)
	if key1 != key2 {
		t.Error("相同工具调用消息应该生成相同缓存键")
	}
	if key1 == key3 {
		t.Error("不同工具调用消息应该生成不同缓存键")
	}
}

func TestGenerateMessagesCacheKey_EmptyMessages(t *testing.T) {
	key := GenerateMessagesCacheKey([]core.ChatMessage{})
	if key == "" {
		t.Error("即使是空消息列表也应该生成缓存键")
	}
}

func TestGenerateToolsCacheKey_EmptyTools(t *testing.T) {
	key := GenerateToolsCacheKey([]core.Tool{})
	if key == "" {
		t.Error("即使是空工具列表也应该生成缓存键")
	}
}

func TestCacheService_QuotaCache_Expiration(t *testing.T) {
	// SetQuotaCache now uses fixed core.QuotaCacheTime (1 hour) TTL,
	// so short-duration expiration cannot be tested at CacheService level.
	// LRU-level TTL expiration is covered by TestLRUCache_Expiration.
	t.Skip("QuotaCache TTL is now fixed at core.QuotaCacheTime")
}

func TestTruncateCacheKey(t *testing.T) {
	tests := []struct {
		name, key, expected string
		maxLen              int
	}{
		{"短于限制不截断", "short", "short", 10},
		{"超过限制截断", "this_is_a_very_long_cache_key", "this_is_a_", 10},
		{"空字符串", "", "", 10},
		{"maxLen为0", "any", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateCacheKey(tt.key, tt.maxLen)
			if result != tt.expected {
				t.Errorf("期望 '%s'，实际 '%s'", tt.expected, result)
			}
		})
	}
}
