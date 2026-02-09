package process

import (
	"testing"
	"time"

	"jetbrainsai2api/internal/cache"
	"jetbrainsai2api/internal/core"

	"github.com/bytedance/sonic"
)

func TestRequestProcessor_ProcessMessages(t *testing.T) {
	c := cache.NewCacheService()
	defer func() { _ = c.Close() }()
	processor := NewRequestProcessor(core.ModelsConfig{}, nil, c, &core.NopMetrics{}, &core.NopLogger{})

	tests := []struct {
		name           string
		messages       []core.ChatMessage
		expectedCount  int
		expectCacheHit bool
		runTwice       bool
	}{
		{"单一用户消息", []core.ChatMessage{{Role: core.RoleUser, Content: "Hello"}}, 1, false, false},
		{"多个消息", []core.ChatMessage{
			{Role: core.RoleSystem, Content: "You are a helpful assistant"},
			{Role: core.RoleUser, Content: "Hello"},
			{Role: core.RoleAssistant, Content: "Hi there!"},
		}, 3, false, false},
		{"测试缓存命中", []core.ChatMessage{{Role: core.RoleUser, Content: "Test caching"}}, 1, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.ProcessMessages(tt.messages)
			if len(result.JetbrainsMessages) != tt.expectedCount {
				t.Errorf("期望 %d 个消息，实际 %d 个", tt.expectedCount, len(result.JetbrainsMessages))
			}
			if result.CacheHit != tt.expectCacheHit {
				t.Errorf("期望缓存命中=%v，实际=%v", tt.expectCacheHit, result.CacheHit)
			}
			if tt.runTwice {
				result2 := processor.ProcessMessages(tt.messages)
				if !result2.CacheHit {
					t.Error("第二次运行应该命中缓存，但没有")
				}
			}
		})
	}
}

func TestRequestProcessor_ProcessTools_NoTools(t *testing.T) {
	c := cache.NewCacheService()
	defer func() { _ = c.Close() }()
	processor := NewRequestProcessor(core.ModelsConfig{}, nil, c, &core.NopMetrics{}, &core.NopLogger{})

	request := &core.ChatCompletionRequest{
		Model:    "gpt-4",
		Messages: []core.ChatMessage{{Role: core.RoleUser, Content: "Hello"}},
		Tools:    []core.Tool{},
	}

	result := processor.ProcessTools(request)
	if result.Error != nil { t.Errorf("不应该有错误: %v", result.Error) }
	if !result.ValidatedDone { t.Error("应该标记为验证完成") }
	if len(result.Data) != 0 { t.Errorf("期望0个数据项，实际 %d 个", len(result.Data)) }
}

func TestRequestProcessor_ProcessTools_WithTools(t *testing.T) {
	c := cache.NewCacheService()
	defer func() { _ = c.Close() }()
	processor := NewRequestProcessor(core.ModelsConfig{}, nil, c, &core.NopMetrics{}, &core.NopLogger{})

	request := &core.ChatCompletionRequest{
		Model:    "gpt-4",
		Messages: []core.ChatMessage{{Role: core.RoleUser, Content: "What's the weather?"}},
		Tools: []core.Tool{
			{
				Type: core.ToolTypeFunction,
				Function: core.ToolFunction{
					Name:        "get_weather",
					Description: "Get weather information",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{"type": "string", "description": "City name"},
						},
						"required": []any{"location"},
					},
				},
			},
		},
	}

	result := processor.ProcessTools(request)
	if result.Error != nil { t.Errorf("不应该有错误: %v", result.Error) }
	if !result.ValidatedDone { t.Error("应该标记为验证完成") }
	if len(result.Data) != 2 { t.Errorf("期望2个数据项，实际 %d 个", len(result.Data)) }
}

func TestRequestProcessor_BuildJetbrainsPayload(t *testing.T) {
	c := cache.NewCacheService()
	defer func() { _ = c.Close() }()
	processor := NewRequestProcessor(core.ModelsConfig{}, nil, c, &core.NopMetrics{}, &core.NopLogger{})

	request := &core.ChatCompletionRequest{
		Model:    "gpt-4",
		Messages: []core.ChatMessage{{Role: core.RoleUser, Content: "Hello"}},
	}
	jetbrainsMessages := []core.JetbrainsMessage{{Type: core.JetBrainsMessageTypeUser, Content: "Hello"}}
	data := []core.JetbrainsData{}

	payloadBytes, err := processor.BuildJetbrainsPayload(request, jetbrainsMessages, data)
	if err != nil { t.Errorf("构建 payload 不应该失败: %v", err) }
	if len(payloadBytes) == 0 { t.Error("payload 不应该为空") }

	var payload core.JetbrainsPayload
	if err := sonic.Unmarshal(payloadBytes, &payload); err != nil {
		t.Errorf("payload 应该是有效的 JSON: %v", err)
	}
	if payload.Prompt != core.JetBrainsChatPrompt {
		t.Errorf("期望 prompt 为 '%s'，实际 '%s'", core.JetBrainsChatPrompt, payload.Prompt)
	}
	if len(payload.Chat.Messages) != 1 {
		t.Errorf("期望1个消息，实际 %d 个", len(payload.Chat.Messages))
	}
}

func TestRequestProcessor_ProcessMessages_WithImageContent(t *testing.T) {
	c := cache.NewCacheService()
	defer func() { _ = c.Close() }()
	processor := NewRequestProcessor(core.ModelsConfig{}, nil, c, &core.NopMetrics{}, &core.NopLogger{})

	messages := []core.ChatMessage{
		{
			Role: core.RoleUser,
			Content: []any{
				map[string]any{"type": core.ContentBlockTypeText, "text": "What's in this image?"},
				map[string]any{
					"type":      "image_url",
					"image_url": map[string]any{"url": "data:image/png;base64,iVBORw0KGgoAAAANSU"},
				},
			},
		},
	}

	result := processor.ProcessMessages(messages)
	if len(result.JetbrainsMessages) == 0 {
		t.Error("期望至少有一个消息被处理")
	}
}

func TestRequestProcessor_ProcessTools_Caching(t *testing.T) {
	testCache := cache.NewCacheService()
	defer func() { _ = testCache.Close() }()
	processor := NewRequestProcessor(core.ModelsConfig{}, nil, testCache, &core.NopMetrics{}, &core.NopLogger{})

	request := &core.ChatCompletionRequest{
		Model: "gpt-4",
		Tools: []core.Tool{
			{
				Type: core.ToolTypeFunction,
				Function: core.ToolFunction{
					Name:        "test_func",
					Description: "Test function",
					Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
				},
			},
		},
	}

	startTime := time.Now()
	result1 := processor.ProcessTools(request)
	duration1 := time.Since(startTime)

	if result1.Error != nil { t.Errorf("第一次调用不应该失败: %v", result1.Error) }

	startTime = time.Now()
	result2 := processor.ProcessTools(request)
	duration2 := time.Since(startTime)

	if result2.Error != nil { t.Errorf("第二次调用不应该失败: %v", result2.Error) }

	if duration2 > duration1 {
		t.Logf("警告: 第二次调用（%v）比第一次（%v）慢", duration2, duration1)
	}
	if len(result1.Data) != len(result2.Data) { t.Error("两次调用应该返回相同数量的数据") }
}

func TestGetInternalModelName(t *testing.T) {
	config := core.ModelsConfig{
		Models: map[string]string{
			"gpt-4":    "openai-gpt-4",
			"claude-3": "anthropic-claude-3",
		},
	}

	tests := []struct {
		name, modelID, expected string
	}{
		{"已映射的模型", "gpt-4", "openai-gpt-4"},
		{"未映射的模型", "unknown-model", "unknown-model"},
		{"空模型ID", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetInternalModelName(config, tt.modelID)
			if result != tt.expected { t.Errorf("期望 '%s'，实际 '%s'", tt.expected, result) }
		})
	}
}
