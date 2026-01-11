package main

import (
	"testing"
)

// TestExtractAllToolUse 测试从消息内容中提取所有 tool_use blocks
func TestExtractAllToolUse(t *testing.T) {
	tests := []struct {
		name              string
		content           any
		expectedCount     int
		expectedToolNames []string
		expectedToolIDs   []string
	}{
		{
			name:          "空内容",
			content:       nil,
			expectedCount: 0,
		},
		{
			name:          "字符串内容（无工具调用）",
			content:       "普通文本消息",
			expectedCount: 0,
		},
		{
			name: "单个 tool_use",
			content: []any{
				map[string]any{
					"type": ContentBlockTypeToolUse,
					"id":   "toolu_01ABC",
					"name": "get_weather",
				},
			},
			expectedCount:     1,
			expectedToolNames: []string{"get_weather"},
			expectedToolIDs:   []string{"toolu_01ABC"},
		},
		{
			name: "两个 tool_use",
			content: []any{
				map[string]any{
					"type": ContentBlockTypeToolUse,
					"id":   "toolu_01ABC",
					"name": "get_weather",
				},
				map[string]any{
					"type": ContentBlockTypeToolUse,
					"id":   "toolu_02DEF",
					"name": "get_time",
				},
			},
			expectedCount:     2,
			expectedToolNames: []string{"get_weather", "get_time"},
			expectedToolIDs:   []string{"toolu_01ABC", "toolu_02DEF"},
		},
		{
			name: "三个 tool_use",
			content: []any{
				map[string]any{
					"type": ContentBlockTypeToolUse,
					"id":   "toolu_01",
					"name": "read_file",
				},
				map[string]any{
					"type": ContentBlockTypeToolUse,
					"id":   "toolu_02",
					"name": "write_file",
				},
				map[string]any{
					"type": ContentBlockTypeToolUse,
					"id":   "toolu_03",
					"name": "execute_command",
				},
			},
			expectedCount:     3,
			expectedToolNames: []string{"read_file", "write_file", "execute_command"},
			expectedToolIDs:   []string{"toolu_01", "toolu_02", "toolu_03"},
		},
		{
			name: "混合内容（text + tool_use）",
			content: []any{
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "让我调用两个工具",
				},
				map[string]any{
					"type": ContentBlockTypeToolUse,
					"id":   "toolu_01ABC",
					"name": "tool_a",
				},
				map[string]any{
					"type": ContentBlockTypeToolUse,
					"id":   "toolu_02DEF",
					"name": "tool_b",
				},
			},
			expectedCount:     2,
			expectedToolNames: []string{"tool_a", "tool_b"},
			expectedToolIDs:   []string{"toolu_01ABC", "toolu_02DEF"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractAllToolUse(tt.content)

			if len(result) != tt.expectedCount {
				t.Errorf("期望提取 %d 个工具调用，实际提取 %d 个", tt.expectedCount, len(result))
				return
			}

			for i := 0; i < len(result); i++ {
				if result[i].Name != tt.expectedToolNames[i] {
					t.Errorf("工具 %d 名称错误，期望 '%s'，实际 '%s'",
						i, tt.expectedToolNames[i], result[i].Name)
				}
				if result[i].ID != tt.expectedToolIDs[i] {
					t.Errorf("工具 %d ID错误，期望 '%s'，实际 '%s'",
						i, tt.expectedToolIDs[i], result[i].ID)
				}
			}
		})
	}
}

// TestAnthropicToJetbrainsMessages_MultipleToolUse 测试 Anthropic 消息转换多工具调用
func TestAnthropicToJetbrainsMessages_MultipleToolUse(t *testing.T) {
	tests := []struct {
		name              string
		messages          []AnthropicMessage
		expectedCount     int
		expectedTypes     []string
		expectedToolNames []string
		expectedToolIDs   []string
	}{
		{
			name: "单个 assistant 消息包含两个 tool_use",
			messages: []AnthropicMessage{
				{
					Role: RoleAssistant,
					Content: []any{
						map[string]any{
							"type": ContentBlockTypeToolUse,
							"id":   "toolu_01",
							"name": "get_weather",
						},
						map[string]any{
							"type": ContentBlockTypeToolUse,
							"id":   "toolu_02",
							"name": "get_time",
						},
					},
				},
			},
			expectedCount:     2,
			expectedTypes:     []string{JetBrainsMessageTypeAssistantTool, JetBrainsMessageTypeAssistantTool},
			expectedToolNames: []string{"get_weather", "get_time"},
			expectedToolIDs:   []string{"toolu_01", "toolu_02"},
		},
		{
			name: "单个 assistant 消息包含三个 tool_use",
			messages: []AnthropicMessage{
				{
					Role: RoleAssistant,
					Content: []any{
						map[string]any{
							"type": ContentBlockTypeToolUse,
							"id":   "toolu_a",
							"name": "read_file",
						},
						map[string]any{
							"type": ContentBlockTypeToolUse,
							"id":   "toolu_b",
							"name": "write_file",
						},
						map[string]any{
							"type": ContentBlockTypeToolUse,
							"id":   "toolu_c",
							"name": "delete_file",
						},
					},
				},
			},
			expectedCount:     3,
			expectedTypes:     []string{JetBrainsMessageTypeAssistantTool, JetBrainsMessageTypeAssistantTool, JetBrainsMessageTypeAssistantTool},
			expectedToolNames: []string{"read_file", "write_file", "delete_file"},
			expectedToolIDs:   []string{"toolu_a", "toolu_b", "toolu_c"},
		},
		{
			name: "普通 assistant 文本消息",
			messages: []AnthropicMessage{
				{
					Role:    RoleAssistant,
					Content: "这是一个普通回复",
				},
			},
			expectedCount:     1,
			expectedTypes:     []string{JetBrainsMessageTypeAssistant},
			expectedToolNames: []string{""}, // 无工具调用
			expectedToolIDs:   []string{""}, // 无工具ID
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := anthropicToJetbrainsMessages(tt.messages)

			if len(result) != tt.expectedCount {
				t.Errorf("期望生成 %d 个消息，实际生成 %d 个", tt.expectedCount, len(result))
				return
			}

			for i := 0; i < len(result); i++ {
				if result[i].Type != tt.expectedTypes[i] {
					t.Errorf("消息 %d 类型错误，期望 '%s'，实际 '%s'",
						i, tt.expectedTypes[i], result[i].Type)
				}
				if tt.expectedToolNames[i] != "" && result[i].ToolName != tt.expectedToolNames[i] {
					t.Errorf("消息 %d 工具名称错误，期望 '%s'，实际 '%s'",
						i, tt.expectedToolNames[i], result[i].ToolName)
				}
				if tt.expectedToolIDs[i] != "" && result[i].ID != tt.expectedToolIDs[i] {
					t.Errorf("消息 %d 工具ID错误，期望 '%s'，实际 '%s'",
						i, tt.expectedToolIDs[i], result[i].ID)
				}
			}
		})
	}
}

// TestHasContentBlockType 测试 hasContentBlockType 函数
func TestHasContentBlockType(t *testing.T) {
	tests := []struct {
		name       string
		content    any
		targetType string
		expected   bool
	}{
		{
			name:       "nil 内容检查 tool_use",
			content:    nil,
			targetType: ContentBlockTypeToolUse,
			expected:   false,
		},
		{
			name:       "字符串内容检查 tool_use",
			content:    "普通文本",
			targetType: ContentBlockTypeToolUse,
			expected:   false,
		},
		{
			name: "只有 text block 检查 tool_use",
			content: []any{
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "文本内容",
				},
			},
			targetType: ContentBlockTypeToolUse,
			expected:   false,
		},
		{
			name: "包含 tool_use block 检查 tool_use",
			content: []any{
				map[string]any{
					"type": ContentBlockTypeToolUse,
					"id":   "toolu_01",
					"name": "test_tool",
				},
			},
			targetType: ContentBlockTypeToolUse,
			expected:   true,
		},
		{
			name: "混合 text 和 tool_use 检查 tool_use",
			content: []any{
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "先说点什么",
				},
				map[string]any{
					"type": ContentBlockTypeToolUse,
					"id":   "toolu_01",
					"name": "test_tool",
				},
			},
			targetType: ContentBlockTypeToolUse,
			expected:   true,
		},
		{
			name:       "nil内容检查 tool_result",
			content:    nil,
			targetType: ContentBlockTypeToolResult,
			expected:   false,
		},
		{
			name:       "字符串内容检查 tool_result",
			content:    "普通文本",
			targetType: ContentBlockTypeToolResult,
			expected:   false,
		},
		{
			name: "只有text block检查 tool_result",
			content: []any{
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "文本内容",
				},
			},
			targetType: ContentBlockTypeToolResult,
			expected:   false,
		},
		{
			name: "包含tool_result block检查 tool_result",
			content: []any{
				map[string]any{
					"type":        ContentBlockTypeToolResult,
					"tool_use_id": "toolu_01",
					"content":     "工具结果",
				},
			},
			targetType: ContentBlockTypeToolResult,
			expected:   true,
		},
		{
			name: "混合text和tool_result检查 tool_result",
			content: []any{
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "一些文本",
				},
				map[string]any{
					"type":        ContentBlockTypeToolResult,
					"tool_use_id": "toolu_01",
					"content":     "工具结果",
				},
			},
			targetType: ContentBlockTypeToolResult,
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasContentBlockType(tt.content, tt.targetType)
			if result != tt.expected {
				t.Errorf("期望 %v，实际 %v", tt.expected, result)
			}
		})
	}
}

// TestExtractMixedContent 测试 extractMixedContent 函数
func TestExtractMixedContent(t *testing.T) {
	tests := []struct {
		name              string
		content           any
		toolIDToName      map[string]string
		expectedToolCount int
		expectedText      string
	}{
		{
			name:              "空内容",
			content:           nil,
			toolIDToName:      map[string]string{},
			expectedToolCount: 0,
			expectedText:      "",
		},
		{
			name: "只有文本",
			content: []any{
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "纯文本内容",
				},
			},
			toolIDToName:      map[string]string{},
			expectedToolCount: 0,
			expectedText:      "纯文本内容",
		},
		{
			name: "只有tool_result（字符串content）",
			content: []any{
				map[string]any{
					"type":        ContentBlockTypeToolResult,
					"tool_use_id": "toolu_01",
					"content":     "工具执行结果",
				},
			},
			toolIDToName:      map[string]string{"toolu_01": "get_weather"},
			expectedToolCount: 1,
			expectedText:      "",
		},
		{
			name: "tool_result带数组content",
			content: []any{
				map[string]any{
					"type":        ContentBlockTypeToolResult,
					"tool_use_id": "toolu_02",
					"content": []any{
						map[string]any{"type": "text", "text": "第一部分"},
						map[string]any{"type": "text", "text": "第二部分"},
					},
				},
			},
			toolIDToName:      map[string]string{"toolu_02": "search"},
			expectedToolCount: 1,
			expectedText:      "",
		},
		{
			name: "混合text和tool_result",
			content: []any{
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "文本1",
				},
				map[string]any{
					"type":        ContentBlockTypeToolResult,
					"tool_use_id": "toolu_01",
					"content":     "结果",
				},
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "文本2",
				},
			},
			toolIDToName:      map[string]string{"toolu_01": "tool_a"},
			expectedToolCount: 1,
			expectedText:      "文本1 文本2",
		},
		{
			name: "未知工具ID使用Unknown",
			content: []any{
				map[string]any{
					"type":        ContentBlockTypeToolResult,
					"tool_use_id": "unknown_id",
					"content":     "结果",
				},
			},
			toolIDToName:      map[string]string{},
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

// TestExtractToolInfo 测试 extractToolInfo 函数
func TestExtractToolInfo(t *testing.T) {
	tests := []struct {
		name       string
		content    any
		expectNil  bool
		expectedID string
	}{
		{
			name:      "nil内容",
			content:   nil,
			expectNil: true,
		},
		{
			name:      "字符串内容",
			content:   "普通文本",
			expectNil: true,
		},
		{
			name: "只有text block",
			content: []any{
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "文本",
				},
			},
			expectNil: true,
		},
		{
			name: "包含tool_use",
			content: []any{
				map[string]any{
					"type": ContentBlockTypeToolUse,
					"id":   "toolu_abc",
					"name": "test_tool",
				},
			},
			expectNil:  false,
			expectedID: "toolu_abc",
		},
		{
			name: "包含tool_result（字符串content）",
			content: []any{
				map[string]any{
					"type":        ContentBlockTypeToolResult,
					"tool_use_id": "toolu_def",
					"content":     "工具结果",
				},
			},
			expectNil:  false,
			expectedID: "toolu_def",
		},
		{
			name: "包含tool_result（数组content）",
			content: []any{
				map[string]any{
					"type":        ContentBlockTypeToolResult,
					"tool_use_id": "toolu_ghi",
					"content": []any{
						map[string]any{"type": "text", "text": "结果文本"},
					},
				},
			},
			expectNil:  false,
			expectedID: "toolu_ghi",
		},
		{
			name: "tool_use优先于tool_result",
			content: []any{
				map[string]any{
					"type": ContentBlockTypeToolUse,
					"id":   "toolu_first",
					"name": "first_tool",
				},
				map[string]any{
					"type":        ContentBlockTypeToolResult,
					"tool_use_id": "toolu_second",
					"content":     "结果",
				},
			},
			expectNil:  false,
			expectedID: "toolu_first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractToolInfo(tt.content)

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

// TestExtractStringContent 测试字符串内容提取
func TestExtractStringContent(t *testing.T) {
	tests := []struct {
		name     string
		content  any
		expected string
	}{
		{
			name:     "字符串内容",
			content:  "Hello World",
			expected: "Hello World",
		},
		{
			name:     "空字符串",
			content:  "",
			expected: "",
		},
		{
			name: "单个text块",
			content: []any{
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "Text content",
				},
			},
			expected: "Text content",
		},
		{
			name: "多个text块取第一个",
			content: []any{
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "First text",
				},
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "Second text",
				},
			},
			expected: "First text",
		},
		{
			name: "tool_use块返回input的JSON",
			content: []any{
				map[string]any{
					"type":  ContentBlockTypeToolUse,
					"id":    "toolu_123",
					"name":  "get_weather",
					"input": map[string]any{"city": "Beijing"},
				},
			},
			expected: `{"city":"Beijing"}`,
		},
		{
			name: "空text块被忽略",
			content: []any{
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "",
				},
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "Valid text",
				},
			},
			expected: "Valid text",
		},
		{
			name:     "数字类型",
			content:  123,
			expected: "123",
		},
		{
			name:     "nil内容",
			content:  nil,
			expected: "<nil>",
		},
		{
			name:     "空数组",
			content:  []any{},
			expected: "[]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractStringContent(tt.content)
			if result != tt.expected {
				t.Errorf("期望 '%s'，实际 '%s'", tt.expected, result)
			}
		})
	}
}

// TestAnthropicToJetbrainsTools 测试Anthropic工具转换为JetBrains格式
func TestAnthropicToJetbrainsTools(t *testing.T) {
	tests := []struct {
		name          string
		tools         []AnthropicTool
		expectedCount int
		expectedNames []string
	}{
		{
			name:          "空工具列表",
			tools:         []AnthropicTool{},
			expectedCount: 0,
		},
		{
			name:          "nil工具列表",
			tools:         nil,
			expectedCount: 0,
		},
		{
			name: "单个工具",
			tools: []AnthropicTool{
				{
					Name:        "get_weather",
					Description: "Get weather info",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"city": map[string]any{"type": "string"},
						},
					},
				},
			},
			expectedCount: 1,
			expectedNames: []string{"get_weather"},
		},
		{
			name: "多个工具",
			tools: []AnthropicTool{
				{
					Name:        "tool_a",
					Description: "Tool A description",
					InputSchema: map[string]any{"type": "object"},
				},
				{
					Name:        "tool_b",
					Description: "Tool B description",
					InputSchema: map[string]any{"type": "object"},
				},
				{
					Name:        "tool_c",
					Description: "Tool C description",
					InputSchema: map[string]any{"type": "object"},
				},
			},
			expectedCount: 3,
			expectedNames: []string{"tool_a", "tool_b", "tool_c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := anthropicToJetbrainsTools(tt.tools)

			if len(result) != tt.expectedCount {
				t.Errorf("期望 %d 个工具，实际 %d 个", tt.expectedCount, len(result))
				return
			}

			for i, expectedName := range tt.expectedNames {
				if result[i].Name != expectedName {
					t.Errorf("工具 %d 名称错误，期望 '%s'，实际 '%s'",
						i, expectedName, result[i].Name)
				}
			}
		})
	}
}
