package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestGenerateMessageID 测试消息ID生成
func TestGenerateMessageID(t *testing.T) {
	id := generateMessageID()

	// 验证前缀
	if !strings.HasPrefix(id, MessageIDPrefix) {
		t.Errorf("消息ID应以 '%s' 为前缀，实际: '%s'", MessageIDPrefix, id)
	}

	// 验证长度合理（前缀 + 纳秒时间戳）
	if len(id) < len(MessageIDPrefix)+10 {
		t.Errorf("消息ID长度过短: %s", id)
	}

	// 验证格式：前缀后应该是数字
	numPart := id[len(MessageIDPrefix):]
	for _, c := range numPart {
		if c < '0' || c > '9' {
			t.Errorf("消息ID数字部分包含非数字字符: %c in %s", c, id)
			break
		}
	}
}

// TestGenerateAnthropicStreamResponse 测试 Anthropic 流式响应生成
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
			responseType: StreamEventTypeMessageStart,
			content:      "",
			index:        0,
			validate: func(t *testing.T, data []byte) {
				var resp AnthropicStreamResponse
				if err := json.Unmarshal(data, &resp); err != nil {
					t.Fatalf("解析JSON失败: %v", err)
				}

				// 验证事件类型
				if resp.Type != StreamEventTypeMessageStart {
					t.Errorf("期望 type=%s, 实际=%s", StreamEventTypeMessageStart, resp.Type)
				}

				// 验证 message 字段存在
				if resp.Message == nil {
					t.Fatal("message 字段不应为 nil")
				}

				// 验证 message 结构
				if resp.Message.Type != AnthropicTypeMessage {
					t.Errorf("期望 message.type=%s, 实际=%s", AnthropicTypeMessage, resp.Message.Type)
				}
				if resp.Message.Role != RoleAssistant {
					t.Errorf("期望 message.role=%s, 实际=%s", RoleAssistant, resp.Message.Role)
				}
				if !strings.HasPrefix(resp.Message.ID, MessageIDPrefix) {
					t.Errorf("message.id 应以 '%s' 开头，实际=%s", MessageIDPrefix, resp.Message.ID)
				}

				// 验证 usage 初始化为0
				if resp.Message.Usage.InputTokens != 0 {
					t.Errorf("期望 usage.input_tokens=0, 实际=%d", resp.Message.Usage.InputTokens)
				}
				if resp.Message.Usage.OutputTokens != 0 {
					t.Errorf("期望 usage.output_tokens=0, 实际=%d", resp.Message.Usage.OutputTokens)
				}
			},
		},
		{
			name:         "content_block_start事件",
			responseType: StreamEventTypeContentBlockStart,
			content:      "",
			index:        0,
			validate: func(t *testing.T, data []byte) {
				var resp AnthropicStreamResponse
				if err := json.Unmarshal(data, &resp); err != nil {
					t.Fatalf("解析JSON失败: %v", err)
				}

				// 验证事件类型
				if resp.Type != StreamEventTypeContentBlockStart {
					t.Errorf("期望 type=%s, 实际=%s", StreamEventTypeContentBlockStart, resp.Type)
				}

				// 验证 index 字段存在且值正确
				if resp.Index == nil {
					t.Fatal("index 字段不应为 nil")
				}
				if *resp.Index != 0 {
					t.Errorf("期望 index=0, 实际=%d", *resp.Index)
				}

				// 验证 delta 字段不存在
				if resp.Delta != nil {
					t.Error("delta 字段应为 nil")
				}
			},
		},
		{
			name:         "content_block_delta事件-文本内容",
			responseType: StreamEventTypeContentBlockDelta,
			content:      "Hello, world!",
			index:        0,
			validate: func(t *testing.T, data []byte) {
				var resp AnthropicStreamResponse
				if err := json.Unmarshal(data, &resp); err != nil {
					t.Fatalf("解析JSON失败: %v", err)
				}

				// 验证事件类型
				if resp.Type != StreamEventTypeContentBlockDelta {
					t.Errorf("期望 type=%s, 实际=%s", StreamEventTypeContentBlockDelta, resp.Type)
				}

				// 验证 index 字段
				if resp.Index == nil {
					t.Fatal("index 字段不应为 nil")
				}
				if *resp.Index != 0 {
					t.Errorf("期望 index=0, 实际=%d", *resp.Index)
				}

				// 验证 delta 字段
				if resp.Delta == nil {
					t.Fatal("delta 字段不应为 nil")
				}
				if resp.Delta.Type != AnthropicDeltaTypeText {
					t.Errorf("期望 delta.type=%s, 实际=%s", AnthropicDeltaTypeText, resp.Delta.Type)
				}
				if resp.Delta.Text != "Hello, world!" {
					t.Errorf("期望 delta.text='Hello, world!', 实际='%s'", resp.Delta.Text)
				}
			},
		},
		{
			name:         "content_block_delta事件-空文本",
			responseType: StreamEventTypeContentBlockDelta,
			content:      "",
			index:        1,
			validate: func(t *testing.T, data []byte) {
				var resp AnthropicStreamResponse
				if err := json.Unmarshal(data, &resp); err != nil {
					t.Fatalf("解析JSON失败: %v", err)
				}

				// 验证 index 变化
				if resp.Index == nil || *resp.Index != 1 {
					t.Errorf("期望 index=1, 实际=%v", resp.Index)
				}

				// 验证空文本
				if resp.Delta == nil {
					t.Fatal("delta 字段不应为 nil")
				}
				if resp.Delta.Text != "" {
					t.Errorf("期望 delta.text='', 实际='%s'", resp.Delta.Text)
				}
			},
		},
		{
			name:         "content_block_delta事件-中文内容",
			responseType: StreamEventTypeContentBlockDelta,
			content:      "你好，世界！",
			index:        2,
			validate: func(t *testing.T, data []byte) {
				var resp AnthropicStreamResponse
				if err := json.Unmarshal(data, &resp); err != nil {
					t.Fatalf("解析JSON失败: %v", err)
				}

				// 验证中文内容正确编码
				if resp.Delta == nil {
					t.Fatal("delta 字段不应为 nil")
				}
				if resp.Delta.Text != "你好，世界！" {
					t.Errorf("期望 delta.text='你好，世界！', 实际='%s'", resp.Delta.Text)
				}
			},
		},
		{
			name:         "content_block_stop事件",
			responseType: StreamEventTypeContentBlockStop,
			content:      "",
			index:        0,
			validate: func(t *testing.T, data []byte) {
				var resp AnthropicStreamResponse
				if err := json.Unmarshal(data, &resp); err != nil {
					t.Fatalf("解析JSON失败: %v", err)
				}

				// 验证事件类型
				if resp.Type != StreamEventTypeContentBlockStop {
					t.Errorf("期望 type=%s, 实际=%s", StreamEventTypeContentBlockStop, resp.Type)
				}

				// 验证 index 字段存在
				if resp.Index == nil {
					t.Fatal("index 字段不应为 nil")
				}
				if *resp.Index != 0 {
					t.Errorf("期望 index=0, 实际=%d", *resp.Index)
				}

				// 验证 delta 字段不存在
				if resp.Delta != nil {
					t.Error("delta 字段应为 nil")
				}
			},
		},
		{
			name:         "message_stop事件",
			responseType: StreamEventTypeMessageStop,
			content:      "",
			index:        0,
			validate: func(t *testing.T, data []byte) {
				var resp AnthropicStreamResponse
				if err := json.Unmarshal(data, &resp); err != nil {
					t.Fatalf("解析JSON失败: %v", err)
				}

				// 验证事件类型
				if resp.Type != StreamEventTypeMessageStop {
					t.Errorf("期望 type=%s, 实际=%s", StreamEventTypeMessageStop, resp.Type)
				}

				// 验证可选字段都为空
				if resp.Index != nil {
					t.Error("index 字段应为 nil")
				}
				if resp.Delta != nil {
					t.Error("delta 字段应为 nil")
				}
				if resp.Message != nil {
					t.Error("message 字段应为 nil")
				}
			},
		},
		{
			name:         "未知事件类型",
			responseType: "unknown_event",
			content:      "",
			index:        0,
			validate: func(t *testing.T, data []byte) {
				var resp AnthropicStreamResponse
				if err := json.Unmarshal(data, &resp); err != nil {
					t.Fatalf("解析JSON失败: %v", err)
				}

				// 验证返回错误类型
				if resp.Type != "error" {
					t.Errorf("未知事件应返回 type='error', 实际=%s", resp.Type)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 调用函数生成响应
			data := generateAnthropicStreamResponse(tt.responseType, tt.content, tt.index)

			// 验证返回非空
			if len(data) == 0 {
				t.Fatal("返回的数据不应为空")
			}

			// 验证是否为有效 JSON
			if !json.Valid(data) {
				t.Fatalf("返回的数据不是有效的 JSON: %s", string(data))
			}

			// 执行自定义验证
			tt.validate(t, data)
		})
	}
}

// TestGenerateAnthropicStreamResponse_JSONFormat 测试 JSON 格式正确性
func TestGenerateAnthropicStreamResponse_JSONFormat(t *testing.T) {
	// 测试各种事件的 JSON 序列化格式
	tests := []struct {
		name         string
		responseType string
		checkFields  func(t *testing.T, data map[string]any)
	}{
		{
			name:         "message_start包含必需字段",
			responseType: StreamEventTypeMessageStart,
			checkFields: func(t *testing.T, data map[string]any) {
				// 检查顶层字段
				if _, ok := data["type"]; !ok {
					t.Error("缺少 type 字段")
				}
				if _, ok := data["message"]; !ok {
					t.Error("缺少 message 字段")
				}

				// 检查 message 子字段
				if msg, ok := data["message"].(map[string]any); ok {
					requiredFields := []string{"id", "type", "role", "usage"}
					for _, field := range requiredFields {
						if _, exists := msg[field]; !exists {
							t.Errorf("message 缺少 %s 字段", field)
						}
					}
				} else {
					t.Error("message 字段格式错误")
				}
			},
		},
		{
			name:         "content_block_delta包含delta字段",
			responseType: StreamEventTypeContentBlockDelta,
			checkFields: func(t *testing.T, data map[string]any) {
				if _, ok := data["delta"]; !ok {
					t.Error("缺少 delta 字段")
				}

				// 验证 delta 结构
				if delta, ok := data["delta"].(map[string]any); ok {
					if _, exists := delta["type"]; !exists {
						t.Error("delta 缺少 type 字段")
					}
					if _, exists := delta["text"]; !exists {
						t.Error("delta 缺少 text 字段")
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := generateAnthropicStreamResponse(tt.responseType, "test", 0)

			// 解析为 map 进行字段检查
			var jsonMap map[string]any
			if err := json.Unmarshal(data, &jsonMap); err != nil {
				t.Fatalf("解析JSON失败: %v", err)
			}

			tt.checkFields(t, jsonMap)
		})
	}
}
