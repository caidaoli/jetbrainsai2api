package main

import (
	"fmt"
	"strings"

	"github.com/bytedance/sonic"
)

// parseJetbrainsToAnthropicDirect 直接将 JetBrains 响应转换为 Anthropic 格式
// KISS: 消除不必要的中间转换步骤
func parseJetbrainsToAnthropicDirect(body []byte, model string, logger Logger) (*AnthropicMessagesResponse, error) {
	bodyStr := string(body)

	// 检查是否是流式响应格式
	if strings.HasPrefix(strings.TrimSpace(bodyStr), "data:") {
		return parseJetbrainsStreamToAnthropic(bodyStr, model, logger)
	}

	// 尝试解析为完整的聊天响应
	var jetbrainsResp map[string]any
	if err := sonic.Unmarshal(body, &jetbrainsResp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// 直接构建 Anthropic 响应
	var content []AnthropicContentBlock
	stopReason := StopReasonEndTurn

	// 提取文本内容
	if contentField, exists := jetbrainsResp["content"]; exists {
		if contentStr, ok := contentField.(string); ok && contentStr != "" {
			content = append(content, AnthropicContentBlock{
				Type: ContentBlockTypeText,
				Text: contentStr,
			})
		}
	}

	response := &AnthropicMessagesResponse{
		ID:         generateMessageID(),
		Type:       AnthropicTypeMessage,
		Role:       RoleAssistant,
		Content:    content,
		Model:      model,
		StopReason: stopReason,
		Usage: AnthropicUsage{
			InputTokens:  0,
			OutputTokens: estimateTokenCount(getContentText(content)),
		},
	}

	logger.Debug("Direct JetBrains→Anthropic conversion: id=%s, content_blocks=%d",
		response.ID, len(response.Content))

	return response, nil
}

// parseJetbrainsStreamToAnthropic 解析 JetBrains 流式响应为 Anthropic 格式
// SRP: 专门处理流式响应的单一职责
func parseJetbrainsStreamToAnthropic(bodyStr, model string, logger Logger) (*AnthropicMessagesResponse, error) {
	lines := strings.Split(bodyStr, "\n")
	var content []AnthropicContentBlock
	var currentToolCall *AnthropicContentBlock
	var textParts []string
	finishReason := StopReasonEndTurn

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == StreamEndLine {
			continue
		}

		if strings.HasPrefix(line, StreamChunkPrefix) {
			jsonData := strings.TrimPrefix(line, StreamChunkPrefix)
			var streamData map[string]any
			if err := sonic.Unmarshal([]byte(jsonData), &streamData); err != nil {
				logger.Debug("Failed to parse stream JSON: %v", err)
				continue
			}

			eventType, _ := streamData["type"].(string)
			switch eventType {
			case JetBrainsEventTypeContent:
				// 文本内容
				if text, ok := streamData["content"].(string); ok {
					textParts = append(textParts, text)
				}
			case JetBrainsEventTypeToolCall:
				// 工具调用处理
				if upstreamID, ok := streamData["id"].(string); ok && upstreamID != "" {
					// 开始新的工具调用
					if name, ok := streamData["name"].(string); ok && name != "" {
						currentToolCall = &AnthropicContentBlock{
							Type:  ContentBlockTypeToolUse,
							ID:    upstreamID,
							Name:  name,
							Input: make(map[string]any),
						}
						logger.Debug("Started tool call: id=%s, name=%s", upstreamID, name)
					}
				} else if currentToolCall != nil {
					// 累积工具参数
					if contentStr, ok := streamData["content"].(string); ok {
						// 这里需要解析JSON参数
						if currentToolCall.Input == nil {
							currentToolCall.Input = make(map[string]any)
						}
						// 累积参数字符串，在FinishMetadata时解析
						if existing, exists := currentToolCall.Input["_raw_args"]; exists {
							currentToolCall.Input["_raw_args"] = existing.(string) + contentStr
						} else {
							currentToolCall.Input["_raw_args"] = contentStr
						}
					}
				}
			case JetBrainsEventTypeFinishMetadata:
				// 完成处理
				if reasonStr, ok := streamData["reason"].(string); ok {
					finishReason = mapJetbrainsFinishReason(reasonStr)
				}

				// 完成工具调用
				if currentToolCall != nil {
					if rawArgs, exists := currentToolCall.Input["_raw_args"]; exists {
						// 解析累积的JSON参数
						var parsedArgs map[string]any
						if err := sonic.Unmarshal([]byte(rawArgs.(string)), &parsedArgs); err == nil {
							currentToolCall.Input = parsedArgs
						} else {
							// 如果JSON解析失败，保留原始字符串
							currentToolCall.Input = map[string]any{"arguments": rawArgs.(string)}
						}
					}
					content = append(content, *currentToolCall)
					logger.Debug("Completed tool call: id=%s, args=%v", currentToolCall.ID, currentToolCall.Input)
					currentToolCall = nil
				}
			}
		}
	}

	// 添加文本内容
	if len(textParts) > 0 {
		fullText := strings.Join(textParts, "")
		if fullText != "" {
			textContent := AnthropicContentBlock{
				Type: ContentBlockTypeText,
				Text: fullText,
			}
			// 将文本内容放在工具调用之前
			content = append([]AnthropicContentBlock{textContent}, content...)
		}
	}

	response := &AnthropicMessagesResponse{
		ID:         generateMessageID(),
		Type:       AnthropicTypeMessage,
		Role:       RoleAssistant,
		Content:    content,
		Model:      model,
		StopReason: finishReason,
		Usage: AnthropicUsage{
			InputTokens:  0,
			OutputTokens: 0,
		},
	}

	logger.Debug("Successfully parsed JetBrains stream to Anthropic: content_blocks=%d, finish_reason=%s",
		len(content), finishReason)

	return response, nil
}

// mapJetbrainsFinishReason 映射 JetBrains 结束原因到 Anthropic 格式
// KISS: 简单的映射逻辑
func mapJetbrainsFinishReason(jetbrainsReason string) string {
	switch jetbrainsReason {
	case JetBrainsFinishReasonToolCall:
		return StopReasonToolUse
	case JetBrainsFinishReasonLength:
		return StopReasonMaxTokens
	case JetBrainsFinishReasonStop:
		return StopReasonEndTurn
	default:
		return StopReasonEndTurn
	}
}

// getContentText 提取内容块中的文本用于token估算
// DRY: 复用的工具函数
func getContentText(content []AnthropicContentBlock) string {
	var textParts []string
	for _, block := range content {
		if block.Type == ContentBlockTypeText && block.Text != "" {
			textParts = append(textParts, block.Text)
		}
	}
	return strings.Join(textParts, " ")
}

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

	data, err := marshalJSON(resp)
	if err != nil {
		// marshalJSON 不应对我们自己构造的结构体失败
		return []byte{}
	}
	return data
}

// generateMessageID 生成 Anthropic 消息 ID
func generateMessageID() string {
	return generateID(MessageIDPrefix)
}
