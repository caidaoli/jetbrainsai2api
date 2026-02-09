package convert

import (
	"strings"

	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/util"

	"github.com/bytedance/sonic"
)

// ParseJetbrainsToAnthropicDirect converts JetBrains response to Anthropic format
func ParseJetbrainsToAnthropicDirect(body []byte, model string, logger core.Logger) (*core.AnthropicMessagesResponse, error) {
	bodyStr := string(body)

	if strings.HasPrefix(strings.TrimSpace(bodyStr), "data:") {
		return ParseJetbrainsStreamToAnthropic(bodyStr, model, logger)
	}

	var jetbrainsResp map[string]any
	if err := sonic.Unmarshal(body, &jetbrainsResp); err != nil {
		return nil, err
	}

	var content []core.AnthropicContentBlock
	stopReason := core.StopReasonEndTurn

	if contentField, exists := jetbrainsResp["content"]; exists {
		if contentStr, ok := contentField.(string); ok && contentStr != "" {
			content = append(content, core.AnthropicContentBlock{
				Type: core.ContentBlockTypeText,
				Text: contentStr,
			})
		}
	}

	response := &core.AnthropicMessagesResponse{
		ID:         GenerateMessageID(),
		Type:       core.AnthropicTypeMessage,
		Role:       core.RoleAssistant,
		Content:    content,
		Model:      model,
		StopReason: stopReason,
		Usage: core.AnthropicUsage{
			InputTokens:  0,
			OutputTokens: util.EstimateTokenCount(GetContentText(content)),
		},
	}

	logger.Debug("Direct JetBrains→Anthropic conversion: id=%s, content_blocks=%d",
		response.ID, len(response.Content))

	return response, nil
}

// ParseJetbrainsStreamToAnthropic parses JetBrains streaming response to Anthropic format
func ParseJetbrainsStreamToAnthropic(bodyStr, model string, logger core.Logger) (*core.AnthropicMessagesResponse, error) {
	lines := strings.Split(bodyStr, "\n")
	var content []core.AnthropicContentBlock
	var currentToolCall *core.AnthropicContentBlock
	var textParts []string
	finishReason := core.StopReasonEndTurn

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == core.StreamEndLine {
			continue
		}

		if strings.HasPrefix(line, core.StreamChunkPrefix) {
			jsonData := strings.TrimPrefix(line, core.StreamChunkPrefix)
			var streamData map[string]any
			if err := sonic.Unmarshal([]byte(jsonData), &streamData); err != nil {
				logger.Debug("Failed to parse stream JSON: %v", err)
				continue
			}

			eventType, _ := streamData["type"].(string)
			switch eventType {
			case core.JetBrainsEventTypeContent:
				if text, ok := streamData["content"].(string); ok {
					textParts = append(textParts, text)
				}
			case core.JetBrainsEventTypeToolCall:
				if upstreamID, ok := streamData["id"].(string); ok && upstreamID != "" {
					if name, ok := streamData["name"].(string); ok && name != "" {
						currentToolCall = &core.AnthropicContentBlock{
							Type:  core.ContentBlockTypeToolUse,
							ID:    upstreamID,
							Name:  name,
							Input: make(map[string]any),
						}
						logger.Debug("Started tool call: id=%s, name=%s", upstreamID, name)
					}
				} else if currentToolCall != nil {
					if contentStr, ok := streamData["content"].(string); ok {
						if currentToolCall.Input == nil {
							currentToolCall.Input = make(map[string]any)
						}
						if existing, exists := currentToolCall.Input["_raw_args"]; exists {
							currentToolCall.Input["_raw_args"] = existing.(string) + contentStr
						} else {
							currentToolCall.Input["_raw_args"] = contentStr
						}
					}
				}
			case core.JetBrainsEventTypeFinishMetadata:
				if reasonStr, ok := streamData["reason"].(string); ok {
					finishReason = MapJetbrainsFinishReason(reasonStr)
				}

				if currentToolCall != nil {
					if rawArgs, exists := currentToolCall.Input["_raw_args"]; exists {
						var parsedArgs map[string]any
						if err := sonic.Unmarshal([]byte(rawArgs.(string)), &parsedArgs); err == nil {
							currentToolCall.Input = parsedArgs
						} else {
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

	if len(textParts) > 0 {
		fullText := strings.Join(textParts, "")
		if fullText != "" {
			textContent := core.AnthropicContentBlock{
				Type: core.ContentBlockTypeText,
				Text: fullText,
			}
			content = append([]core.AnthropicContentBlock{textContent}, content...)
		}
	}

	response := &core.AnthropicMessagesResponse{
		ID:         GenerateMessageID(),
		Type:       core.AnthropicTypeMessage,
		Role:       core.RoleAssistant,
		Content:    content,
		Model:      model,
		StopReason: finishReason,
		Usage: core.AnthropicUsage{
			InputTokens:  0,
			OutputTokens: 0,
		},
	}

	logger.Debug("Successfully parsed JetBrains stream to Anthropic: content_blocks=%d, finish_reason=%s",
		len(content), finishReason)

	return response, nil
}

// MapJetbrainsFinishReason maps JetBrains finish reason to Anthropic format
func MapJetbrainsFinishReason(jetbrainsReason string) string {
	switch jetbrainsReason {
	case core.JetBrainsFinishReasonToolCall:
		return core.StopReasonToolUse
	case core.JetBrainsFinishReasonLength:
		return core.StopReasonMaxTokens
	case core.JetBrainsFinishReasonStop:
		return core.StopReasonEndTurn
	default:
		return core.StopReasonEndTurn
	}
}

// GetContentText extracts text from content blocks for token estimation
func GetContentText(content []core.AnthropicContentBlock) string {
	var textParts []string
	for _, block := range content {
		if block.Type == core.ContentBlockTypeText && block.Text != "" {
			textParts = append(textParts, block.Text)
		}
	}
	return strings.Join(textParts, " ")
}

// GenerateAnthropicStreamResponse generates Anthropic format streaming response
func GenerateAnthropicStreamResponse(responseType string, content string, index int) []byte {
	var resp core.AnthropicStreamResponse

	switch responseType {
	case core.StreamEventTypeContentBlockStart:
		resp = core.AnthropicStreamResponse{
			Type:  core.StreamEventTypeContentBlockStart,
			Index: &index,
		}

	case core.StreamEventTypeContentBlockDelta:
		resp = core.AnthropicStreamResponse{
			Type:  core.StreamEventTypeContentBlockDelta,
			Index: &index,
			Delta: &struct {
				Type string `json:"type,omitempty"`
				Text string `json:"text,omitempty"`
			}{
				Type: core.AnthropicDeltaTypeText,
				Text: content,
			},
		}

	case core.StreamEventTypeContentBlockStop:
		resp = core.AnthropicStreamResponse{
			Type:  core.StreamEventTypeContentBlockStop,
			Index: &index,
		}

	case core.StreamEventTypeMessageStart:
		resp = core.AnthropicStreamResponse{
			Type: core.StreamEventTypeMessageStart,
			Message: &core.AnthropicMessagesResponse{
				ID:   GenerateMessageID(),
				Type: core.AnthropicTypeMessage,
				Role: core.RoleAssistant,
				Usage: core.AnthropicUsage{
					InputTokens:  0,
					OutputTokens: 0,
				},
			},
		}

	case core.StreamEventTypeMessageStop:
		resp = core.AnthropicStreamResponse{
			Type: core.StreamEventTypeMessageStop,
		}

	default:
		resp = core.AnthropicStreamResponse{
			Type: "error",
		}
	}

	data, err := util.MarshalJSON(resp)
	if err != nil {
		return []byte{}
	}
	return data
}

// GenerateMessageID generates an Anthropic message ID
func GenerateMessageID() string {
	return util.GenerateID(core.MessageIDPrefix)
}
