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
				logger:              &NopLogger{},
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
		logger:              &NopLogger{},
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

// TestConvertToolMessage 测试工具响应消息转换
func TestConvertToolMessage(t *testing.T) {
	tests := []struct {
		name         string
		toolCallID   string
		funcName     string
		content      any
		expectResult bool
	}{
		{
			name:         "正常工具响应",
			toolCallID:   "call_123",
			funcName:     "get_weather",
			content:      `{"temperature": 25, "weather": "sunny"}`,
			expectResult: true,
		},
		{
			name:         "缺少函数名映射",
			toolCallID:   "unknown_call",
			funcName:     "", // 不在映射中
			content:      "result",
			expectResult: false,
		},
		{
			name:         "复杂内容类型",
			toolCallID:   "call_456",
			funcName:     "search",
			content:      []any{map[string]any{"type": "text", "text": "搜索结果"}},
			expectResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := &MessageConverter{
				toolIDToFuncNameMap: make(map[string]string),
				logger:              &NopLogger{},
			}

			// 如果有函数名，添加到映射
			if tt.funcName != "" {
				converter.toolIDToFuncNameMap[tt.toolCallID] = tt.funcName
			}

			msg := ChatMessage{
				Role:       RoleTool,
				ToolCallID: tt.toolCallID,
				Content:    tt.content,
			}

			result := converter.convertToolMessage(msg)

			if tt.expectResult {
				if len(result) != 1 {
					t.Errorf("期望生成 1 个消息，实际生成 %d 个", len(result))
					return
				}
				if result[0].Type != JetBrainsMessageTypeTool {
					t.Errorf("消息类型错误，期望 '%s'，实际 '%s'",
						JetBrainsMessageTypeTool, result[0].Type)
				}
				if result[0].ID != tt.toolCallID {
					t.Errorf("工具调用ID错误，期望 '%s'，实际 '%s'",
						tt.toolCallID, result[0].ID)
				}
				if result[0].ToolName != tt.funcName {
					t.Errorf("工具名称错误，期望 '%s'，实际 '%s'",
						tt.funcName, result[0].ToolName)
				}
			} else {
				if result != nil {
					t.Errorf("期望返回 nil，实际返回 %v", result)
				}
			}
		})
	}
}

// TestConvertDefaultMessage 测试默认消息转换
func TestConvertDefaultMessage(t *testing.T) {
	tests := []struct {
		name        string
		content     any
		wantContent string
	}{
		{
			name:        "字符串内容",
			content:     "普通文本内容",
			wantContent: "普通文本内容",
		},
		{
			name: "复杂内容数组",
			content: []any{
				map[string]any{"type": "text", "text": "第一部分"},
				map[string]any{"type": "text", "text": "第二部分"},
			},
			wantContent: "第一部分 第二部分",
		},
		{
			name:        "空内容",
			content:     "",
			wantContent: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := &MessageConverter{
				toolIDToFuncNameMap: make(map[string]string),
				logger:              &NopLogger{},
			}

			msg := ChatMessage{
				Role:    "unknown", // 非标准角色
				Content: tt.content,
			}

			result := converter.convertDefaultMessage(msg)

			if len(result) != 1 {
				t.Errorf("期望生成 1 个消息，实际生成 %d 个", len(result))
				return
			}

			if result[0].Type != JetBrainsMessageTypeUser {
				t.Errorf("消息类型错误，期望 '%s'，实际 '%s'",
					JetBrainsMessageTypeUser, result[0].Type)
			}

			if result[0].Content != tt.wantContent {
				t.Errorf("消息内容错误，期望 '%s'，实际 '%s'",
					tt.wantContent, result[0].Content)
			}
		})
	}
}

// TestConvertSystemMessage 测试系统消息转换
func TestConvertSystemMessage(t *testing.T) {
	converter := &MessageConverter{
		toolIDToFuncNameMap: make(map[string]string),
		logger:              &NopLogger{},
	}

	msg := ChatMessage{
		Role:    RoleSystem,
		Content: "你是一个有帮助的助手",
	}

	result := converter.convertSystemMessage(msg)

	if len(result) != 1 {
		t.Errorf("期望生成 1 个消息，实际生成 %d 个", len(result))
		return
	}

	if result[0].Type != JetBrainsMessageTypeSystem {
		t.Errorf("消息类型错误，期望 '%s'，实际 '%s'",
			JetBrainsMessageTypeSystem, result[0].Type)
	}

	if result[0].Content != "你是一个有帮助的助手" {
		t.Errorf("消息内容错误")
	}
}

// TestBuildToolIDMap 测试工具ID映射构建
func TestBuildToolIDMap(t *testing.T) {
	converter := &MessageConverter{
		toolIDToFuncNameMap: make(map[string]string),
		logger:              &NopLogger{},
	}

	messages := []ChatMessage{
		{
			Role: RoleAssistant,
			ToolCalls: []ToolCall{
				{ID: "call_1", Function: Function{Name: "func_a"}},
				{ID: "call_2", Function: Function{Name: "func_b"}},
			},
		},
		{
			Role:    RoleUser,
			Content: "普通消息，不影响映射",
		},
		{
			Role: RoleAssistant,
			ToolCalls: []ToolCall{
				{ID: "call_3", Function: Function{Name: "func_c"}},
			},
		},
	}

	converter.buildToolIDMap(messages)

	// 验证映射
	expected := map[string]string{
		"call_1": "func_a",
		"call_2": "func_b",
		"call_3": "func_c",
	}

	for id, name := range expected {
		if converter.toolIDToFuncNameMap[id] != name {
			t.Errorf("工具ID %s 映射错误，期望 '%s'，实际 '%s'",
				id, name, converter.toolIDToFuncNameMap[id])
		}
	}
}

// TestMessageConverterConvert 测试完整消息转换流程
func TestMessageConverterConvert(t *testing.T) {
	messages := []ChatMessage{
		{Role: RoleSystem, Content: "系统提示"},
		{Role: RoleUser, Content: "用户消息"},
		{Role: RoleAssistant, Content: "助手回复"},
	}

	converter := &MessageConverter{
		toolIDToFuncNameMap: make(map[string]string),
		validator:           NewImageValidator(),
		logger:              &NopLogger{},
	}

	result := converter.Convert(messages)

	if len(result) != 3 {
		t.Errorf("期望生成 3 个消息，实际生成 %d 个", len(result))
	}

	// 验证消息类型顺序
	expectedTypes := []string{
		JetBrainsMessageTypeSystem,
		JetBrainsMessageTypeUser,
		JetBrainsMessageTypeAssistantText,
	}

	for i, expectedType := range expectedTypes {
		if result[i].Type != expectedType {
			t.Errorf("消息 %d 类型错误，期望 '%s'，实际 '%s'",
				i, expectedType, result[i].Type)
		}
	}
}

// TestConvertMessage 测试 convertMessage 方法的各分支
func TestConvertMessage(t *testing.T) {
	tests := []struct {
		name         string
		msg          ChatMessage
		toolIDMap    map[string]string
		expectedType string
	}{
		{
			name:         "用户消息",
			msg:          ChatMessage{Role: RoleUser, Content: "用户输入"},
			expectedType: JetBrainsMessageTypeUser,
		},
		{
			name:         "系统消息",
			msg:          ChatMessage{Role: RoleSystem, Content: "系统提示"},
			expectedType: JetBrainsMessageTypeSystem,
		},
		{
			name:         "助手消息",
			msg:          ChatMessage{Role: RoleAssistant, Content: "助手回复"},
			expectedType: JetBrainsMessageTypeAssistantText,
		},
		{
			name: "工具消息",
			msg: ChatMessage{
				Role:       RoleTool,
				ToolCallID: "call_123",
				Content:    "工具结果",
			},
			toolIDMap:    map[string]string{"call_123": "test_tool"},
			expectedType: JetBrainsMessageTypeTool,
		},
		{
			name:         "未知角色消息",
			msg:          ChatMessage{Role: "custom_role", Content: "自定义消息"},
			expectedType: JetBrainsMessageTypeUser,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolMap := tt.toolIDMap
			if toolMap == nil {
				toolMap = make(map[string]string)
			}
			converter := &MessageConverter{
				toolIDToFuncNameMap: toolMap,
				validator:           NewImageValidator(),
				logger:              &NopLogger{},
			}

			result := converter.convertMessage(tt.msg)

			if len(result) == 0 {
				if tt.expectedType != "" {
					t.Errorf("期望生成消息，实际为空")
				}
				return
			}

			if result[0].Type != tt.expectedType {
				t.Errorf("消息类型错误，期望 '%s'，实际 '%s'",
					tt.expectedType, result[0].Type)
			}
		})
	}
}

// TestConvertImageContent 测试图像内容转换
func TestConvertImageContent(t *testing.T) {
	tests := []struct {
		name           string
		mediaType      string
		imageData      string
		content        any
		expectedCount  int
		expectedTypes  []string
		hasMediaMsg    bool
		expectTextOnly bool
	}{
		{
			name:      "有效图像无文本",
			mediaType: "image/png",
			imageData: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
			content: []any{
				map[string]any{
					"type": "image_url",
					"image_url": map[string]any{
						"url": "data:image/png;base64,xxx",
					},
				},
			},
			expectedCount: 1,
			expectedTypes: []string{JetBrainsMessageTypeMedia},
			hasMediaMsg:   true,
		},
		{
			name:      "有效图像带文本",
			mediaType: "image/jpeg",
			imageData: "/9j/4AAQSkZJRg==",
			content: []any{
				map[string]any{
					"type": "text",
					"text": "描述这张图片",
				},
				map[string]any{
					"type": "image_url",
					"image_url": map[string]any{
						"url": "data:image/jpeg;base64,xxx",
					},
				},
			},
			expectedCount: 2,
			expectedTypes: []string{JetBrainsMessageTypeMedia, JetBrainsMessageTypeUser},
			hasMediaMsg:   true,
		},
		{
			name:      "无效图像格式回退文本",
			mediaType: "image/bmp", // 不支持的格式
			imageData: "invalid_data",
			content: []any{
				map[string]any{
					"type": "text",
					"text": "回退文本内容",
				},
			},
			expectedCount:  1,
			expectedTypes:  []string{JetBrainsMessageTypeUser},
			hasMediaMsg:    false,
			expectTextOnly: true,
		},
		{
			name:      "无效base64数据回退文本",
			mediaType: "image/png",
			imageData: "not-valid-base64!!!",
			content: []any{
				map[string]any{
					"type": "text",
					"text": "错误时的文本",
				},
			},
			expectedCount:  1,
			expectedTypes:  []string{JetBrainsMessageTypeUser},
			hasMediaMsg:    false,
			expectTextOnly: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := &MessageConverter{
				toolIDToFuncNameMap: make(map[string]string),
				validator:           NewImageValidator(),
				logger:              &NopLogger{},
			}

			result := converter.convertImageContent(tt.mediaType, tt.imageData, tt.content)

			if len(result) != tt.expectedCount {
				t.Errorf("期望生成 %d 个消息，实际生成 %d 个", tt.expectedCount, len(result))
				return
			}

			for i, expectedType := range tt.expectedTypes {
				if i < len(result) && result[i].Type != expectedType {
					t.Errorf("消息 %d 类型错误，期望 '%s'，实际 '%s'",
						i, expectedType, result[i].Type)
				}
			}

			// 验证是否有媒体消息
			hasMedia := false
			for _, msg := range result {
				if msg.Type == JetBrainsMessageTypeMedia {
					hasMedia = true
					break
				}
			}
			if hasMedia != tt.hasMediaMsg {
				t.Errorf("媒体消息存在状态错误，期望 %v，实际 %v", tt.hasMediaMsg, hasMedia)
			}
		})
	}
}

// TestConvertUserMessage 测试用户消息转换
func TestConvertUserMessage(t *testing.T) {
	tests := []struct {
		name          string
		content       any
		expectedCount int
		expectedTypes []string
	}{
		{
			name:          "纯文本字符串",
			content:       "简单的用户消息",
			expectedCount: 1,
			expectedTypes: []string{JetBrainsMessageTypeUser},
		},
		{
			name: "文本数组内容",
			content: []any{
				map[string]any{"type": "text", "text": "第一段"},
				map[string]any{"type": "text", "text": "第二段"},
			},
			expectedCount: 2,
			expectedTypes: []string{JetBrainsMessageTypeUser, JetBrainsMessageTypeUser},
		},
		{
			name: "包含图像的内容",
			content: []any{
				map[string]any{"type": "text", "text": "描述图片"},
				map[string]any{
					"type": "image_url",
					"image_url": map[string]any{
						"url": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
					},
				},
			},
			expectedCount: 2,
			expectedTypes: []string{JetBrainsMessageTypeMedia, JetBrainsMessageTypeUser},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := &MessageConverter{
				toolIDToFuncNameMap: make(map[string]string),
				validator:           NewImageValidator(),
				logger:              &NopLogger{},
			}

			msg := ChatMessage{Role: RoleUser, Content: tt.content}
			result := converter.convertUserMessage(msg)

			if len(result) != tt.expectedCount {
				t.Errorf("期望生成 %d 个消息，实际生成 %d 个", tt.expectedCount, len(result))
				return
			}

			for i, expectedType := range tt.expectedTypes {
				if i < len(result) && result[i].Type != expectedType {
					t.Errorf("消息 %d 类型错误，期望 '%s'，实际 '%s'",
						i, expectedType, result[i].Type)
				}
			}
		})
	}
}

// TestConvertTextContent 测试文本内容转换
func TestConvertTextContent(t *testing.T) {
	tests := []struct {
		name            string
		content         any
		expectedCount   int
		expectedContent []string
	}{
		{
			name:            "字符串内容",
			content:         "纯文本",
			expectedCount:   1,
			expectedContent: []string{"纯文本"},
		},
		{
			name: "单个文本块",
			content: []any{
				map[string]any{"type": "text", "text": "块内文本"},
			},
			expectedCount:   1,
			expectedContent: []string{"块内文本"},
		},
		{
			name: "多个文本块",
			content: []any{
				map[string]any{"type": "text", "text": "第一块"},
				map[string]any{"type": "text", "text": "第二块"},
				map[string]any{"type": "text", "text": "第三块"},
			},
			expectedCount:   3,
			expectedContent: []string{"第一块", "第二块", "第三块"},
		},
		{
			name: "空文本块被忽略",
			content: []any{
				map[string]any{"type": "text", "text": ""},
				map[string]any{"type": "text", "text": "有效内容"},
			},
			expectedCount:   1,
			expectedContent: []string{"有效内容"},
		},
		{
			name: "非文本类型被忽略",
			content: []any{
				map[string]any{"type": "image_url", "url": "xxx"},
				map[string]any{"type": "text", "text": "只有文本"},
			},
			expectedCount:   1,
			expectedContent: []string{"只有文本"},
		},
		{
			name:            "空数组",
			content:         []any{},
			expectedCount:   0,
			expectedContent: []string{},
		},
		{
			name:            "数字内容（不支持）",
			content:         12345,
			expectedCount:   1,
			expectedContent: []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := &MessageConverter{
				toolIDToFuncNameMap: make(map[string]string),
				validator:           NewImageValidator(),
				logger:              &NopLogger{},
			}

			result := converter.convertTextContent(tt.content)

			if len(result) != tt.expectedCount {
				t.Errorf("期望生成 %d 个消息，实际生成 %d 个", tt.expectedCount, len(result))
				return
			}

			for i, expectedContent := range tt.expectedContent {
				if i < len(result) && result[i].Content != expectedContent {
					t.Errorf("消息 %d 内容错误，期望 '%s'，实际 '%s'",
						i, expectedContent, result[i].Content)
				}
			}
		})
	}
}

// TestConvertAssistantToolCall 测试助手工具调用转换
func TestConvertAssistantToolCall(t *testing.T) {
	tests := []struct {
		name           string
		toolCall       ToolCall
		expectedID     string
		expectedName   string
		checkArgsValid bool
	}{
		{
			name: "正常工具调用",
			toolCall: ToolCall{
				ID:   "call_abc123",
				Type: ToolTypeFunction,
				Function: Function{
					Name:      "get_weather",
					Arguments: `{"city":"Beijing","unit":"celsius"}`,
				},
			},
			expectedID:     "call_abc123",
			expectedName:   "get_weather",
			checkArgsValid: true,
		},
		{
			name: "无效JSON参数保持原样",
			toolCall: ToolCall{
				ID:   "call_xyz",
				Type: ToolTypeFunction,
				Function: Function{
					Name:      "broken_tool",
					Arguments: `{invalid json`,
				},
			},
			expectedID:   "call_xyz",
			expectedName: "broken_tool",
		},
		{
			name: "空参数",
			toolCall: ToolCall{
				ID:   "call_empty",
				Type: ToolTypeFunction,
				Function: Function{
					Name:      "no_args_tool",
					Arguments: `{}`,
				},
			},
			expectedID:     "call_empty",
			expectedName:   "no_args_tool",
			checkArgsValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := &MessageConverter{
				toolIDToFuncNameMap: make(map[string]string),
				logger:              &NopLogger{},
			}

			result := converter.convertAssistantToolCall(tt.toolCall)

			if len(result) != 1 {
				t.Errorf("期望生成 1 个消息，实际生成 %d 个", len(result))
				return
			}

			if result[0].Type != JetBrainsMessageTypeAssistantTool {
				t.Errorf("消息类型错误，期望 '%s'，实际 '%s'",
					JetBrainsMessageTypeAssistantTool, result[0].Type)
			}

			if result[0].ID != tt.expectedID {
				t.Errorf("工具ID错误，期望 '%s'，实际 '%s'",
					tt.expectedID, result[0].ID)
			}

			if result[0].ToolName != tt.expectedName {
				t.Errorf("工具名称错误，期望 '%s'，实际 '%s'",
					tt.expectedName, result[0].ToolName)
			}
		})
	}
}
