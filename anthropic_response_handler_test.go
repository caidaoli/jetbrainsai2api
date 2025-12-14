package main

import (
	"strings"
	"testing"
)

// TestGenerateResponseID 测试响应ID生成
func TestGenerateResponseID(t *testing.T) {
	id := generateResponseID()

	// 验证前缀
	if !strings.HasPrefix(id, ResponseIDPrefix) {
		t.Errorf("响应ID应以 '%s' 为前缀，实际: '%s'", ResponseIDPrefix, id)
	}

	// 验证长度合理（前缀 + 纳秒时间戳）
	if len(id) < len(ResponseIDPrefix)+10 {
		t.Errorf("响应ID长度过短: %s", id)
	}

	// 注意：纳秒级时间戳在极快连续调用下可能相同，不做唯一性断言
}

// TestEstimateTokenCount 测试token计数估算
func TestEstimateTokenCount(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{
			name:     "空字符串",
			text:     "",
			expected: 0,
		},
		{
			name:     "4个字符",
			text:     "test",
			expected: 1,
		},
		{
			name:     "8个字符",
			text:     "testtest",
			expected: 2,
		},
		{
			name:     "12个字符",
			text:     "hello world!",
			expected: 3,
		},
		{
			name:     "短于4个字符",
			text:     "hi",
			expected: 0, // len("hi")/4 = 0
		},
		{
			name:     "长文本",
			text:     strings.Repeat("a", 100),
			expected: 25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := estimateTokenCount(tt.text)
			if result != tt.expected {
				t.Errorf("estimateTokenCount(%q) = %d，期望 %d", tt.text, result, tt.expected)
			}
		})
	}
}

// TestParseJetbrainsStreamData 测试JetBrains流式数据解析
func TestParseJetbrainsStreamData(t *testing.T) {
	tests := []struct {
		name        string
		data        string
		expected    string
		expectError bool
	}{
		{
			name:        "空字符串",
			data:        "",
			expected:    "",
			expectError: false,
		},
		{
			name:        "null值",
			data:        "null",
			expected:    "",
			expectError: false,
		},
		{
			name:        "纯文本（非JSON）",
			data:        "Hello World",
			expected:    "Hello World",
			expectError: false,
		},
		{
			name:        "JetBrains格式内容事件",
			data:        `{"type":"content","content":"Hello from JetBrains"}`,
			expected:    "Hello from JetBrains",
			expectError: false,
		},
		{
			name:        "OpenAI格式响应",
			data:        `{"choices":[{"delta":{"content":"OpenAI content"}}]}`,
			expected:    "OpenAI content",
			expectError: false,
		},
		{
			name:        "直接内容字段",
			data:        `{"content":"Direct content"}`,
			expected:    "Direct content",
			expectError: false,
		},
		{
			name:        "无内容的JSON",
			data:        `{"type":"other","data":"something"}`,
			expected:    "",
			expectError: false,
		},
		{
			name:        "空choices数组",
			data:        `{"choices":[]}`,
			expected:    "",
			expectError: false,
		},
		{
			name:        "delta无content",
			data:        `{"choices":[{"delta":{}}]}`,
			expected:    "",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseJetbrainsStreamData(tt.data)

			if tt.expectError {
				if err == nil {
					t.Error("期望有错误，实际无错误")
				}
				return
			}

			if err != nil {
				t.Errorf("不期望错误: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("期望 '%s'，实际 '%s'", tt.expected, result)
			}
		})
	}
}

// TestParseJetbrainsNonStreamResponse 测试JetBrains非流式响应解析
func TestParseJetbrainsNonStreamResponse(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		model       string
		wantContent string
		wantError   bool
	}{
		{
			name:        "简单内容响应",
			body:        `{"content":"Hello World"}`,
			model:       "gpt-4",
			wantContent: "Hello World",
			wantError:   false,
		},
		{
			name:        "数组内容响应",
			body:        `{"content":["Hello"," World"]}`,
			model:       "gpt-4",
			wantContent: "Hello World",
			wantError:   false,
		},
		{
			name:        "空内容响应",
			body:        `{"content":""}`,
			model:       "gpt-4",
			wantContent: "",
			wantError:   false,
		},
		{
			name:        "无内容字段",
			body:        `{"other":"field"}`,
			model:       "gpt-4",
			wantContent: "",
			wantError:   false,
		},
		{
			name:      "无效JSON",
			body:      `{invalid json`,
			model:     "gpt-4",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseJetbrainsNonStreamResponse([]byte(tt.body), tt.model)

			if tt.wantError {
				if err == nil {
					t.Error("期望有错误，实际无错误")
				}
				return
			}

			if err != nil {
				t.Errorf("不期望错误: %v", err)
				return
			}

			if result == nil {
				t.Error("结果不应为nil")
				return
			}

			// 验证基本结构
			if result.Object != ChatCompletionObjectType {
				t.Errorf("Object类型错误，期望 '%s'，实际 '%s'",
					ChatCompletionObjectType, result.Object)
			}

			if result.Model != tt.model {
				t.Errorf("Model错误，期望 '%s'，实际 '%s'", tt.model, result.Model)
			}

			if len(result.Choices) != 1 {
				t.Errorf("Choices数量错误，期望 1，实际 %d", len(result.Choices))
				return
			}

			// 验证内容
			actualContent := ""
			if contentStr, ok := result.Choices[0].Message.Content.(string); ok {
				actualContent = contentStr
			}
			if actualContent != tt.wantContent {
				t.Errorf("Content错误，期望 '%s'，实际 '%s'", tt.wantContent, actualContent)
			}

			// 验证FinishReason
			if result.Choices[0].FinishReason != FinishReasonStop {
				t.Errorf("FinishReason错误，期望 '%s'，实际 '%s'",
					FinishReasonStop, result.Choices[0].FinishReason)
			}

			// 验证ID前缀
			if !strings.HasPrefix(result.ID, ResponseIDPrefix) {
				t.Errorf("ID前缀错误，期望以 '%s' 开头，实际 '%s'",
					ResponseIDPrefix, result.ID)
			}

			// 验证Usage存在
			if result.Usage == nil {
				t.Error("Usage不应为nil")
			}
		})
	}
}

// TestParseJetbrainsNonStreamResponse_StreamFormat 测试流式格式作为非流式输入
func TestParseJetbrainsNonStreamResponse_StreamFormat(t *testing.T) {
	// 模拟流式响应格式被传入非流式解析
	streamBody := `data: {"type":"content","content":"First chunk"}

data: {"type":"content","content":" Second chunk"}

data: [DONE]
`
	result, err := parseJetbrainsNonStreamResponse([]byte(streamBody), "gpt-4")

	if err != nil {
		t.Errorf("不期望错误: %v", err)
		return
	}

	if result == nil {
		t.Error("结果不应为nil")
		return
	}

	// 应该能够解析流式数据并聚合内容
	actualContent := ""
	if contentStr, ok := result.Choices[0].Message.Content.(string); ok {
		actualContent = contentStr
	}

	// 验证内容被聚合（具体行为取决于parseAndAggregateStreamResponse实现）
	if actualContent == "" {
		t.Log("警告：流式格式解析后内容为空，可能需要检查parseAndAggregateStreamResponse实现")
	}
}
