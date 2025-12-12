package main

import (
	"fmt"
	"time"
)

// generateAnthropicStreamResponse 生成 Anthropic 格式的流式响应
// 支持 message_start, content_block_start, content_block_delta, content_block_stop, message_stop 事件
func generateAnthropicStreamResponse(responseType string, content string, index int) []byte {
	var resp AnthropicStreamResponse

	switch responseType {
	case StreamEventTypeContentBlockStart:
		resp = AnthropicStreamResponse{
			Type:  StreamEventTypeContentBlockStart,
			Index: &index,
		}

	case StreamEventTypeContentBlockDelta:
		resp = AnthropicStreamResponse{
			Type:  StreamEventTypeContentBlockDelta,
			Index: &index,
			Delta: &struct {
				Type string `json:"type,omitempty"`
				Text string `json:"text,omitempty"`
			}{
				Type: AnthropicDeltaTypeText,
				Text: content,
			},
		}

	case StreamEventTypeContentBlockStop:
		resp = AnthropicStreamResponse{
			Type:  StreamEventTypeContentBlockStop,
			Index: &index,
		}

	case StreamEventTypeMessageStart:
		resp = AnthropicStreamResponse{
			Type: StreamEventTypeMessageStart,
			Message: &AnthropicMessagesResponse{
				ID:   generateMessageID(),
				Type: AnthropicTypeMessage,
				Role: RoleAssistant,
				Usage: AnthropicUsage{
					InputTokens:  0,
					OutputTokens: 0,
				},
			},
		}

	case StreamEventTypeMessageStop:
		resp = AnthropicStreamResponse{
			Type: StreamEventTypeMessageStop,
		}

	default:
		resp = AnthropicStreamResponse{
			Type: "error",
		}
	}

	data, _ := marshalJSON(resp)
	return data
}

// generateMessageID 生成 Anthropic 消息 ID
func generateMessageID() string {
	return fmt.Sprintf("%s%d", MessageIDPrefix, time.Now().UnixNano())
}
