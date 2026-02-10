package convert

import (
	"testing"
	"time"

	"jetbrainsai2api/internal/cache"
	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/util"
	"jetbrainsai2api/internal/validate"
)

func BenchmarkCacheGet(b *testing.B) {
	c := cache.NewCacheService()
	testData := []core.JetbrainsMessage{{Type: "user_message", Content: "test message"}}
	c.Set("test_key", testData, 10*time.Minute)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Get("test_key")
		}
	})
}

func BenchmarkCacheSet(b *testing.B) {
	c := cache.NewCacheService()
	testData := []core.JetbrainsMessage{{Type: "user_message", Content: "test message"}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set("test_key", testData, 10*time.Minute)
	}
}

func BenchmarkMessageConversion(b *testing.B) {
	messages := []core.ChatMessage{
		{Role: core.RoleSystem, Content: "You are a helpful assistant"},
		{Role: core.RoleUser, Content: "Hello, how are you?"},
		{Role: core.RoleAssistant, Content: "I'm doing well, thank you!"},
		{Role: core.RoleUser, Content: "Can you help me with coding?"},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		OpenAIToJetbrainsMessages(messages)
	}
}

func BenchmarkMessageConversionParallel(b *testing.B) {
	messages := []core.ChatMessage{
		{Role: core.RoleSystem, Content: "You are a helpful assistant"},
		{Role: core.RoleUser, Content: "Hello, how are you?"},
		{Role: core.RoleAssistant, Content: "I'm doing well, thank you!"},
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			OpenAIToJetbrainsMessages(messages)
		}
	})
}

func BenchmarkToolsValidation(b *testing.B) {
	logger := &core.NopLogger{}
	tools := []core.Tool{
		{
			Type: core.ToolTypeFunction,
			Function: core.ToolFunction{
				Name:        "get_weather",
				Description: "Get weather information",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{"type": "string", "description": "City name"},
						"unit":     map[string]any{"type": "string", "enum": []string{"celsius", "fahrenheit"}},
					},
					"required": []string{"location"},
				},
			},
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = validate.ValidateAndTransformTools(tools, logger)
	}
}

func BenchmarkLRUCacheOperations(b *testing.B) {
	c := cache.NewCache()
	b.Run("Set", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			c.Set("key", "value", time.Hour)
		}
	})
	b.Run("Get", func(b *testing.B) {
		c.Set("key", "value", time.Hour)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c.Get("key")
		}
	})
	b.Run("GetParallel", func(b *testing.B) {
		c.Set("key", "value", time.Hour)
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				c.Get("key")
			}
		})
	})
}

func BenchmarkJSONMarshal(b *testing.B) {
	resp := core.ChatCompletionResponse{
		ID:      "chatcmpl-123",
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   "gpt-4",
		Choices: []core.ChatCompletionChoice{
			{Index: 0, Message: core.ChatMessage{Role: core.RoleAssistant, Content: "Hello!"}, FinishReason: core.FinishReasonStop},
		},
		Usage: core.OpenAIUsage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = util.MarshalJSON(resp)
	}
}

func BenchmarkCacheKeyGeneration(b *testing.B) {
	messages := []core.ChatMessage{
		{Role: core.RoleUser, Content: "Hello"},
		{Role: core.RoleAssistant, Content: "Hi there!"},
		{Role: core.RoleUser, Content: "How are you?"},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.GenerateMessagesCacheKey(messages)
	}
}
