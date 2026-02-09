package convert

import (
	"fmt"
	"strings"

	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/util"
)

// AnthropicToJetbrainsMessages converts Anthropic messages to JetBrains format
func AnthropicToJetbrainsMessages(anthMessages []core.AnthropicMessage) []core.JetbrainsMessage {
	var jetbrainsMessages []core.JetbrainsMessage

	// First pass: build tool ID to name mapping
	toolIDToName := make(map[string]string)
	for _, msg := range anthMessages {
		if msg.Role == core.RoleAssistant && HasContentBlockType(msg.Content, core.ContentBlockTypeToolUse) {
			if contentArray, ok := msg.Content.([]any); ok {
				for _, block := range contentArray {
					if blockMap, ok := block.(map[string]any); ok {
						if blockType, _ := blockMap["type"].(string); blockType == core.ContentBlockTypeToolUse {
							if id, ok := blockMap["id"].(string); ok {
								if name, ok := blockMap["name"].(string); ok {
									toolIDToName[id] = name
								}
							}
						}
					}
				}
			}
		}
	}

	// Second pass: convert messages
	for _, msg := range anthMessages {
		if msg.Role == core.RoleUser && HasContentBlockType(msg.Content, core.ContentBlockTypeToolResult) {
			toolMessages, textContent := extractMixedContent(msg.Content, toolIDToName)
			jetbrainsMessages = append(jetbrainsMessages, toolMessages...)
			if textContent != "" {
				jetbrainsMessages = append(jetbrainsMessages, core.JetbrainsMessage{
					Type:    core.JetBrainsMessageTypeUser,
					Content: textContent,
				})
			}
			continue
		}

		var messageType string
		switch msg.Role {
		case core.RoleUser:
			messageType = core.JetBrainsMessageTypeUser
		case core.RoleAssistant:
			if HasContentBlockType(msg.Content, core.ContentBlockTypeToolUse) {
				toolInfos := ExtractAllToolUse(msg.Content)
				for _, toolInfo := range toolInfos {
					jetbrainsMessages = append(jetbrainsMessages, core.JetbrainsMessage{
						Type:     core.JetBrainsMessageTypeAssistantTool,
						ID:       toolInfo.ID,
						ToolName: toolInfo.Name,
						Content:  "",
					})
				}
				continue
			} else {
				messageType = core.JetBrainsMessageTypeAssistant
			}
		case core.RoleTool:
			messageType = core.JetBrainsMessageTypeTool
		default:
			messageType = core.JetBrainsMessageTypeUser
		}

		jetbrainsMessage := core.JetbrainsMessage{
			Type:    messageType,
			Content: ExtractStringContent(msg.Content),
		}

		if messageType == core.JetBrainsMessageTypeTool {
			if toolInfo := ExtractToolInfo(msg.Content); toolInfo != nil {
				jetbrainsMessage.ID = toolInfo.ID
				jetbrainsMessage.ToolName = toolInfo.Name
				jetbrainsMessage.Result = toolInfo.Result
				jetbrainsMessage.Content = ""
			}
		}

		jetbrainsMessages = append(jetbrainsMessages, jetbrainsMessage)
	}

	return jetbrainsMessages
}

// AnthropicToJetbrainsTools converts Anthropic tool definitions to JetBrains format
func AnthropicToJetbrainsTools(anthTools []core.AnthropicTool) []core.JetbrainsToolDefinition {
	var jetbrainsTools []core.JetbrainsToolDefinition

	for _, tool := range anthTools {
		jetbrainsTools = append(jetbrainsTools, core.JetbrainsToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters: core.JetbrainsToolParametersWrapper{
				Schema: tool.InputSchema,
			},
		})
	}

	return jetbrainsTools
}

// ExtractStringContent extracts string content from message
func ExtractStringContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var textParts []string
		for _, block := range v {
			if blockMap, ok := block.(map[string]any); ok {
				if blockType, _ := blockMap["type"].(string); blockType == core.ContentBlockTypeText {
					if text, _ := blockMap["text"].(string); text != "" {
						textParts = append(textParts, text)
					}
				} else if blockType == core.ContentBlockTypeToolUse {
					if input, ok := blockMap["input"]; ok {
						if inputJSON, err := util.MarshalJSON(input); err == nil {
							return string(inputJSON)
						}
					}
				}
			}
		}
		if len(textParts) > 0 {
			return textParts[0]
		}
	}
	return fmt.Sprintf("%v", content)
}

// HasContentBlockType checks if content has a specific block type
func HasContentBlockType(content any, targetType string) bool {
	if contentArray, ok := content.([]any); ok {
		for _, block := range contentArray {
			if blockMap, ok := block.(map[string]any); ok {
				if blockType, _ := blockMap["type"].(string); blockType == targetType {
					return true
				}
			}
		}
	}
	return false
}

func extractMixedContent(content any, toolIDToName map[string]string) ([]core.JetbrainsMessage, string) {
	var toolMessages []core.JetbrainsMessage
	var textParts []string

	if contentArray, ok := content.([]any); ok {
		for _, block := range contentArray {
			if blockMap, ok := block.(map[string]any); ok {
				blockType, _ := blockMap["type"].(string)

				//nolint:staticcheck // QF1003: if-else is more readable here
				if blockType == core.ContentBlockTypeToolResult {
					toolMsg := core.JetbrainsMessage{
						Type:    core.JetBrainsMessageTypeTool,
						Content: "",
					}

					if toolUseID, ok := blockMap["tool_use_id"].(string); ok {
						toolMsg.ID = toolUseID
						if toolName, exists := toolIDToName[toolUseID]; exists {
							toolMsg.ToolName = toolName
						} else {
							toolMsg.ToolName = "Unknown"
						}
					}

					if result, ok := blockMap["content"]; ok {
						if resultStr, ok := result.(string); ok {
							toolMsg.Result = resultStr
						} else if resultArray, ok := result.([]any); ok {
							var resultParts []string
							for _, part := range resultArray {
								if partMap, ok := part.(map[string]any); ok {
									if text, ok := partMap["text"].(string); ok {
										resultParts = append(resultParts, text)
									}
								}
							}
							if len(resultParts) > 0 {
								toolMsg.Result = strings.Join(resultParts, "")
							}
						} else {
							toolMsg.Result = fmt.Sprintf("%v", result)
						}
					}

					toolMessages = append(toolMessages, toolMsg)

				} else if blockType == core.ContentBlockTypeText {
					if text, ok := blockMap["text"].(string); ok && text != "" {
						textParts = append(textParts, text)
					}
				}
			}
		}
	}

	textContent := strings.Join(textParts, " ")
	return toolMessages, textContent
}

// ExtractToolInfo extracts tool info from message content
func ExtractToolInfo(content any) *core.ToolInfo {
	if contentArray, ok := content.([]any); ok {
		for _, block := range contentArray {
			if blockMap, ok := block.(map[string]any); ok {
				blockType, _ := blockMap["type"].(string)

				//nolint:staticcheck // QF1003: if-else is more readable here
				if blockType == core.ContentBlockTypeToolUse {
					toolInfo := &core.ToolInfo{}
					if id, ok := blockMap["id"].(string); ok {
						toolInfo.ID = id
					}
					if name, ok := blockMap["name"].(string); ok {
						toolInfo.Name = name
					}
					return toolInfo
				} else if blockType == core.ContentBlockTypeToolResult {
					toolInfo := &core.ToolInfo{}
					if id, ok := blockMap["tool_use_id"].(string); ok {
						toolInfo.ID = id
					}
					if result, ok := blockMap["content"]; ok {
						if resultStr, ok := result.(string); ok {
							toolInfo.Result = resultStr
						} else if resultArray, ok := result.([]any); ok {
							var resultParts []string
							for _, part := range resultArray {
								if partMap, ok := part.(map[string]any); ok {
									if text, ok := partMap["text"].(string); ok {
										resultParts = append(resultParts, text)
									}
								}
							}
							if len(resultParts) > 0 {
								toolInfo.Result = strings.Join(resultParts, "")
							}
						} else {
							toolInfo.Result = fmt.Sprintf("%v", result)
						}
					}
					return toolInfo
				}
			}
		}
	}
	return nil
}

// ExtractAllToolUse extracts all tool_use blocks from message content
func ExtractAllToolUse(content any) []core.ToolInfo {
	var toolInfos []core.ToolInfo
	if contentArray, ok := content.([]any); ok {
		for _, block := range contentArray {
			if blockMap, ok := block.(map[string]any); ok {
				if blockType, _ := blockMap["type"].(string); blockType == core.ContentBlockTypeToolUse {
					toolInfo := core.ToolInfo{}
					if id, ok := blockMap["id"].(string); ok {
						toolInfo.ID = id
					}
					if name, ok := blockMap["name"].(string); ok {
						toolInfo.Name = name
					}
					toolInfos = append(toolInfos, toolInfo)
				}
			}
		}
	}
	return toolInfos
}
