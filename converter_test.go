package main

import (
	"testing"
)

func TestOpenAIToJetbrainsMessages_MultipleTextContent(t *testing.T) {
	messages := []ChatMessage{
		{
			Role: RoleUser,
			Content: []any{
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "第一条消息内容",
				},
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "第二条消息内容",
				},
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "第三条消息内容",
				},
			},
		},
	}

	result := openAIToJetbrainsMessages(messages)

	// 修复后应该生成3个独立的user_message
	expectedCount := 3
	if len(result) != expectedCount {
		t.Errorf("期望生成 %d 个消息，实际生成 %d 个", expectedCount, len(result))
	}

	// 验证每个消息都是user_message类型
	for i, msg := range result {
		if msg.Type != JetBrainsMessageTypeUser {
			t.Errorf("消息 %d 类型错误，期望 '%s'，实际 '%s'", i, JetBrainsMessageTypeUser, msg.Type)
		}
	}

	// 验证消息内容是否正确分离
	expectedContents := []string{"第一条消息内容", "第二条消息内容", "第三条消息内容"}
	for i, expectedContent := range expectedContents {
		if result[i].Content != expectedContent {
			t.Errorf("消息 %d 内容错误，期望 '%s'，实际 '%s'", i, expectedContent, result[i].Content)
		}
	}
}

func TestOpenAIToJetbrainsMessages_SingleTextContent(t *testing.T) {
	messages := []ChatMessage{
		{
			Role:    RoleUser,
			Content: "单一文本消息",
		},
	}

	result := openAIToJetbrainsMessages(messages)

	if len(result) != 1 {
		t.Errorf("期望生成 1 个消息，实际生成 %d 个", len(result))
	}

	if result[0].Type != JetBrainsMessageTypeUser {
		t.Errorf("消息类型错误，期望 '%s'，实际 '%s'", JetBrainsMessageTypeUser, result[0].Type)
	}

	if result[0].Content != "单一文本消息" {
		t.Errorf("消息内容错误，期望 '单一文本消息'，实际 '%s'", result[0].Content)
	}
}

// TestConvertAssistantMessage_MultipleToolCalls 测试多工具调用转换
func TestConvertAssistantMessage_MultipleToolCalls(t *testing.T) {
	tests := []struct {
		name              string
		toolCalls         []ToolCall
		expectedCount     int
		expectedToolNames []string
		expectedToolIDs   []string
	}{
		{
			name: "单个工具调用",
			toolCalls: []ToolCall{
				{ID: "call_1", Type: ToolTypeFunction, Function: Function{Name: "get_weather", Arguments: `{"city":"Beijing"}`}},
			},
			expectedCount:     1,
			expectedToolNames: []string{"get_weather"},
			expectedToolIDs:   []string{"call_1"},
		},
		{
			name: "两个工具调用",
			toolCalls: []ToolCall{
				{ID: "call_1", Type: ToolTypeFunction, Function: Function{Name: "get_weather", Arguments: `{"city":"Beijing"}`}},
				{ID: "call_2", Type: ToolTypeFunction, Function: Function{Name: "get_time", Arguments: `{"timezone":"UTC"}`}},
			},
			expectedCount:     2,
			expectedToolNames: []string{"get_weather", "get_time"},
			expectedToolIDs:   []string{"call_1", "call_2"},
		},
		{
			name: "三个工具调用",
			toolCalls: []ToolCall{
				{ID: "call_1", Type: ToolTypeFunction, Function: Function{Name: "read_file", Arguments: `{"path":"/tmp/a.txt"}`}},
				{ID: "call_2", Type: ToolTypeFunction, Function: Function{Name: "write_file", Arguments: `{"path":"/tmp/b.txt"}`}},
				{ID: "call_3", Type: ToolTypeFunction, Function: Function{Name: "delete_file", Arguments: `{"path":"/tmp/c.txt"}`}},
			},
			expectedCount:     3,
			expectedToolNames: []string{"read_file", "write_file", "delete_file"},
			expectedToolIDs:   []string{"call_1", "call_2", "call_3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := &MessageConverter{
				toolIDToFuncNameMap: make(map[string]string),
			}
			msg := ChatMessage{
				Role:      RoleAssistant,
				ToolCalls: tt.toolCalls,
			}

			result := converter.convertAssistantMessage(msg)

			// 验证生成的消息数量
			if len(result) != tt.expectedCount {
				t.Errorf("期望生成 %d 个消息，实际生成 %d 个", tt.expectedCount, len(result))
				return
			}

			// 验证每个消息的类型、工具名称和ID
			for i, expectedName := range tt.expectedToolNames {
				if result[i].Type != JetBrainsMessageTypeAssistantTool {
					t.Errorf("消息 %d 类型错误，期望 '%s'，实际 '%s'",
						i, JetBrainsMessageTypeAssistantTool, result[i].Type)
				}
				if result[i].ToolName != expectedName {
					t.Errorf("消息 %d 工具名称错误，期望 '%s'，实际 '%s'",
						i, expectedName, result[i].ToolName)
				}
				if result[i].ID != tt.expectedToolIDs[i] {
					t.Errorf("消息 %d 工具ID错误，期望 '%s'，实际 '%s'",
						i, tt.expectedToolIDs[i], result[i].ID)
				}
			}
		})
	}
}

// TestConvertAssistantMessage_NoToolCalls 测试无工具调用时返回文本消息
func TestConvertAssistantMessage_NoToolCalls(t *testing.T) {
	converter := &MessageConverter{
		toolIDToFuncNameMap: make(map[string]string),
	}
	msg := ChatMessage{
		Role:    RoleAssistant,
		Content: "这是一个普通的文本回复",
	}

	result := converter.convertAssistantMessage(msg)

	if len(result) != 1 {
		t.Errorf("期望生成 1 个消息，实际生成 %d 个", len(result))
		return
	}

	if result[0].Type != JetBrainsMessageTypeAssistantText {
		t.Errorf("消息类型错误，期望 '%s'，实际 '%s'",
			JetBrainsMessageTypeAssistantText, result[0].Type)
	}

	if result[0].Content != "这是一个普通的文本回复" {
		t.Errorf("消息内容错误，期望 '这是一个普通的文本回复'，实际 '%s'", result[0].Content)
	}
}
