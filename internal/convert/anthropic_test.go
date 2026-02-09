package convert

import (
	"testing"

	"jetbrainsai2api/internal/core"
)

func TestExtractAllToolUse(t *testing.T) {
	tests := []struct {
		name              string
		content           any
		expectedCount     int
		expectedToolNames []string
		expectedToolIDs   []string
	}{
		{name: "空内容", content: nil, expectedCount: 0},
		{name: "字符串内容（无工具调用）", content: "普通文本消息", expectedCount: 0},
		{
			name: "单个 tool_use",
			content: []any{
				map[string]any{"type": core.ContentBlockTypeToolUse, "id": "toolu_01ABC", "name": "get_weather"},
			},
			expectedCount: 1, expectedToolNames: []string{"get_weather"}, expectedToolIDs: []string{"toolu_01ABC"},
		},
		{
			name: "两个 tool_use",
			content: []any{
				map[string]any{"type": core.ContentBlockTypeToolUse, "id": "toolu_01ABC", "name": "get_weather"},
				map[string]any{"type": core.ContentBlockTypeToolUse, "id": "toolu_02DEF", "name": "get_time"},
			},
			expectedCount: 2, expectedToolNames: []string{"get_weather", "get_time"}, expectedToolIDs: []string{"toolu_01ABC", "toolu_02DEF"},
		},
		{
			name: "混合内容（text + tool_use）",
			content: []any{
				map[string]any{"type": core.ContentBlockTypeText, "text": "让我调用两个工具"},
				map[string]any{"type": core.ContentBlockTypeToolUse, "id": "toolu_01ABC", "name": "tool_a"},
				map[string]any{"type": core.ContentBlockTypeToolUse, "id": "toolu_02DEF", "name": "tool_b"},
			},
			expectedCount: 2, expectedToolNames: []string{"tool_a", "tool_b"}, expectedToolIDs: []string{"toolu_01ABC", "toolu_02DEF"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractAllToolUse(tt.content)
			if len(result) != tt.expectedCount {
				t.Errorf("期望提取 %d 个工具调用，实际提取 %d 个", tt.expectedCount, len(result))
				return
			}
			for i := 0; i < len(result); i++ {
				if result[i].Name != tt.expectedToolNames[i] {
					t.Errorf("工具 %d 名称错误，期望 '%s'，实际 '%s'", i, tt.expectedToolNames[i], result[i].Name)
				}
				if result[i].ID != tt.expectedToolIDs[i] {
					t.Errorf("工具 %d ID错误，期望 '%s'，实际 '%s'", i, tt.expectedToolIDs[i], result[i].ID)
				}
			}
		})
	}
}

func TestAnthropicToJetbrainsMessages_MultipleToolUse(t *testing.T) {
	tests := []struct {
		name              string
		messages          []core.AnthropicMessage
		expectedCount     int
		expectedTypes     []string
		expectedToolNames []string
		expectedToolIDs   []string
	}{
		{
			name: "单个 assistant 消息包含两个 tool_use",
			messages: []core.AnthropicMessage{
				{
					Role: core.RoleAssistant,
					Content: []any{
						map[string]any{"type": core.ContentBlockTypeToolUse, "id": "toolu_01", "name": "get_weather"},
						map[string]any{"type": core.ContentBlockTypeToolUse, "id": "toolu_02", "name": "get_time"},
					},
				},
			},
			expectedCount:     2,
			expectedTypes:     []string{core.JetBrainsMessageTypeAssistantTool, core.JetBrainsMessageTypeAssistantTool},
			expectedToolNames: []string{"get_weather", "get_time"},
			expectedToolIDs:   []string{"toolu_01", "toolu_02"},
		},
		{
			name: "普通 assistant 文本消息",
			messages: []core.AnthropicMessage{
				{Role: core.RoleAssistant, Content: "这是一个普通回复"},
			},
			expectedCount:     1,
			expectedTypes:     []string{core.JetBrainsMessageTypeAssistant},
			expectedToolNames: []string{""},
			expectedToolIDs:   []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AnthropicToJetbrainsMessages(tt.messages)
			if len(result) != tt.expectedCount {
				t.Errorf("期望生成 %d 个消息，实际生成 %d 个", tt.expectedCount, len(result))
				return
			}
			for i := 0; i < len(result); i++ {
				if result[i].Type != tt.expectedTypes[i] {
					t.Errorf("消息 %d 类型错误，期望 '%s'，实际 '%s'", i, tt.expectedTypes[i], result[i].Type)
				}
				if tt.expectedToolNames[i] != "" && result[i].ToolName != tt.expectedToolNames[i] {
					t.Errorf("消息 %d 工具名称错误，期望 '%s'，实际 '%s'", i, tt.expectedToolNames[i], result[i].ToolName)
				}
				if tt.expectedToolIDs[i] != "" && result[i].ID != tt.expectedToolIDs[i] {
					t.Errorf("消息 %d 工具ID错误，期望 '%s'，实际 '%s'", i, tt.expectedToolIDs[i], result[i].ID)
				}
			}
		})
	}
}

func TestHasContentBlockType(t *testing.T) {
	tests := []struct {
		name       string
		content    any
		targetType string
		expected   bool
	}{
		{name: "nil 内容检查 tool_use", content: nil, targetType: core.ContentBlockTypeToolUse, expected: false},
		{name: "字符串内容检查 tool_use", content: "普通文本", targetType: core.ContentBlockTypeToolUse, expected: false},
		{
			name:       "包含 tool_use block 检查 tool_use",
			content:    []any{map[string]any{"type": core.ContentBlockTypeToolUse, "id": "toolu_01", "name": "test_tool"}},
			targetType: core.ContentBlockTypeToolUse,
			expected:   true,
		},
		{
			name:       "只有 text block 检查 tool_use",
			content:    []any{map[string]any{"type": core.ContentBlockTypeText, "text": "文本内容"}},
			targetType: core.ContentBlockTypeToolUse,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasContentBlockType(tt.content, tt.targetType)
			if result != tt.expected {
				t.Errorf("期望 %v，实际 %v", tt.expected, result)
			}
		})
	}
}

func TestExtractMixedContent(t *testing.T) {
	tests := []struct {
		name              string
		content           any
		toolIDToName      map[string]string
		expectedToolCount int
		expectedText      string
	}{
		{name: "空内容", content: nil, toolIDToName: map[string]string{}, expectedToolCount: 0, expectedText: ""},
		{
			name:              "只有文本",
			content:           []any{map[string]any{"type": core.ContentBlockTypeText, "text": "纯文本内容"}},
			toolIDToName:      map[string]string{},
			expectedToolCount: 0,
			expectedText:      "纯文本内容",
		},
		{
			name:              "只有tool_result（字符串content）",
			content:           []any{map[string]any{"type": core.ContentBlockTypeToolResult, "tool_use_id": "toolu_01", "content": "工具执行结果"}},
			toolIDToName:      map[string]string{"toolu_01": "get_weather"},
			expectedToolCount: 1,
			expectedText:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolMsgs, textContent := extractMixedContent(tt.content, tt.toolIDToName)
			if len(toolMsgs) != tt.expectedToolCount {
				t.Errorf("期望 %d 个工具消息，实际 %d 个", tt.expectedToolCount, len(toolMsgs))
			}
			if textContent != tt.expectedText {
				t.Errorf("期望文本 '%s'，实际 '%s'", tt.expectedText, textContent)
			}
		})
	}
}

func TestExtractToolInfo(t *testing.T) {
	tests := []struct {
		name       string
		content    any
		expectNil  bool
		expectedID string
	}{
		{name: "nil内容", content: nil, expectNil: true},
		{name: "字符串内容", content: "普通文本", expectNil: true},
		{
			name:       "包含tool_use",
			content:    []any{map[string]any{"type": core.ContentBlockTypeToolUse, "id": "toolu_abc", "name": "test_tool"}},
			expectNil:  false,
			expectedID: "toolu_abc",
		},
		{
			name:       "包含tool_result",
			content:    []any{map[string]any{"type": core.ContentBlockTypeToolResult, "tool_use_id": "toolu_def", "content": "工具结果"}},
			expectNil:  false,
			expectedID: "toolu_def",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractToolInfo(tt.content)
			if tt.expectNil {
				if result != nil {
					t.Errorf("期望 nil，实际得到 %+v", result)
				}
			} else {
				if result == nil {
					t.Error("期望非nil结果，实际得到nil")
					return
				}
				if result.ID != tt.expectedID {
					t.Errorf("期望ID '%s'，实际 '%s'", tt.expectedID, result.ID)
				}
			}
		})
	}
}

func TestExtractStringContent(t *testing.T) {
	tests := []struct {
		name     string
		content  any
		expected string
	}{
		{name: "字符串内容", content: "Hello World", expected: "Hello World"},
		{name: "空字符串", content: "", expected: ""},
		{
			name:     "单个text块",
			content:  []any{map[string]any{"type": core.ContentBlockTypeText, "text": "Text content"}},
			expected: "Text content",
		},
		{
			name:     "tool_use块返回input的JSON",
			content:  []any{map[string]any{"type": core.ContentBlockTypeToolUse, "id": "toolu_123", "name": "get_weather", "input": map[string]any{"city": "Beijing"}}},
			expected: `{"city":"Beijing"}`,
		},
		{name: "nil内容", content: nil, expected: "<nil>"},
		{name: "空数组", content: []any{}, expected: "[]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractStringContent(tt.content)
			if result != tt.expected {
				t.Errorf("期望 '%s'，实际 '%s'", tt.expected, result)
			}
		})
	}
}

func TestAnthropicToJetbrainsTools(t *testing.T) {
	tests := []struct {
		name          string
		tools         []core.AnthropicTool
		expectedCount int
		expectedNames []string
	}{
		{name: "空工具列表", tools: []core.AnthropicTool{}, expectedCount: 0},
		{name: "nil工具列表", tools: nil, expectedCount: 0},
		{
			name: "单个工具",
			tools: []core.AnthropicTool{
				{Name: "get_weather", Description: "Get weather info", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}}},
			},
			expectedCount: 1, expectedNames: []string{"get_weather"},
		},
		{
			name: "多个工具",
			tools: []core.AnthropicTool{
				{Name: "tool_a", Description: "Tool A description", InputSchema: map[string]any{"type": "object"}},
				{Name: "tool_b", Description: "Tool B description", InputSchema: map[string]any{"type": "object"}},
				{Name: "tool_c", Description: "Tool C description", InputSchema: map[string]any{"type": "object"}},
			},
			expectedCount: 3, expectedNames: []string{"tool_a", "tool_b", "tool_c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AnthropicToJetbrainsTools(tt.tools)
			if len(result) != tt.expectedCount {
				t.Errorf("期望 %d 个工具，实际 %d 个", tt.expectedCount, len(result))
				return
			}
			for i, expectedName := range tt.expectedNames {
				if result[i].Name != expectedName {
					t.Errorf("工具 %d 名称错误，期望 '%s'，实际 '%s'", i, expectedName, result[i].Name)
				}
			}
		})
	}
}
