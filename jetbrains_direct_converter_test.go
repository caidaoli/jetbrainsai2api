package main

import (
	"testing"
)

// TestMapJetbrainsFinishReason 测试JetBrains到Anthropic结束原因映射
func TestMapJetbrainsFinishReason(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "tool_call映射到tool_use",
			input:    JetBrainsFinishReasonToolCall,
			expected: StopReasonToolUse,
		},
		{
			name:     "length映射到max_tokens",
			input:    JetBrainsFinishReasonLength,
			expected: StopReasonMaxTokens,
		},
		{
			name:     "stop映射到end_turn",
			input:    JetBrainsFinishReasonStop,
			expected: StopReasonEndTurn,
		},
		{
			name:     "未知值默认映射到end_turn",
			input:    "unknown",
			expected: StopReasonEndTurn,
		},
		{
			name:     "空字符串默认映射到end_turn",
			input:    "",
			expected: StopReasonEndTurn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapJetbrainsFinishReason(tt.input)
			if result != tt.expected {
				t.Errorf("期望 '%s'，实际 '%s'", tt.expected, result)
			}
		})
	}
}

// TestGetContentText 测试从内容块提取文本
func TestGetContentText(t *testing.T) {
	tests := []struct {
		name     string
		content  []AnthropicContentBlock
		expected string
	}{
		{
			name:     "空内容",
			content:  []AnthropicContentBlock{},
			expected: "",
		},
		{
			name:     "nil内容",
			content:  nil,
			expected: "",
		},
		{
			name: "单个text块",
			content: []AnthropicContentBlock{
				{Type: ContentBlockTypeText, Text: "Hello World"},
			},
			expected: "Hello World",
		},
		{
			name: "多个text块",
			content: []AnthropicContentBlock{
				{Type: ContentBlockTypeText, Text: "First"},
				{Type: ContentBlockTypeText, Text: "Second"},
				{Type: ContentBlockTypeText, Text: "Third"},
			},
			expected: "First Second Third",
		},
		{
			name: "混合类型块",
			content: []AnthropicContentBlock{
				{Type: ContentBlockTypeText, Text: "Text before"},
				{Type: ContentBlockTypeToolUse, ID: "toolu_123"},
				{Type: ContentBlockTypeText, Text: "Text after"},
			},
			expected: "Text before Text after",
		},
		{
			name: "空text块被忽略",
			content: []AnthropicContentBlock{
				{Type: ContentBlockTypeText, Text: "Valid"},
				{Type: ContentBlockTypeText, Text: ""},
				{Type: ContentBlockTypeText, Text: "Also valid"},
			},
			expected: "Valid Also valid",
		},
		{
			name: "只有非text块",
			content: []AnthropicContentBlock{
				{Type: ContentBlockTypeToolUse, ID: "toolu_123"},
				{Type: ContentBlockTypeToolResult, ID: "toolu_123"},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getContentText(tt.content)
			if result != tt.expected {
				t.Errorf("期望 '%s'，实际 '%s'", tt.expected, result)
			}
		})
	}
}
