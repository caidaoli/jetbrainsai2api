package main

import (
	"strings"
	"testing"
)

// TestGenerateRandomID 测试随机ID生成
func TestGenerateRandomID(t *testing.T) {
	id := generateRandomID(ToolCallIDPrefix)

	// 验证前缀
	if !strings.HasPrefix(id, ToolCallIDPrefix) {
		t.Errorf("工具调用ID应以 '%s' 为前缀，实际: '%s'", ToolCallIDPrefix, id)
	}

	// 验证长度：前缀(6) + hex编码(20) = 26
	expectedLen := len(ToolCallIDPrefix) + 20
	if len(id) != expectedLen {
		t.Errorf("工具调用ID长度应为 %d，实际: %d", expectedLen, len(id))
	}

	// 验证hex部分只包含有效字符
	hexPart := id[len(ToolCallIDPrefix):]
	for _, c := range hexPart {
		isDigit := c >= '0' && c <= '9'
		isHexLower := c >= 'a' && c <= 'f'
		if !isDigit && !isHexLower {
			t.Errorf("hex部分包含无效字符: %c", c)
		}
	}

	// 验证多次生成的ID不同（概率上）
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		newID := generateRandomID(ToolCallIDPrefix)
		if ids[newID] {
			t.Errorf("生成了重复的ID: %s", newID)
		}
		ids[newID] = true
	}
}

// TestMapJetbrainsToOpenAIFinishReason 测试JetBrains到OpenAI结束原因映射
func TestMapJetbrainsToOpenAIFinishReason(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "tool_call映射到tool_calls",
			input:    JetBrainsFinishReasonToolCall,
			expected: FinishReasonToolCalls,
		},
		{
			name:     "length映射到length",
			input:    JetBrainsFinishReasonLength,
			expected: FinishReasonLength,
		},
		{
			name:     "stop映射到stop",
			input:    JetBrainsFinishReasonStop,
			expected: FinishReasonStop,
		},
		{
			name:     "未知值默认映射到stop",
			input:    "unknown_reason",
			expected: FinishReasonStop,
		},
		{
			name:     "空字符串默认映射到stop",
			input:    "",
			expected: FinishReasonStop,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapJetbrainsToOpenAIFinishReason(tt.input)
			if result != tt.expected {
				t.Errorf("期望 '%s'，实际 '%s'", tt.expected, result)
			}
		})
	}
}

// TestStringPtr 测试字符串指针辅助函数
func TestStringPtr(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"普通字符串", "hello"},
		{"空字符串", ""},
		{"中文字符串", "你好世界"},
		{"特殊字符", "!@#$%^&*()"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stringPtr(tt.input)

			// 验证返回的是指针
			if result == nil {
				t.Error("返回值不应为nil")
				return
			}

			// 验证指针指向的值正确
			if *result != tt.input {
				t.Errorf("期望 '%s'，实际 '%s'", tt.input, *result)
			}

			// 验证修改指针不影响原值
			original := tt.input
			*result = "modified"
			if original != tt.input {
				t.Error("修改指针值不应影响原始值")
			}
		})
	}
}
