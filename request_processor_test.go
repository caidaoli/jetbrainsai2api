package main

import (
	"testing"
	"time"

	"github.com/bytedance/sonic"
)

// mockMetricsCollector 测试用的 MetricsCollector 实现
type mockMetricsCollector struct{}

func (m *mockMetricsCollector) RecordCacheHit()                             {}
func (m *mockMetricsCollector) RecordCacheMiss()                            {}
func (m *mockMetricsCollector) RecordToolValidation(duration time.Duration) {}
func (m *mockMetricsCollector) RecordRequest(success bool, responseTime int64, model, account string) {
}
func (m *mockMetricsCollector) RecordHTTPRequest(duration time.Duration)     {}
func (m *mockMetricsCollector) RecordHTTPError()                             {}
func (m *mockMetricsCollector) RecordAccountPoolWait(duration time.Duration) {}
func (m *mockMetricsCollector) RecordAccountPoolError()                      {}
func (m *mockMetricsCollector) UpdateSystemMetrics()                         {}
func (m *mockMetricsCollector) ResetWindow()                                 {}
func (m *mockMetricsCollector) GetQPS() float64                              { return 0 }
func (m *mockMetricsCollector) GetMetricsString() string                     { return "" }

func newMockMetrics() MetricsCollector {
	return &mockMetricsCollector{}
}

// mockLogger 测试用的 Logger 实现
type mockLogger struct{}

func (m *mockLogger) Debug(format string, args ...any) {}
func (m *mockLogger) Info(format string, args ...any)  {}
func (m *mockLogger) Warn(format string, args ...any)  {}
func (m *mockLogger) Error(format string, args ...any) {}
func (m *mockLogger) Fatal(format string, args ...any) {}

func newMockLogger() Logger {
	return &mockLogger{}
}

func TestRequestProcessor_ProcessMessages(t *testing.T) {
	processor := NewRequestProcessor(ModelsConfig{}, nil, NewCache(), newMockMetrics(), newMockLogger())

	tests := []struct {
		name           string
		messages       []ChatMessage
		expectedCount  int
		expectCacheHit bool
		runTwice       bool // 运行两次来测试缓存
	}{
		{
			name: "单一用户消息",
			messages: []ChatMessage{
				{Role: RoleUser, Content: "Hello"},
			},
			expectedCount:  1,
			expectCacheHit: false,
		},
		{
			name: "多个消息",
			messages: []ChatMessage{
				{Role: RoleSystem, Content: "You are a helpful assistant"},
				{Role: RoleUser, Content: "Hello"},
				{Role: RoleAssistant, Content: "Hi there!"},
			},
			expectedCount:  3,
			expectCacheHit: false,
		},
		{
			name: "测试缓存命中",
			messages: []ChatMessage{
				{Role: RoleUser, Content: "Test caching"},
			},
			expectedCount:  1,
			expectCacheHit: false,
			runTwice:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 第一次运行
			result := processor.ProcessMessages(tt.messages)

			if len(result.JetbrainsMessages) != tt.expectedCount {
				t.Errorf("期望 %d 个消息，实际 %d 个", tt.expectedCount, len(result.JetbrainsMessages))
			}

			if result.CacheHit != tt.expectCacheHit {
				t.Errorf("期望缓存命中=%v，实际=%v", tt.expectCacheHit, result.CacheHit)
			}

			// 如果需要测试缓存，运行第二次
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
	processor := NewRequestProcessor(ModelsConfig{}, nil, NewCache(), newMockMetrics(), newMockLogger())

	request := &ChatCompletionRequest{
		Model:    "gpt-4",
		Messages: []ChatMessage{{Role: RoleUser, Content: "Hello"}},
		Tools:    []Tool{}, // 没有工具
	}

	result := processor.ProcessTools(request)

	if result.Error != nil {
		t.Errorf("不应该有错误: %v", result.Error)
	}

	if !result.ValidatedDone {
		t.Error("应该标记为验证完成")
	}

	if len(result.Data) != 0 {
		t.Errorf("期望0个数据项，实际 %d 个", len(result.Data))
	}
}

func TestRequestProcessor_ProcessTools_WithTools(t *testing.T) {
	processor := NewRequestProcessor(ModelsConfig{}, nil, NewCache(), newMockMetrics(), newMockLogger())

	request := &ChatCompletionRequest{
		Model:    "gpt-4",
		Messages: []ChatMessage{{Role: RoleUser, Content: "What's the weather?"}},
		Tools: []Tool{
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
						},
						"required": []any{"location"},
					},
				},
			},
		},
	}

	result := processor.ProcessTools(request)

	if result.Error != nil {
		t.Errorf("不应该有错误: %v", result.Error)
	}

	if !result.ValidatedDone {
		t.Error("应该标记为验证完成")
	}

	// 应该有2个数据项：FQDN 和工具定义
	if len(result.Data) != 2 {
		t.Errorf("期望2个数据项，实际 %d 个", len(result.Data))
	}

	// 注意：ToolChoice 的设置已移到 handler 层（职责分离）
	// ProcessTools 不再修改传入的 request 参数
}

func TestRequestProcessor_BuildJetbrainsPayload(t *testing.T) {
	processor := NewRequestProcessor(ModelsConfig{}, nil, NewCache(), newMockMetrics(), newMockLogger())

	request := &ChatCompletionRequest{
		Model: "gpt-4",
		Messages: []ChatMessage{
			{Role: RoleUser, Content: "Hello"},
		},
	}

	jetbrainsMessages := []JetbrainsMessage{
		{Type: JetBrainsMessageTypeUser, Content: "Hello"},
	}

	data := []JetbrainsData{}

	payloadBytes, err := processor.BuildJetbrainsPayload(request, jetbrainsMessages, data)

	if err != nil {
		t.Errorf("构建 payload 不应该失败: %v", err)
	}

	if len(payloadBytes) == 0 {
		t.Error("payload 不应该为空")
	}

	// 验证 payload 可以被解析回去
	var payload JetbrainsPayload
	if err := sonic.Unmarshal(payloadBytes, &payload); err != nil {
		t.Errorf("payload 应该是有效的 JSON: %v", err)
	}

	if payload.Prompt != JetBrainsChatPrompt {
		t.Errorf("期望 prompt 为 '%s'，实际 '%s'", JetBrainsChatPrompt, payload.Prompt)
	}

	if len(payload.Chat.Messages) != 1 {
		t.Errorf("期望1个消息，实际 %d 个", len(payload.Chat.Messages))
	}
}

func TestRequestProcessor_ProcessMessages_WithImageContent(t *testing.T) {
	processor := NewRequestProcessor(ModelsConfig{}, nil, NewCache(), newMockMetrics(), newMockLogger())

	messages := []ChatMessage{
		{
			Role: RoleUser,
			Content: []any{
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "What's in this image?",
				},
				map[string]any{
					"type": "image_url",
					"image_url": map[string]any{
						"url": "data:image/png;base64,iVBORw0KGgoAAAANSU",
					},
				},
			},
		},
	}

	result := processor.ProcessMessages(messages)

	// 应该至少有一个消息被处理
	if len(result.JetbrainsMessages) == 0 {
		t.Error("期望至少有一个消息被处理")
	}

	// 验证类型正确
	for _, msg := range result.JetbrainsMessages {
		if msg.Type != JetBrainsMessageTypeUser && msg.Type != JetBrainsMessageTypeMedia {
			t.Errorf("消息类型不正确: %s", msg.Type)
		}
	}
}

func TestRequestProcessor_ProcessTools_Caching(t *testing.T) {
	// 使用新的缓存实例进行测试
	testCache := NewCache()
	processor := NewRequestProcessor(ModelsConfig{}, nil, testCache, newMockMetrics(), newMockLogger())

	request := &ChatCompletionRequest{
		Model: "gpt-4",
		Tools: []Tool{
			{
				Type: "function",
				Function: ToolFunction{
					Name:        "test_func",
					Description: "Test function",
					Parameters: map[string]any{
						"type":       "object",
						"properties": map[string]any{},
					},
				},
			},
		},
	}

	// 第一次调用 - 应该不命中缓存
	startTime := time.Now()
	result1 := processor.ProcessTools(request)
	duration1 := time.Since(startTime)

	if result1.Error != nil {
		t.Errorf("第一次调用不应该失败: %v", result1.Error)
	}

	// 第二次调用 - 应该命中缓存（更快）
	startTime = time.Now()
	result2 := processor.ProcessTools(request)
	duration2 := time.Since(startTime)

	if result2.Error != nil {
		t.Errorf("第二次调用不应该失败: %v", result2.Error)
	}

	// 第二次应该更快（命中缓存）
	// 注意：这个测试可能在某些情况下不稳定，但通常缓存命中应该快得多
	if duration2 > duration1 {
		t.Logf("警告: 第二次调用（%v）比第一次（%v）慢，可能未命中缓存", duration2, duration1)
	}

	// 验证结果一致
	if len(result1.Data) != len(result2.Data) {
		t.Error("两次调用应该返回相同数量的数据")
	}
}
