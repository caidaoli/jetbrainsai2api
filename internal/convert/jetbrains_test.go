package convert

import (
	"encoding/json"
	"strings"
	"testing"

	"jetbrainsai2api/internal/core"

	"github.com/bytedance/sonic"
)

func TestMapJetbrainsFinishReason(t *testing.T) {
	tests := []struct {
		name, input, expected string
	}{
		{"tool_call映射到tool_use", core.JetBrainsFinishReasonToolCall, core.StopReasonToolUse},
		{"length映射到max_tokens", core.JetBrainsFinishReasonLength, core.StopReasonMaxTokens},
		{"stop映射到end_turn", core.JetBrainsFinishReasonStop, core.StopReasonEndTurn},
		{"未知值默认映射到end_turn", "unknown", core.StopReasonEndTurn},
		{"空字符串默认映射到end_turn", "", core.StopReasonEndTurn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if result := MapJetbrainsFinishReason(tt.input); result != tt.expected {
				t.Errorf("期望 '%s'，实际 '%s'", tt.expected, result)
			}
		})
	}
}

func TestGetContentText(t *testing.T) {
	tests := []struct {
		name     string
		content  []core.AnthropicContentBlock
		expected string
	}{
		{"空内容", []core.AnthropicContentBlock{}, ""},
		{"nil内容", nil, ""},
		{"单个text块", []core.AnthropicContentBlock{{Type: core.ContentBlockTypeText, Text: "Hello World"}}, "Hello World"},
		{"多个text块", []core.AnthropicContentBlock{
			{Type: core.ContentBlockTypeText, Text: "First"},
			{Type: core.ContentBlockTypeText, Text: "Second"},
		}, "First Second"},
		{"混合类型块", []core.AnthropicContentBlock{
			{Type: core.ContentBlockTypeText, Text: "Text before"},
			{Type: core.ContentBlockTypeToolUse, ID: "toolu_123"},
			{Type: core.ContentBlockTypeText, Text: "Text after"},
		}, "Text before Text after"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if result := GetContentText(tt.content); result != tt.expected {
				t.Errorf("期望 '%s'，实际 '%s'", tt.expected, result)
			}
		})
	}
}

func TestParseJetbrainsToAnthropicDirect(t *testing.T) {
	model := "claude-3-5-sonnet-20241022"
	tests := []struct {
		name           string
		input          string
		wantErr        bool
		validateResult func(*testing.T, *core.AnthropicMessagesResponse)
	}{
		{
			name:  "纯文本响应",
			input: `{"content": "Hello, how can I help you?"}`,
			validateResult: func(t *testing.T, resp *core.AnthropicMessagesResponse) {
				if resp.Type != core.AnthropicTypeMessage {
					t.Errorf("期望 type=%s, 实际 type=%s", core.AnthropicTypeMessage, resp.Type)
				}
				if len(resp.Content) != 1 || resp.Content[0].Text != "Hello, how can I help you?" {
					t.Errorf("内容不匹配")
				}
			},
		},
		{name: "非法JSON格式", input: `{invalid json`, wantErr: true},
		{
			name:  "流式响应格式",
			input: "data: {\"type\":\"Content\",\"content\":\"Hello\"}\ndata: {\"type\":\"FinishMetadata\",\"reason\":\"stop\"}",
			validateResult: func(t *testing.T, resp *core.AnthropicMessagesResponse) {
				if len(resp.Content) != 1 || resp.Content[0].Text != "Hello" {
					t.Errorf("内容不匹配")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := ParseJetbrainsToAnthropicDirect([]byte(tt.input), model, &core.NopLogger{})
			if tt.wantErr {
				if err == nil {
					t.Error("期望返回错误，但没有错误")
				}
				return
			}
			if err != nil {
				t.Fatalf("不期望错误，但得到: %v", err)
			}
			if tt.validateResult != nil {
				tt.validateResult(t, resp)
			}
		})
	}
}

func TestParseJetbrainsStreamToAnthropic(t *testing.T) {
	model := "claude-3-5-sonnet-20241022"
	tests := []struct {
		name           string
		input          string
		validateResult func(*testing.T, *core.AnthropicMessagesResponse)
	}{
		{
			name:  "纯文本流式响应",
			input: "data: {\"type\":\"Content\",\"content\":\"Hello\"}\ndata: {\"type\":\"Content\",\"content\":\" world\"}\ndata: {\"type\":\"FinishMetadata\",\"reason\":\"stop\"}",
			validateResult: func(t *testing.T, resp *core.AnthropicMessagesResponse) {
				if len(resp.Content) != 1 || resp.Content[0].Text != "Hello world" {
					t.Errorf("期望 'Hello world', 实际 '%s'", resp.Content[0].Text)
				}
			},
		},
		{
			name:  "工具调用流式响应",
			input: "data: {\"type\":\"ToolCall\",\"id\":\"toolu_123\",\"name\":\"get_weather\"}\ndata: {\"type\":\"ToolCall\",\"content\":\"{\\\"location\\\":\\\"\"}\ndata: {\"type\":\"ToolCall\",\"content\":\"Beijing\\\"}\"}\ndata: {\"type\":\"FinishMetadata\",\"reason\":\"tool_call\"}",
			validateResult: func(t *testing.T, resp *core.AnthropicMessagesResponse) {
				if len(resp.Content) != 1 || resp.Content[0].Type != core.ContentBlockTypeToolUse {
					t.Errorf("期望 tool_use 块")
				}
				if resp.StopReason != core.StopReasonToolUse {
					t.Errorf("期望 stop_reason=%s", core.StopReasonToolUse)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := ParseJetbrainsStreamToAnthropic(tt.input, model, &core.NopLogger{})
			if err != nil {
				t.Fatalf("不期望错误: %v", err)
			}
			if resp.Type != core.AnthropicTypeMessage || resp.Role != core.RoleAssistant {
				t.Errorf("基础字段错误")
			}
			if tt.validateResult != nil {
				tt.validateResult(t, resp)
			}
		})
	}
}

func TestParseJetbrainsToAnthropicDirectEdgeCases(t *testing.T) {
	model := "test-model"
	t.Run("极长文本内容", func(t *testing.T) {
		longText := strings.Repeat("x", 10000)
		inputMap := map[string]any{"content": longText}
		inputBytes, _ := sonic.Marshal(inputMap)
		resp, err := ParseJetbrainsToAnthropicDirect(inputBytes, model, &core.NopLogger{})
		if err != nil {
			t.Fatalf("不期望错误: %v", err)
		}
		if len(resp.Content) != 1 || len(resp.Content[0].Text) == 0 {
			t.Error("期望能处理长文本")
		}
	})
}

func TestGenerateMessageID(t *testing.T) {
	id := GenerateMessageID()
	if !strings.HasPrefix(id, core.MessageIDPrefix) {
		t.Errorf("消息ID应以 '%s' 为前缀，实际: '%s'", core.MessageIDPrefix, id)
	}
	if len(id) < len(core.MessageIDPrefix)+10 {
		t.Errorf("消息ID长度过短: %s", id)
	}
}

func TestGenerateAnthropicStreamResponse(t *testing.T) {
	tests := []struct {
		name         string
		responseType string
		content      string
		index        int
		validate     func(t *testing.T, data []byte)
	}{
		{
			name:         "message_start事件",
			responseType: core.StreamEventTypeMessageStart,
			validate: func(t *testing.T, data []byte) {
				var resp core.AnthropicStreamResponse
				if err := json.Unmarshal(data, &resp); err != nil {
					t.Fatalf("解析JSON失败: %v", err)
				}
				if resp.Type != core.StreamEventTypeMessageStart {
					t.Errorf("期望 type=%s, 实际=%s", core.StreamEventTypeMessageStart, resp.Type)
				}
				if resp.Message == nil {
					t.Fatal("message 字段不应为 nil")
				}
			},
		},
		{
			name:         "content_block_delta事件-文本内容",
			responseType: core.StreamEventTypeContentBlockDelta,
			content:      "Hello, world!",
			validate: func(t *testing.T, data []byte) {
				var resp core.AnthropicStreamResponse
				if err := json.Unmarshal(data, &resp); err != nil {
					t.Fatalf("解析JSON失败: %v", err)
				}
				if resp.Delta == nil || resp.Delta.Text != "Hello, world!" {
					t.Errorf("delta text 不匹配")
				}
			},
		},
		{
			name:         "message_stop事件",
			responseType: core.StreamEventTypeMessageStop,
			validate: func(t *testing.T, data []byte) {
				var resp core.AnthropicStreamResponse
				if err := json.Unmarshal(data, &resp); err != nil {
					t.Fatalf("解析JSON失败: %v", err)
				}
				if resp.Type != core.StreamEventTypeMessageStop {
					t.Errorf("期望 type=%s", core.StreamEventTypeMessageStop)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := GenerateAnthropicStreamResponse(tt.responseType, tt.content, tt.index)
			if len(data) == 0 || !json.Valid(data) {
				t.Fatalf("返回数据无效")
			}
			if tt.validate != nil {
				tt.validate(t, data)
			}
		})
	}
}
