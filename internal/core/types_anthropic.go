package core

import (
	"fmt"
	"strings"

	"github.com/bytedance/sonic"
)

// AnthropicMessage represents a message in the Anthropic Messages API format.
type AnthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// AnthropicContentBlock represents a content block in an Anthropic message.
type AnthropicContentBlock struct {
	Type  string         `json:"type"`
	Text  string         `json:"text,omitempty"`
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`
}

// FlexibleString supports string or array form of system field.
type FlexibleString string

// UnmarshalJSON custom JSON parsing, supports string and array formats.
func (fs *FlexibleString) UnmarshalJSON(data []byte) error {
	var str string
	if err := sonic.Unmarshal(data, &str); err == nil {
		*fs = FlexibleString(str)
		return nil
	}

	var arr []map[string]any
	if err := sonic.Unmarshal(data, &arr); err == nil {
		var parts []string
		for _, item := range arr {
			if textVal, ok := item["text"]; ok {
				if text, ok := textVal.(string); ok {
					parts = append(parts, text)
				}
			} else if typeVal, ok := item["type"]; ok && typeVal == ContentBlockTypeText {
				if textVal, ok := item["content"]; ok {
					if text, ok := textVal.(string); ok {
						parts = append(parts, text)
					}
				}
			}
		}
		*fs = FlexibleString(strings.Join(parts, ""))
		return nil
	}

	return fmt.Errorf("invalid system field format")
}

// AnthropicTool represents a tool definition in the Anthropic Messages API.
type AnthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

// AnthropicMessagesRequest is the Anthropic Messages API request payload.
type AnthropicMessagesRequest struct {
	Model         string             `json:"model"`
	MaxTokens     int                `json:"max_tokens"`
	Messages      []AnthropicMessage `json:"messages"`
	System        FlexibleString     `json:"system,omitempty"`
	Temperature   *float64           `json:"temperature,omitempty"`
	TopP          *float64           `json:"top_p,omitempty"`
	TopK          *int               `json:"top_k,omitempty"`
	Stream        *bool              `json:"stream,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
	Tools         []AnthropicTool    `json:"tools,omitempty"`
	ToolChoice    any                `json:"tool_choice,omitempty"`
}

// AnthropicUsage holds token usage information for Anthropic API responses.
type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// AnthropicMessagesResponse is the Anthropic Messages API non-streaming response.
type AnthropicMessagesResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"`
	Role         string                  `json:"role"`
	Content      []AnthropicContentBlock `json:"content"`
	Model        string                  `json:"model"`
	StopReason   string                  `json:"stop_reason,omitempty"`
	StopSequence *string                 `json:"stop_sequence,omitempty"`
	Usage        AnthropicUsage          `json:"usage"`
}

// AnthropicStreamResponse is the Anthropic Messages API streaming response event.
type AnthropicStreamResponse struct {
	Type         string                 `json:"type"`
	Index        *int                   `json:"index,omitempty"`
	ContentBlock *AnthropicContentBlock `json:"content_block,omitempty"`
	Delta        *struct {
		Type string `json:"type,omitempty"`
		Text string `json:"text,omitempty"`
	} `json:"delta,omitempty"`
	Message *AnthropicMessagesResponse `json:"message,omitempty"`
	Usage   *AnthropicUsage            `json:"usage,omitempty"`
}
