package main

import (
	"testing"
	"time"
)

// ============================================================================
// 性能基准测试
// 运行: go test -bench=. -benchmem
// ============================================================================

// BenchmarkCacheGet 测试缓存读取性能
func BenchmarkCacheGet(b *testing.B) {
	cache := NewCacheService()
	testData := []JetbrainsMessage{{Type: "user_message", Content: "test message"}}
	cache.Set("test_key", testData, 10*time.Minute)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cache.Get("test_key")
		}
	})
}

// BenchmarkCacheSet 测试缓存写入性能
func BenchmarkCacheSet(b *testing.B) {
	cache := NewCacheService()
	testData := []JetbrainsMessage{{Type: "user_message", Content: "test message"}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set("test_key", testData, 10*time.Minute)
	}
}

// BenchmarkMessageConversion 测试消息转换性能
func BenchmarkMessageConversion(b *testing.B) {
	messages := []ChatMessage{
		{Role: RoleSystem, Content: "You are a helpful assistant"},
		{Role: RoleUser, Content: "Hello, how are you?"},
		{Role: RoleAssistant, Content: "I'm doing well, thank you!"},
		{Role: RoleUser, Content: "Can you help me with coding?"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		openAIToJetbrainsMessages(messages)
	}
}

// BenchmarkMessageConversionParallel 测试并发消息转换性能
func BenchmarkMessageConversionParallel(b *testing.B) {
	messages := []ChatMessage{
		{Role: RoleSystem, Content: "You are a helpful assistant"},
		{Role: RoleUser, Content: "Hello, how are you?"},
		{Role: RoleAssistant, Content: "I'm doing well, thank you!"},
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			openAIToJetbrainsMessages(messages)
		}
	})
}

// BenchmarkToolsValidation 测试工具验证性能
func BenchmarkToolsValidation(b *testing.B) {
	cache := NewCacheService()
	metrics := newMockMetrics()
	logger := &NopLogger{}
	tools := []Tool{
		{
			Type: ToolTypeFunction,
			Function: ToolFunction{
				Name:        "get_weather",
				Description: "Get weather information",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{
							"type":        "string",
							"description": "City name",
						},
						"unit": map[string]any{
							"type": "string",
							"enum": []string{"celsius", "fahrenheit"},
						},
					},
					"required": []string{"location"},
				},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ValidateAndTransformToolsWithMetrics(tools, cache, metrics, logger)
	}
}

// BenchmarkLRUCacheOperations 测试 LRU 缓存操作性能
func BenchmarkLRUCacheOperations(b *testing.B) {
	cache := NewCache()

	b.Run("Set", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			cache.Set("key", "value", time.Hour)
		}
	})

	b.Run("Get", func(b *testing.B) {
		cache.Set("key", "value", time.Hour)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cache.Get("key")
		}
	})

	b.Run("GetParallel", func(b *testing.B) {
		cache.Set("key", "value", time.Hour)
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				cache.Get("key")
			}
		})
	})
}

// BenchmarkJSONMarshal 测试 JSON 序列化性能 (Sonic)
func BenchmarkJSONMarshal(b *testing.B) {
	resp := ChatCompletionResponse{
		ID:      "chatcmpl-123",
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   "gpt-4",
		Choices: []ChatCompletionChoice{
			{
				Index: 0,
				Message: ChatMessage{
					Role:    RoleAssistant,
					Content: "Hello! How can I help you today?",
				},
				FinishReason: FinishReasonStop,
			},
		},
		Usage: map[string]int{
			"prompt_tokens":     10,
			"completion_tokens": 20,
			"total_tokens":      30,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		marshalJSON(resp)
	}
}

// BenchmarkCacheKeyGeneration 测试缓存键生成性能
func BenchmarkCacheKeyGeneration(b *testing.B) {
	messages := []ChatMessage{
		{Role: RoleUser, Content: "Hello"},
		{Role: RoleAssistant, Content: "Hi there!"},
		{Role: RoleUser, Content: "How are you?"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		generateMessagesCacheKey(messages)
	}
}
