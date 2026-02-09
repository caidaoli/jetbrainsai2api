package server

import (
	"strings"
	"testing"

	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/util"
)

func TestGenerateRandomID(t *testing.T) {
	id := util.GenerateRandomID(core.ToolCallIDPrefix)
	if !strings.HasPrefix(id, core.ToolCallIDPrefix) {
		t.Errorf("工具调用ID应以 '%s' 为前缀，实际: '%s'", core.ToolCallIDPrefix, id)
	}
	expectedLen := len(core.ToolCallIDPrefix) + 20
	if len(id) != expectedLen {
		t.Errorf("工具调用ID长度应为 %d，实际: %d", expectedLen, len(id))
	}
	hexPart := id[len(core.ToolCallIDPrefix):]
	for _, c := range hexPart {
		isDigit := c >= '0' && c <= '9'
		isHexLower := c >= 'a' && c <= 'f'
		if !isDigit && !isHexLower {
			t.Errorf("hex部分包含无效字符: %c", c)
		}
	}
}

func TestMapJetbrainsToOpenAIFinishReason(t *testing.T) {
	tests := []struct {
		name, input, expected string
	}{
		{"tool_call映射到tool_calls", core.JetBrainsFinishReasonToolCall, core.FinishReasonToolCalls},
		{"length映射到length", core.JetBrainsFinishReasonLength, core.FinishReasonLength},
		{"stop映射到stop", core.JetBrainsFinishReasonStop, core.FinishReasonStop},
		{"未知值默认映射到stop", "unknown_reason", core.FinishReasonStop},
		{"空字符串默认映射到stop", "", core.FinishReasonStop},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapJetbrainsToOpenAIFinishReason(tt.input)
			if result != tt.expected { t.Errorf("期望 '%s'，实际 '%s'", tt.expected, result) }
		})
	}
}

func TestStringPtr(t *testing.T) {
	tests := []struct {
		name, input string
	}{
		{"普通字符串", "hello"},
		{"空字符串", ""},
		{"中文字符串", "你好世界"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stringPtr(tt.input)
			if result == nil { t.Error("返回值不应为nil"); return }
			if *result != tt.input { t.Errorf("期望 '%s'，实际 '%s'", tt.input, *result) }
		})
	}
}
