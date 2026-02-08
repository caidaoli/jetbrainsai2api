package main

import (
	"sync"

	"github.com/bytedance/sonic"
)

// ============================================================================
// OpenAI → JetBrains 消息转换器
// SRP: 每个转换函数只负责一种消息类型的转换
// ============================================================================

// MessageConverter 消息转换器，封装转换过程中需要的状态
type MessageConverter struct {
	toolIDToFuncNameMap map[string]string
	validator           *ImageValidator
	logger              Logger
}

// converterPool 对象池，复用 MessageConverter 减少内存分配
var converterPool = sync.Pool{
	New: func() any {
		return &MessageConverter{
			toolIDToFuncNameMap: make(map[string]string, 8), // 预分配容量
			validator:           NewImageValidator(),
			logger:              &NopLogger{},
		}
	},
}

// openAIToJetbrainsMessages converts OpenAI chat messages to JetBrains format
// 使用对象池复用 MessageConverter，减少内存分配
func openAIToJetbrainsMessages(messages []ChatMessage) []JetbrainsMessage {
	converter := converterPool.Get().(*MessageConverter)
	defer func() {
		// 清理 map 但保留容量
		clear(converter.toolIDToFuncNameMap)
		converterPool.Put(converter)
	}()
	return converter.Convert(messages)
}

// Convert 执行消息转换
func (c *MessageConverter) Convert(messages []ChatMessage) []JetbrainsMessage {
	// 第一遍：构建 tool ID 到函数名的映射
	c.buildToolIDMap(messages)

	// 第二遍：转换每条消息
	var result []JetbrainsMessage
	for _, msg := range messages {
		converted := c.convertMessage(msg)
		result = append(result, converted...)
	}
	return result
}

// buildToolIDMap 构建 tool ID 到函数名的映射，用于 tool result 消息的处理
func (c *MessageConverter) buildToolIDMap(messages []ChatMessage) {
	for _, msg := range messages {
		if msg.Role == RoleAssistant && msg.ToolCalls != nil {
			for _, tc := range msg.ToolCalls {
				if tc.ID != "" && tc.Function.Name != "" {
					c.toolIDToFuncNameMap[tc.ID] = tc.Function.Name
				}
			}
		}
	}
}

// convertMessage 根据消息角色分发到对应的转换函数
func (c *MessageConverter) convertMessage(msg ChatMessage) []JetbrainsMessage {
	switch msg.Role {
	case RoleUser:
		return c.convertUserMessage(msg)
	case RoleSystem:
		return c.convertSystemMessage(msg)
	case RoleAssistant:
		return c.convertAssistantMessage(msg)
	case RoleTool:
		return c.convertToolMessage(msg)
	default:
		return c.convertDefaultMessage(msg)
	}
}

// convertUserMessage 转换用户消息
// 处理文本内容和图像内容（多模态支持）
func (c *MessageConverter) convertUserMessage(msg ChatMessage) []JetbrainsMessage {
	var result []JetbrainsMessage

	// 检查是否包含图像内容
	mediaType, imageData, hasImage := ExtractImageDataFromContent(msg.Content)
	if hasImage {
		result = append(result, c.convertImageContent(mediaType, imageData, msg.Content)...)
		return result
	}

	// 处理纯文本内容
	result = append(result, c.convertTextContent(msg.Content)...)
	return result
}

// convertImageContent 转换包含图像的消息内容
func (c *MessageConverter) convertImageContent(mediaType, imageData string, content any) []JetbrainsMessage {
	var result []JetbrainsMessage

	// 验证图像
	if err := c.validator.ValidateImageData(mediaType, imageData); err != nil {
		c.logger.Warn("Image validation failed: %v", err)
		// 图像验证失败，仅使用文本内容
		textContent := extractTextContent(content)
		result = append(result, JetbrainsMessage{
			Type:    JetBrainsMessageTypeUser,
			Content: textContent,
		})
		return result
	}

	// 添加图像消息
	result = append(result, JetbrainsMessage{
		Type:      JetBrainsMessageTypeMedia,
		MediaType: mediaType,
		Data:      imageData,
	})

	// 如果还有文本内容，也添加进去
	textContent := extractTextContent(content)
	if textContent != "" {
		result = append(result, JetbrainsMessage{
			Type:    JetBrainsMessageTypeUser,
			Content: textContent,
		})
	}

	return result
}

// convertTextContent 转换纯文本消息内容
// 支持单个文本字符串和多个文本块数组
func (c *MessageConverter) convertTextContent(content any) []JetbrainsMessage {
	var result []JetbrainsMessage

	// 检查是否是内容块数组
	if contentArray, ok := content.([]any); ok {
		for _, item := range contentArray {
			if itemMap, ok := item.(map[string]any); ok {
				if itemType, ok := itemMap["type"].(string); ok && itemType == ContentBlockTypeText {
					if text, ok := itemMap["text"].(string); ok && text != "" {
						result = append(result, JetbrainsMessage{
							Type:    JetBrainsMessageTypeUser,
							Content: text,
						})
					}
				}
			}
		}
		return result
	}

	// 单个文本内容
	textContent := extractTextContent(content)
	result = append(result, JetbrainsMessage{
		Type:    JetBrainsMessageTypeUser,
		Content: textContent,
	})
	return result
}

// convertSystemMessage 转换系统消息
func (c *MessageConverter) convertSystemMessage(msg ChatMessage) []JetbrainsMessage {
	textContent := extractTextContent(msg.Content)
	return []JetbrainsMessage{{
		Type:    JetBrainsMessageTypeSystem,
		Content: textContent,
	}}
}

// convertAssistantMessage 转换助手消息
// 处理文本响应和工具调用（支持多工具调用）
func (c *MessageConverter) convertAssistantMessage(msg ChatMessage) []JetbrainsMessage {
	if len(msg.ToolCalls) > 0 {
		// 遍历所有 tool_calls，为每个生成一条 JetbrainsMessage
		var result []JetbrainsMessage
		for _, toolCall := range msg.ToolCalls {
			result = append(result, c.convertAssistantToolCall(toolCall)...)
		}
		return result
	}

	// 文本响应
	textContent := extractTextContent(msg.Content)
	return []JetbrainsMessage{{
		Type:    JetBrainsMessageTypeAssistantText,
		Content: textContent,
	}}
}

// convertAssistantToolCall 转换助手的工具调用消息
func (c *MessageConverter) convertAssistantToolCall(toolCall ToolCall) []JetbrainsMessage {
	// 尝试解析并规范化参数 JSON
	args := toolCall.Function.Arguments
	var argsMap map[string]any
	if err := sonic.UnmarshalString(args, &argsMap); err == nil {
		// 重新编码以确保格式一致
		if cleanArgs, err := marshalJSON(argsMap); err == nil {
			args = string(cleanArgs)
		}
	}

	return []JetbrainsMessage{{
		Type:     JetBrainsMessageTypeAssistantTool,
		ID:       toolCall.ID,
		ToolName: toolCall.Function.Name,
		Content:  args,
	}}
}

// convertToolMessage 转换工具结果消息
func (c *MessageConverter) convertToolMessage(msg ChatMessage) []JetbrainsMessage {
	functionName := c.toolIDToFuncNameMap[msg.ToolCallID]
	if functionName == "" {
		c.logger.Warn("Cannot find function name for tool_call_id %s", msg.ToolCallID)
		return nil
	}

	textContent := extractTextContent(msg.Content)
	return []JetbrainsMessage{{
		Type:     JetBrainsMessageTypeTool,
		ID:       msg.ToolCallID,
		ToolName: functionName,
		Result:   textContent,
	}}
}

// convertDefaultMessage 转换未知角色的消息（回退为用户消息）
func (c *MessageConverter) convertDefaultMessage(msg ChatMessage) []JetbrainsMessage {
	textContent := extractTextContent(msg.Content)
	return []JetbrainsMessage{{
		Type:    JetBrainsMessageTypeUser,
		Content: textContent,
	}}
}
