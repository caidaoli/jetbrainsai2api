package main

import (
	"testing"

	"github.com/bytedance/sonic"
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

// TestParseJetbrainsToAnthropicDirect 测试直接响应解析
func TestParseJetbrainsToAnthropicDirect(t *testing.T) {
	model := "claude-3-5-sonnet-20241022"

	tests := []struct {
		name           string
		input          string
		wantErr        bool
		validateResult func(*testing.T, *AnthropicMessagesResponse)
	}{
		{
			name:  "纯文本响应",
			input: `{"content": "Hello, how can I help you?"}`,
			validateResult: func(t *testing.T, resp *AnthropicMessagesResponse) {
				if resp.Type != AnthropicTypeMessage {
					t.Errorf("期望 type=%s, 实际 type=%s", AnthropicTypeMessage, resp.Type)
				}
				if resp.Role != RoleAssistant {
					t.Errorf("期望 role=%s, 实际 role=%s", RoleAssistant, resp.Role)
				}
				if resp.Model != model {
					t.Errorf("期望 model=%s, 实际 model=%s", model, resp.Model)
				}
				if len(resp.Content) != 1 {
					t.Fatalf("期望 1 个内容块, 实际 %d 个", len(resp.Content))
				}
				if resp.Content[0].Type != ContentBlockTypeText {
					t.Errorf("期望内容类型为 text, 实际 %s", resp.Content[0].Type)
				}
				if resp.Content[0].Text != "Hello, how can I help you?" {
					t.Errorf("期望文本 'Hello, how can I help you?', 实际 '%s'", resp.Content[0].Text)
				}
				if resp.StopReason != StopReasonEndTurn {
					t.Errorf("期望 stop_reason=%s, 实际 %s", StopReasonEndTurn, resp.StopReason)
				}
			},
		},
		{
			name:  "空内容响应",
			input: `{"content": ""}`,
			validateResult: func(t *testing.T, resp *AnthropicMessagesResponse) {
				if len(resp.Content) != 0 {
					t.Errorf("期望空内容块, 实际有 %d 个", len(resp.Content))
				}
			},
		},
		{
			name:  "无content字段响应",
			input: `{"other_field": "value"}`,
			validateResult: func(t *testing.T, resp *AnthropicMessagesResponse) {
				if len(resp.Content) != 0 {
					t.Errorf("期望空内容块, 实际有 %d 个", len(resp.Content))
				}
			},
		},
		{
			name:    "非法JSON格式",
			input:   `{invalid json`,
			wantErr: true,
		},
		{
			name: "流式响应格式(应调用流式解析器)",
			input: `data: {"type":"Content","content":"Hello"}
data: {"type":"FinishMetadata","reason":"stop"}`,
			validateResult: func(t *testing.T, resp *AnthropicMessagesResponse) {
				// 这会委托给 parseJetbrainsStreamToAnthropic
				if len(resp.Content) != 1 {
					t.Fatalf("期望 1 个内容块, 实际 %d 个", len(resp.Content))
				}
				if resp.Content[0].Text != "Hello" {
					t.Errorf("期望文本 'Hello', 实际 '%s'", resp.Content[0].Text)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := parseJetbrainsToAnthropicDirect([]byte(tt.input), model)

			if tt.wantErr {
				if err == nil {
					t.Error("期望返回错误，但没有错误")
				}
				return
			}

			if err != nil {
				t.Fatalf("不期望错误，但得到: %v", err)
			}

			if resp == nil {
				t.Fatal("响应为 nil")
			}

			if tt.validateResult != nil {
				tt.validateResult(t, resp)
			}
		})
	}
}

// TestParseJetbrainsStreamToAnthropic 测试流式响应解析
func TestParseJetbrainsStreamToAnthropic(t *testing.T) {
	model := "claude-3-5-sonnet-20241022"

	tests := []struct {
		name           string
		input          string
		validateResult func(*testing.T, *AnthropicMessagesResponse)
	}{
		{
			name: "纯文本流式响应",
			input: `data: {"type":"Content","content":"Hello"}
data: {"type":"Content","content":" world"}
data: {"type":"FinishMetadata","reason":"stop"}`,
			validateResult: func(t *testing.T, resp *AnthropicMessagesResponse) {
				if len(resp.Content) != 1 {
					t.Fatalf("期望 1 个内容块, 实际 %d 个", len(resp.Content))
				}
				if resp.Content[0].Type != ContentBlockTypeText {
					t.Errorf("期望类型为 text, 实际 %s", resp.Content[0].Type)
				}
				if resp.Content[0].Text != "Hello world" {
					t.Errorf("期望文本 'Hello world', 实际 '%s'", resp.Content[0].Text)
				}
				if resp.StopReason != StopReasonEndTurn {
					t.Errorf("期望 stop_reason=%s, 实际 %s", StopReasonEndTurn, resp.StopReason)
				}
			},
		},
		{
			name: "工具调用流式响应",
			input: `data: {"type":"ToolCall","id":"toolu_123","name":"get_weather"}
data: {"type":"ToolCall","content":"{\"location\":\""}
data: {"type":"ToolCall","content":"Beijing\"}"}
data: {"type":"FinishMetadata","reason":"tool_call"}`,
			validateResult: func(t *testing.T, resp *AnthropicMessagesResponse) {
				if len(resp.Content) != 1 {
					t.Fatalf("期望 1 个内容块, 实际 %d 个", len(resp.Content))
				}
				if resp.Content[0].Type != ContentBlockTypeToolUse {
					t.Errorf("期望类型为 tool_use, 实际 %s", resp.Content[0].Type)
				}
				if resp.Content[0].ID != "toolu_123" {
					t.Errorf("期望 ID 'toolu_123', 实际 '%s'", resp.Content[0].ID)
				}
				if resp.Content[0].Name != "get_weather" {
					t.Errorf("期望名称 'get_weather', 实际 '%s'", resp.Content[0].Name)
				}
				location, ok := resp.Content[0].Input["location"].(string)
				if !ok || location != "Beijing" {
					t.Errorf("期望参数 location='Beijing', 实际 %v", resp.Content[0].Input)
				}
				if resp.StopReason != StopReasonToolUse {
					t.Errorf("期望 stop_reason=%s, 实际 %s", StopReasonToolUse, resp.StopReason)
				}
			},
		},
		{
			name: "文本+工具调用混合响应",
			input: `data: {"type":"Content","content":"I'll check the weather for you."}
data: {"type":"ToolCall","id":"toolu_456","name":"get_weather"}
data: {"type":"ToolCall","content":"{\"city\":\"Shanghai\"}"}
data: {"type":"FinishMetadata","reason":"tool_call"}`,
			validateResult: func(t *testing.T, resp *AnthropicMessagesResponse) {
				if len(resp.Content) != 2 {
					t.Fatalf("期望 2 个内容块, 实际 %d 个", len(resp.Content))
				}
				// 文本应该在前
				if resp.Content[0].Type != ContentBlockTypeText {
					t.Errorf("第一个块期望类型为 text, 实际 %s", resp.Content[0].Type)
				}
				if resp.Content[0].Text != "I'll check the weather for you." {
					t.Errorf("期望文本 'I'll check the weather for you.', 实际 '%s'", resp.Content[0].Text)
				}
				// 工具调用在后
				if resp.Content[1].Type != ContentBlockTypeToolUse {
					t.Errorf("第二个块期望类型为 tool_use, 实际 %s", resp.Content[1].Type)
				}
				if resp.Content[1].Name != "get_weather" {
					t.Errorf("期望工具名称 'get_weather', 实际 '%s'", resp.Content[1].Name)
				}
			},
		},
		{
			name: "finish_reason=length映射",
			input: `data: {"type":"Content","content":"Long response"}
data: {"type":"FinishMetadata","reason":"length"}`,
			validateResult: func(t *testing.T, resp *AnthropicMessagesResponse) {
				if resp.StopReason != StopReasonMaxTokens {
					t.Errorf("期望 stop_reason=%s, 实际 %s", StopReasonMaxTokens, resp.StopReason)
				}
			},
		},
		{
			name: "空数据处理",
			input: `

data: {"type":"Content","content":"Test"}

data: end`,
			validateResult: func(t *testing.T, resp *AnthropicMessagesResponse) {
				if len(resp.Content) != 1 {
					t.Fatalf("期望 1 个内容块, 实际 %d 个", len(resp.Content))
				}
				if resp.Content[0].Text != "Test" {
					t.Errorf("期望文本 'Test', 实际 '%s'", resp.Content[0].Text)
				}
			},
		},
		{
			name: "非法JSON行被跳过",
			input: `data: invalid json
data: {"type":"Content","content":"Valid"}
data: {"type":"FinishMetadata","reason":"stop"}`,
			validateResult: func(t *testing.T, resp *AnthropicMessagesResponse) {
				if len(resp.Content) != 1 {
					t.Fatalf("期望 1 个内容块, 实际 %d 个", len(resp.Content))
				}
				if resp.Content[0].Text != "Valid" {
					t.Errorf("期望文本 'Valid', 实际 '%s'", resp.Content[0].Text)
				}
			},
		},
		{
			name: "工具参数JSON解析失败时保留原始字符串",
			input: `data: {"type":"ToolCall","id":"toolu_789","name":"test_tool"}
data: {"type":"ToolCall","content":"invalid json"}
data: {"type":"FinishMetadata","reason":"tool_call"}`,
			validateResult: func(t *testing.T, resp *AnthropicMessagesResponse) {
				if len(resp.Content) != 1 {
					t.Fatalf("期望 1 个内容块, 实际 %d 个", len(resp.Content))
				}
				if resp.Content[0].Type != ContentBlockTypeToolUse {
					t.Errorf("期望类型为 tool_use, 实际 %s", resp.Content[0].Type)
				}
				// 应该保留原始字符串在 arguments 字段
				args, ok := resp.Content[0].Input["arguments"].(string)
				if !ok {
					t.Errorf("期望 arguments 字段为 string, 实际 %v", resp.Content[0].Input)
				}
				if args != "invalid json" {
					t.Errorf("期望 arguments='invalid json', 实际 '%s'", args)
				}
			},
		},
		{
			name: "多个工具调用",
			input: `data: {"type":"ToolCall","id":"toolu_001","name":"tool1"}
data: {"type":"ToolCall","content":"{\"param1\":\"value1\"}"}
data: {"type":"FinishMetadata","reason":"tool_call"}
data: {"type":"ToolCall","id":"toolu_002","name":"tool2"}
data: {"type":"ToolCall","content":"{\"param2\":\"value2\"}"}
data: {"type":"FinishMetadata","reason":"tool_call"}`,
			validateResult: func(t *testing.T, resp *AnthropicMessagesResponse) {
				// 实际上两个 FinishMetadata 都会完成各自的工具调用
				if len(resp.Content) != 2 {
					t.Fatalf("期望 2 个内容块(两个工具调用), 实际 %d 个", len(resp.Content))
				}
				if resp.Content[0].Name != "tool1" {
					t.Errorf("期望第一个工具名称 'tool1', 实际 '%s'", resp.Content[0].Name)
				}
				if resp.Content[1].Name != "tool2" {
					t.Errorf("期望第二个工具名称 'tool2', 实际 '%s'", resp.Content[1].Name)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := parseJetbrainsStreamToAnthropic(tt.input, model)

			if err != nil {
				t.Fatalf("不期望错误，但得到: %v", err)
			}

			if resp == nil {
				t.Fatal("响应为 nil")
			}

			// 验证基础字段
			if resp.Type != AnthropicTypeMessage {
				t.Errorf("期望 type=%s, 实际 type=%s", AnthropicTypeMessage, resp.Type)
			}
			if resp.Role != RoleAssistant {
				t.Errorf("期望 role=%s, 实际 role=%s", RoleAssistant, resp.Role)
			}
			if resp.Model != model {
				t.Errorf("期望 model=%s, 实际 model=%s", model, resp.Model)
			}

			if tt.validateResult != nil {
				tt.validateResult(t, resp)
			}
		})
	}
}

// TestParseJetbrainsToAnthropicDirectEdgeCases 测试边界情况
func TestParseJetbrainsToAnthropicDirectEdgeCases(t *testing.T) {
	model := "test-model"

	t.Run("多个内容块(content作为数组)", func(t *testing.T) {
		// 注意：当前实现只支持 content 为 string
		// 如果是数组会被忽略，这个测试验证这个行为
		input := `{"content": ["block1", "block2"]}`
		resp, err := parseJetbrainsToAnthropicDirect([]byte(input), model)
		if err != nil {
			t.Fatalf("不期望错误，但得到: %v", err)
		}
		// content 数组会被忽略，因为类型断言为 string 失败
		if len(resp.Content) != 0 {
			t.Errorf("期望空内容(因为content是数组而非字符串), 实际有 %d 个块", len(resp.Content))
		}
	})

	t.Run("极长文本内容", func(t *testing.T) {
		longText := string(make([]byte, 10000))
		for i := range longText {
			longText = longText[:i] + "x"
		}
		inputMap := map[string]any{"content": longText}
		inputBytes, _ := sonic.Marshal(inputMap)

		resp, err := parseJetbrainsToAnthropicDirect(inputBytes, model)
		if err != nil {
			t.Fatalf("不期望错误，但得到: %v", err)
		}
		if len(resp.Content) != 1 || len(resp.Content[0].Text) == 0 {
			t.Error("期望能处理长文本")
		}
	})

	t.Run("特殊字符处理", func(t *testing.T) {
		specialChars := `{"content": "包含换行\n和引号\"以及emoji 😊"}`
		resp, err := parseJetbrainsToAnthropicDirect([]byte(specialChars), model)
		if err != nil {
			t.Fatalf("不期望错误，但得到: %v", err)
		}
		if len(resp.Content) != 1 {
			t.Fatalf("期望 1 个内容块, 实际 %d 个", len(resp.Content))
		}
		expected := "包含换行\n和引号\"以及emoji 😊"
		if resp.Content[0].Text != expected {
			t.Errorf("特殊字符处理不正确，期望 '%s', 实际 '%s'", expected, resp.Content[0].Text)
		}
	})
}
