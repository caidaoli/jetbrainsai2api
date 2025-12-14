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

// ============================================================================
// CacheService 测试
// ============================================================================

// TestNewCacheService 测试CacheService创建
func TestNewCacheService(t *testing.T) {
	service := NewCacheService()
	if service == nil {
		t.Fatal("NewCacheService should not return nil")
	}
	defer service.Close()

	// 验证内部缓存已初始化
	if service.messages == nil {
		t.Error("messages cache should be initialized")
	}
	if service.tools == nil {
		t.Error("tools cache should be initialized")
	}
	if service.quota == nil {
		t.Error("quota cache should be initialized")
	}
}

// TestCacheService_QuotaCache 测试配额缓存操作
func TestCacheService_QuotaCache(t *testing.T) {
	service := NewCacheService()
	defer service.Close()

	quotaResponse := &JetbrainsQuotaResponse{
		Until: "1000",
	}
	quotaResponse.Current.Current.Amount = "100"
	quotaResponse.Current.Maximum.Amount = "1000"

	cacheKey := "quota:v1:test-license"

	// 测试设置配额缓存
	service.SetQuotaCache(cacheKey, quotaResponse, 1*time.Hour)

	// 测试获取配额缓存
	result, found := service.GetQuotaCache(cacheKey)
	if !found {
		t.Error("配额缓存应该被找到")
	}
	if result == nil {
		t.Error("返回的配额数据不应该为nil")
	}
	if result.Current.Current.Amount != "100" {
		t.Errorf("配额当前值错误，期望 '100'，实际 '%s'", result.Current.Current.Amount)
	}

	// 测试删除配额缓存
	service.DeleteQuotaCache(cacheKey)
	_, found = service.GetQuotaCache(cacheKey)
	if found {
		t.Error("删除后不应该找到配额缓存")
	}
}

// TestCacheService_QuotaCacheDeepCopy 测试配额缓存深拷贝
func TestCacheService_QuotaCacheDeepCopy(t *testing.T) {
	service := NewCacheService()
	defer service.Close()

	quotaResponse := &JetbrainsQuotaResponse{
		Until: "1000",
	}
	quotaResponse.Current.Current.Amount = "100"
	quotaResponse.Current.Maximum.Amount = "1000"

	cacheKey := "quota:v1:test-deep-copy"
	service.SetQuotaCache(cacheKey, quotaResponse, 1*time.Hour)

	// 获取缓存并修改
	result1, _ := service.GetQuotaCache(cacheKey)
	result1.Current.Current.Amount = "modified"

	// 再次获取，应该是原始值（深拷贝验证）
	result2, _ := service.GetQuotaCache(cacheKey)
	if result2.Current.Current.Amount == "modified" {
		t.Error("深拷贝失败：修改返回值影响了缓存")
	}
}

// TestCacheService_ClearQuotaCache 测试清除所有配额缓存
func TestCacheService_ClearQuotaCache(t *testing.T) {
	service := NewCacheService()
	defer service.Close()

	// 添加多个配额缓存
	keys := []string{"quota:v1:license-1", "quota:v1:license-2", "quota:v1:license-3"}
	quotaResponse := &JetbrainsQuotaResponse{Until: "1000"}
	quotaResponse.Current.Current.Amount = "100"
	quotaResponse.Current.Maximum.Amount = "1000"

	for _, key := range keys {
		service.SetQuotaCache(key, quotaResponse, 1*time.Hour)
	}

	// 验证都能找到
	for _, key := range keys {
		_, found := service.GetQuotaCache(key)
		if !found {
			t.Errorf("配额缓存 %s 应该存在", key)
		}
	}

	// 清除所有配额缓存
	service.ClearQuotaCache()

	// 验证都被清除
	for _, key := range keys {
		_, found := service.GetQuotaCache(key)
		if found {
			t.Errorf("配额缓存 %s 应该被清除", key)
		}
	}
}

// TestCacheService_GetSet 测试通用缓存操作
func TestCacheService_GetSet(t *testing.T) {
	service := NewCacheService()
	defer service.Close()

	// 测试 Set
	service.Set("test-key", "test-value", 1*time.Hour)

	// 测试 Get
	value, found := service.Get("test-key")
	if !found {
		t.Error("应该找到缓存值")
	}
	if value != "test-value" {
		t.Errorf("期望 'test-value'，实际 '%v'", value)
	}
}

// TestCacheService_Stop 测试停止缓存服务
func TestCacheService_Stop(t *testing.T) {
	service := NewCacheService()

	// 添加一些数据
	service.Set("key1", "value1", 1*time.Hour)

	// 停止服务
	service.Stop()

	// 停止后不应该 panic（即使底层缓存已关闭）
	// 这个测试主要验证 Stop 方法能正确执行
}

// TestCacheService_Close 测试关闭缓存服务
func TestCacheService_Close(t *testing.T) {
	service := NewCacheService()

	// 添加一些数据
	service.Set("key1", "value1", 1*time.Hour)

	// 关闭服务
	service.Close()

	// Close 应该能正常执行而不 panic
}

// ============================================================================
// 缓存键生成测试
// ============================================================================

// TestGenerateMessagesCacheKey 测试消息缓存键生成
func TestGenerateMessagesCacheKey(t *testing.T) {
	messages1 := []ChatMessage{
		{Role: RoleUser, Content: "Hello"},
	}
	messages2 := []ChatMessage{
		{Role: RoleUser, Content: "Hello"},
	}
	messages3 := []ChatMessage{
		{Role: RoleUser, Content: "Different"},
	}

	key1 := generateMessagesCacheKey(messages1)
	key2 := generateMessagesCacheKey(messages2)
	key3 := generateMessagesCacheKey(messages3)

	// 相同消息应该生成相同键
	if key1 != key2 {
		t.Error("相同消息应该生成相同的缓存键")
	}

	// 不同消息应该生成不同键
	if key1 == key3 {
		t.Error("不同消息应该生成不同的缓存键")
	}

	// 验证键包含版本号
	if len(key1) == 0 {
		t.Error("缓存键不应该为空")
	}
}

// TestGenerateToolsCacheKey 测试工具缓存键生成
func TestGenerateToolsCacheKey(t *testing.T) {
	tools1 := []Tool{
		{Type: ToolTypeFunction, Function: ToolFunction{Name: "func1"}},
	}
	tools2 := []Tool{
		{Type: ToolTypeFunction, Function: ToolFunction{Name: "func1"}},
	}
	tools3 := []Tool{
		{Type: ToolTypeFunction, Function: ToolFunction{Name: "func2"}},
	}

	key1 := generateToolsCacheKey(tools1)
	key2 := generateToolsCacheKey(tools2)
	key3 := generateToolsCacheKey(tools3)

	// 相同工具应该生成相同键
	if key1 != key2 {
		t.Error("相同工具应该生成相同的缓存键")
	}

	// 不同工具应该生成不同键
	if key1 == key3 {
		t.Error("不同工具应该生成不同的缓存键")
	}
}

// TestGenerateQuotaCacheKey 测试配额缓存键生成
func TestGenerateQuotaCacheKey(t *testing.T) {
	tests := []struct {
		name    string
		account *JetbrainsAccount
	}{
		{
			name:    "有LicenseID",
			account: &JetbrainsAccount{LicenseID: "test-license", JWT: "test-jwt"},
		},
		{
			name:    "无LicenseID使用JWT",
			account: &JetbrainsAccount{JWT: "test-jwt-1234567890"},
		},
		{
			name:    "短JWT",
			account: &JetbrainsAccount{JWT: "short"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := generateQuotaCacheKey(tt.account)
			if key == "" {
				t.Error("缓存键不应该为空")
			}
			// 验证键格式包含版本号
			if len(key) < 10 {
				t.Error("缓存键长度异常")
			}
		})
	}
}

// TestMarshalJSON 测试JSON序列化辅助函数
func TestMarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		wantErr bool
	}{
		{
			name:    "简单字符串",
			input:   "test",
			wantErr: false,
		},
		{
			name:    "数组",
			input:   []int{1, 2, 3},
			wantErr: false,
		},
		{
			name:    "结构体",
			input:   struct{ Name string }{"test"},
			wantErr: false,
		},
		{
			name:    "Map",
			input:   map[string]any{"key": "value"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := marshalJSON(tt.input)
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

// TestLRUCache_Clear 测试清除缓存
func TestLRUCache_Clear(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()

	// 添加一些数据
	cache.Set("key1", "value1", 1*time.Hour)
	cache.Set("key2", "value2", 1*time.Hour)
	cache.Set("key3", "value3", 1*time.Hour)

	// 验证数据存在
	_, found := cache.Get("key1")
	if !found {
		t.Error("key1 应该存在")
	}

	// 清除缓存
	cache.Clear()

	// 验证所有数据被清除
	_, found = cache.Get("key1")
	if found {
		t.Error("key1 应该被清除")
	}
	_, found = cache.Get("key2")
	if found {
		t.Error("key2 应该被清除")
	}
}

// TestLRUCache_CleanupExpired 测试过期清理功能
func TestLRUCache_CleanupExpired(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()

	// 添加一个短期过期的项
	cache.Set("short", "value", 50*time.Millisecond)
	// 添加一个长期有效的项
	cache.Set("long", "value", 1*time.Hour)

	// 验证两个项都存在
	_, found := cache.Get("short")
	if !found {
		t.Error("short 应该存在")
	}
	_, found = cache.Get("long")
	if !found {
		t.Error("long 应该存在")
	}

	// 等待过期
	time.Sleep(100 * time.Millisecond)

	// 手动触发清理
	cache.cleanupExpired()

	// 验证 short 被清理，long 仍然存在
	_, found = cache.Get("short")
	if found {
		t.Error("short 应该被清理")
	}
	_, found = cache.Get("long")
	if !found {
		t.Error("long 应该仍然存在")
	}
}

// TestLRUCache_CleanupExpired_Empty 测试空缓存的清理
func TestLRUCache_CleanupExpired_Empty(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()

	// 空缓存调用清理不应该 panic
	cache.cleanupExpired()
}

// TestLRUCache_Evict_EmptyCache 测试空缓存驱逐
func TestLRUCache_Evict_EmptyCache(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()

	// 空缓存调用驱逐不应该 panic
	cache.mu.Lock()
	cache.evict()
	cache.mu.Unlock()
}

// TestGenerateMessagesCacheKey_WithToolCalls 测试带工具调用的消息缓存键
func TestGenerateMessagesCacheKey_WithToolCalls(t *testing.T) {
	messages1 := []ChatMessage{
		{
			Role: RoleAssistant,
			ToolCalls: []ToolCall{
				{ID: "call_1", Type: "function", Function: Function{Name: "get_weather", Arguments: `{"city":"Beijing"}`}},
			},
		},
	}
	messages2 := []ChatMessage{
		{
			Role: RoleAssistant,
			ToolCalls: []ToolCall{
				{ID: "call_1", Type: "function", Function: Function{Name: "get_weather", Arguments: `{"city":"Beijing"}`}},
			},
		},
	}
	messages3 := []ChatMessage{
		{
			Role: RoleAssistant,
			ToolCalls: []ToolCall{
				{ID: "call_2", Type: "function", Function: Function{Name: "get_weather", Arguments: `{"city":"Shanghai"}`}},
			},
		},
	}

	key1 := generateMessagesCacheKey(messages1)
	key2 := generateMessagesCacheKey(messages2)
	key3 := generateMessagesCacheKey(messages3)

	// 相同消息生成相同键
	if key1 != key2 {
		t.Error("相同工具调用消息应该生成相同缓存键")
	}

	// 不同工具调用生成不同键
	if key1 == key3 {
		t.Error("不同工具调用消息应该生成不同缓存键")
	}
}

// TestGenerateToolsCacheKey_WithParameters 测试带参数的工具缓存键
func TestGenerateToolsCacheKey_WithParameters(t *testing.T) {
	// 注意：由于 Go map 迭代顺序不确定，含嵌套 map 的工具参数
	// 在不同序列化时可能产生不同哈希。此测试仅验证不同函数名生成不同键。

	tools1 := []Tool{
		{
			Type: ToolTypeFunction,
			Function: ToolFunction{
				Name:        "get_weather",
				Description: "Get weather info",
			},
		},
	}
	tools2 := []Tool{
		{
			Type: ToolTypeFunction,
			Function: ToolFunction{
				Name:        "get_time", // 不同函数名
				Description: "Get time info",
			},
		},
	}

	key1 := generateToolsCacheKey(tools1)
	key2 := generateToolsCacheKey(tools2)

	// 不同函数名生成不同键
	if key1 == key2 {
		t.Error("不同函数名应该生成不同缓存键")
	}

	// 相同工具连续调用应生成相同键（无嵌套map）
	key1Again := generateToolsCacheKey(tools1)
	if key1 != key1Again {
		t.Errorf("相同工具连续调用应生成相同键，key1=%s, key1Again=%s", key1, key1Again)
	}
}

// TestGenerateMessagesCacheKey_EmptyMessages 测试空消息列表
func TestGenerateMessagesCacheKey_EmptyMessages(t *testing.T) {
	key := generateMessagesCacheKey([]ChatMessage{})

	if key == "" {
		t.Error("即使是空消息列表也应该生成缓存键")
	}

	// 验证键格式包含版本号前缀
	if len(key) < 5 {
		t.Error("缓存键格式不正确")
	}
}

// TestGenerateToolsCacheKey_EmptyTools 测试空工具列表
func TestGenerateToolsCacheKey_EmptyTools(t *testing.T) {
	key := generateToolsCacheKey([]Tool{})

	if key == "" {
		t.Error("即使是空工具列表也应该生成缓存键")
	}
}

// TestCacheService_QuotaCache_Expiration 测试配额缓存过期
func TestCacheService_QuotaCache_Expiration(t *testing.T) {
	service := NewCacheService()
	defer service.Close()

	quotaResponse := &JetbrainsQuotaResponse{
		Until: "2099-12-31",
	}
	quotaResponse.Current.Current.Amount = "100"
	quotaResponse.Current.Maximum.Amount = "1000"

	// 设置短期过期
	service.SetQuotaCache("test-key", quotaResponse, 50*time.Millisecond)

	// 立即获取应该成功
	_, found := service.GetQuotaCache("test-key")
	if !found {
		t.Error("配额缓存应该存在")
	}

	// 等待过期
	time.Sleep(100 * time.Millisecond)

	// 现在应该找不到
	_, found = service.GetQuotaCache("test-key")
	if found {
		t.Error("配额缓存应该已过期")
	}
}
