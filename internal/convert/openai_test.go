package convert

import (
	"testing"

	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/validate"
)

func TestOpenAIToJetbrainsMessages_MultipleTextContent(t *testing.T) {
	messages := []core.ChatMessage{
		{
			Role: core.RoleUser,
			Content: []any{
				map[string]any{"type": core.ContentBlockTypeText, "text": "第一条消息内容"},
				map[string]any{"type": core.ContentBlockTypeText, "text": "第二条消息内容"},
				map[string]any{"type": core.ContentBlockTypeText, "text": "第三条消息内容"},
			},
		},
	}
	result := OpenAIToJetbrainsMessages(messages)
	if len(result) != 3 {
		t.Errorf("期望生成 3 个消息，实际生成 %d 个", len(result))
	}
	for i, msg := range result {
		if msg.Type != core.JetBrainsMessageTypeUser {
			t.Errorf("消息 %d 类型错误，期望 '%s'，实际 '%s'", i, core.JetBrainsMessageTypeUser, msg.Type)
		}
	}
	expectedContents := []string{"第一条消息内容", "第二条消息内容", "第三条消息内容"}
	for i, ec := range expectedContents {
		if result[i].Content != ec {
			t.Errorf("消息 %d 内容错误，期望 '%s'，实际 '%s'", i, ec, result[i].Content)
		}
	}
}

func TestOpenAIToJetbrainsMessages_SingleTextContent(t *testing.T) {
	messages := []core.ChatMessage{{Role: core.RoleUser, Content: "单一文本消息"}}
	result := OpenAIToJetbrainsMessages(messages)
	if len(result) != 1 {
		t.Errorf("期望 1 个消息，实际 %d 个", len(result))
	}
	if result[0].Content != "单一文本消息" {
		t.Errorf("内容错误")
	}
}

func TestConvertAssistantMessage_MultipleToolCalls(t *testing.T) {
	tests := []struct {
		name              string
		toolCalls         []core.ToolCall
		expectedCount     int
		expectedToolNames []string
		expectedToolIDs   []string
	}{
		{
			name:              "单个工具调用",
			toolCalls:         []core.ToolCall{{ID: "call_1", Type: core.ToolTypeFunction, Function: core.Function{Name: "get_weather", Arguments: `{"city":"Beijing"}`}}},
			expectedCount:     1,
			expectedToolNames: []string{"get_weather"},
			expectedToolIDs:   []string{"call_1"},
		},
		{
			name: "三个工具调用",
			toolCalls: []core.ToolCall{
				{ID: "call_1", Type: core.ToolTypeFunction, Function: core.Function{Name: "read_file", Arguments: `{"path":"/tmp/a.txt"}`}},
				{ID: "call_2", Type: core.ToolTypeFunction, Function: core.Function{Name: "write_file", Arguments: `{"path":"/tmp/b.txt"}`}},
				{ID: "call_3", Type: core.ToolTypeFunction, Function: core.Function{Name: "delete_file", Arguments: `{"path":"/tmp/c.txt"}`}},
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
				logger:              &core.NopLogger{},
			}
			msg := core.ChatMessage{Role: core.RoleAssistant, ToolCalls: tt.toolCalls}
			result := converter.convertAssistantMessage(msg)
			if len(result) != tt.expectedCount {
				t.Errorf("期望 %d 个消息，实际 %d 个", tt.expectedCount, len(result))
				return
			}
			for i := range tt.expectedToolNames {
				if result[i].ToolName != tt.expectedToolNames[i] {
					t.Errorf("消息 %d 工具名称错误", i)
				}
				if result[i].ID != tt.expectedToolIDs[i] {
					t.Errorf("消息 %d 工具ID错误", i)
				}
			}
		})
	}
}

func TestConvertAssistantMessage_NoToolCalls(t *testing.T) {
	converter := &MessageConverter{toolIDToFuncNameMap: make(map[string]string), logger: &core.NopLogger{}}
	msg := core.ChatMessage{Role: core.RoleAssistant, Content: "这是一个普通的文本回复"}
	result := converter.convertAssistantMessage(msg)
	if len(result) != 1 || result[0].Type != core.JetBrainsMessageTypeAssistantText {
		t.Errorf("期望 assistant_message_text")
	}
}

func TestConvertToolMessage(t *testing.T) {
	converter := &MessageConverter{toolIDToFuncNameMap: map[string]string{"call_123": "get_weather"}, logger: &core.NopLogger{}}
	msg := core.ChatMessage{Role: core.RoleTool, ToolCallID: "call_123", Content: `{"temperature": 25}`}
	result := converter.convertToolMessage(msg)
	if len(result) != 1 || result[0].Type != core.JetBrainsMessageTypeTool {
		t.Errorf("期望 tool_message")
	}

	// 缺少函数名映射
	converter2 := &MessageConverter{toolIDToFuncNameMap: make(map[string]string), logger: &core.NopLogger{}}
	msg2 := core.ChatMessage{Role: core.RoleTool, ToolCallID: "unknown", Content: "result"}
	result2 := converter2.convertToolMessage(msg2)
	if result2 != nil {
		t.Errorf("期望返回 nil")
	}
}

func TestConvertSystemMessage(t *testing.T) {
	converter := &MessageConverter{toolIDToFuncNameMap: make(map[string]string), logger: &core.NopLogger{}}
	msg := core.ChatMessage{Role: core.RoleSystem, Content: "你是一个有帮助的助手"}
	result := converter.convertSystemMessage(msg)
	if len(result) != 1 || result[0].Type != core.JetBrainsMessageTypeSystem {
		t.Errorf("消息类型错误")
	}
}

func TestBuildToolIDMap(t *testing.T) {
	converter := &MessageConverter{toolIDToFuncNameMap: make(map[string]string), logger: &core.NopLogger{}}
	messages := []core.ChatMessage{
		{Role: core.RoleAssistant, ToolCalls: []core.ToolCall{
			{ID: "call_1", Function: core.Function{Name: "func_a"}},
			{ID: "call_2", Function: core.Function{Name: "func_b"}},
		}},
		{Role: core.RoleUser, Content: "普通消息"},
		{Role: core.RoleAssistant, ToolCalls: []core.ToolCall{
			{ID: "call_3", Function: core.Function{Name: "func_c"}},
		}},
	}
	converter.buildToolIDMap(messages)
	expected := map[string]string{"call_1": "func_a", "call_2": "func_b", "call_3": "func_c"}
	for id, name := range expected {
		if converter.toolIDToFuncNameMap[id] != name {
			t.Errorf("工具ID %s 映射错误", id)
		}
	}
}

func TestMessageConverterConvert(t *testing.T) {
	messages := []core.ChatMessage{
		{Role: core.RoleSystem, Content: "系统提示"},
		{Role: core.RoleUser, Content: "用户消息"},
		{Role: core.RoleAssistant, Content: "助手回复"},
	}
	converter := &MessageConverter{
		toolIDToFuncNameMap: make(map[string]string),
		validator:           validate.NewImageValidator(),
		logger:              &core.NopLogger{},
	}
	result := converter.Convert(messages)
	if len(result) != 3 {
		t.Errorf("期望 3 个消息，实际 %d 个", len(result))
	}
	expectedTypes := []string{core.JetBrainsMessageTypeSystem, core.JetBrainsMessageTypeUser, core.JetBrainsMessageTypeAssistantText}
	for i, et := range expectedTypes {
		if result[i].Type != et {
			t.Errorf("消息 %d 类型错误，期望 '%s'，实际 '%s'", i, et, result[i].Type)
		}
	}
}

func TestConvertTextContent(t *testing.T) {
	converter := &MessageConverter{toolIDToFuncNameMap: make(map[string]string), validator: validate.NewImageValidator(), logger: &core.NopLogger{}}
	tests := []struct {
		name            string
		content         any
		expectedCount   int
		expectedContent []string
	}{
		{"字符串内容", "纯文本", 1, []string{"纯文本"}},
		{"多个文本块", []any{
			map[string]any{"type": "text", "text": "第一块"},
			map[string]any{"type": "text", "text": "第二块"},
		}, 2, []string{"第一块", "第二块"}},
		{"空数组", []any{}, 0, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := converter.convertTextContent(tt.content)
			if len(result) != tt.expectedCount {
				t.Errorf("期望 %d 个消息，实际 %d 个", tt.expectedCount, len(result))
			}
		})
	}
}
