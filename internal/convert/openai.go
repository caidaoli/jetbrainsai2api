package convert

import (
	"sync"

	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/util"
	"jetbrainsai2api/internal/validate"

	"github.com/bytedance/sonic"
)

// MessageConverter handles OpenAI → JetBrains message conversion
type MessageConverter struct {
	toolIDToFuncNameMap map[string]string
	validator           *validate.ImageValidator
	logger              core.Logger
}

var converterPool = sync.Pool{
	New: func() any {
		return &MessageConverter{
			toolIDToFuncNameMap: make(map[string]string, 8),
			validator:           validate.NewImageValidator(),
			logger:              &core.NopLogger{},
		}
	},
}

// OpenAIToJetbrainsMessages converts OpenAI chat messages to JetBrains format
func OpenAIToJetbrainsMessages(messages []core.ChatMessage) []core.JetbrainsMessage {
	converter := converterPool.Get().(*MessageConverter)
	defer func() {
		clear(converter.toolIDToFuncNameMap)
		converterPool.Put(converter)
	}()
	return converter.Convert(messages)
}

// Convert executes message conversion
func (c *MessageConverter) Convert(messages []core.ChatMessage) []core.JetbrainsMessage {
	c.buildToolIDMap(messages)

	var result []core.JetbrainsMessage
	for _, msg := range messages {
		converted := c.convertMessage(msg)
		result = append(result, converted...)
	}
	return result
}

func (c *MessageConverter) buildToolIDMap(messages []core.ChatMessage) {
	for _, msg := range messages {
		if msg.Role == core.RoleAssistant && msg.ToolCalls != nil {
			for _, tc := range msg.ToolCalls {
				if tc.ID != "" && tc.Function.Name != "" {
					c.toolIDToFuncNameMap[tc.ID] = tc.Function.Name
				}
			}
		}
	}
}

func (c *MessageConverter) convertMessage(msg core.ChatMessage) []core.JetbrainsMessage {
	switch msg.Role {
	case core.RoleUser:
		return c.convertUserMessage(msg)
	case core.RoleSystem:
		return c.convertSystemMessage(msg)
	case core.RoleAssistant:
		return c.convertAssistantMessage(msg)
	case core.RoleTool:
		return c.convertToolMessage(msg)
	default:
		return c.convertDefaultMessage(msg)
	}
}

func (c *MessageConverter) convertUserMessage(msg core.ChatMessage) []core.JetbrainsMessage {
	var result []core.JetbrainsMessage

	mediaType, imageData, hasImage := validate.ExtractImageDataFromContent(msg.Content)
	if hasImage {
		result = append(result, c.convertImageContent(mediaType, imageData, msg.Content)...)
		return result
	}

	result = append(result, c.convertTextContent(msg.Content)...)
	return result
}

func (c *MessageConverter) convertImageContent(mediaType, imageData string, content any) []core.JetbrainsMessage {
	var result []core.JetbrainsMessage

	if err := c.validator.ValidateImageData(mediaType, imageData); err != nil {
		c.logger.Warn("Image validation failed: %v", err)
		textContent := util.ExtractTextContent(content)
		result = append(result, core.JetbrainsMessage{
			Type:    core.JetBrainsMessageTypeUser,
			Content: textContent,
		})
		return result
	}

	result = append(result, core.JetbrainsMessage{
		Type:      core.JetBrainsMessageTypeMedia,
		MediaType: mediaType,
		Data:      imageData,
	})

	textContent := util.ExtractTextContent(content)
	if textContent != "" {
		result = append(result, core.JetbrainsMessage{
			Type:    core.JetBrainsMessageTypeUser,
			Content: textContent,
		})
	}

	return result
}

func (c *MessageConverter) convertTextContent(content any) []core.JetbrainsMessage {
	var result []core.JetbrainsMessage

	if contentArray, ok := content.([]any); ok {
		for _, item := range contentArray {
			if itemMap, ok := item.(map[string]any); ok {
				if itemType, ok := itemMap["type"].(string); ok && itemType == core.ContentBlockTypeText {
					if text, ok := itemMap["text"].(string); ok && text != "" {
						result = append(result, core.JetbrainsMessage{
							Type:    core.JetBrainsMessageTypeUser,
							Content: text,
						})
					}
				}
			}
		}
		return result
	}

	textContent := util.ExtractTextContent(content)
	result = append(result, core.JetbrainsMessage{
		Type:    core.JetBrainsMessageTypeUser,
		Content: textContent,
	})
	return result
}

func (c *MessageConverter) convertSystemMessage(msg core.ChatMessage) []core.JetbrainsMessage {
	textContent := util.ExtractTextContent(msg.Content)
	return []core.JetbrainsMessage{{
		Type:    core.JetBrainsMessageTypeSystem,
		Content: textContent,
	}}
}

func (c *MessageConverter) convertAssistantMessage(msg core.ChatMessage) []core.JetbrainsMessage {
	if len(msg.ToolCalls) > 0 {
		var result []core.JetbrainsMessage

		// If both text content and tool calls exist, preserve text first
		textContent := util.ExtractTextContent(msg.Content)
		if textContent != "" {
			result = append(result, core.JetbrainsMessage{
				Type:    core.JetBrainsMessageTypeAssistantText,
				Content: textContent,
			})
		}

		for _, toolCall := range msg.ToolCalls {
			result = append(result, c.convertAssistantToolCall(toolCall)...)
		}
		return result
	}

	textContent := util.ExtractTextContent(msg.Content)
	return []core.JetbrainsMessage{{
		Type:    core.JetBrainsMessageTypeAssistantText,
		Content: textContent,
	}}
}

func (c *MessageConverter) convertAssistantToolCall(toolCall core.ToolCall) []core.JetbrainsMessage {
	args := toolCall.Function.Arguments
	var argsMap map[string]any
	if err := sonic.UnmarshalString(args, &argsMap); err == nil {
		if cleanArgs, err := util.MarshalJSON(argsMap); err == nil {
			args = string(cleanArgs)
		}
	}

	return []core.JetbrainsMessage{{
		Type:     core.JetBrainsMessageTypeAssistantTool,
		ID:       toolCall.ID,
		ToolName: toolCall.Function.Name,
		Content:  args,
	}}
}

func (c *MessageConverter) convertToolMessage(msg core.ChatMessage) []core.JetbrainsMessage {
	functionName := c.toolIDToFuncNameMap[msg.ToolCallID]
	if functionName == "" {
		c.logger.Warn("Cannot find function name for tool_call_id %s, using fallback", msg.ToolCallID)
		functionName = "unknown_tool"
	}

	textContent := util.ExtractTextContent(msg.Content)
	return []core.JetbrainsMessage{{
		Type:     core.JetBrainsMessageTypeTool,
		ID:       msg.ToolCallID,
		ToolName: functionName,
		Result:   textContent,
	}}
}

func (c *MessageConverter) convertDefaultMessage(msg core.ChatMessage) []core.JetbrainsMessage {
	textContent := util.ExtractTextContent(msg.Content)
	return []core.JetbrainsMessage{{
		Type:    core.JetBrainsMessageTypeUser,
		Content: textContent,
	}}
}
