package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// anthropicToJetbrainsMessages 直接将 Anthropic 消息转换为 JetBrains 格式
// KISS: 消除不必要的中间转换层
// 根据 Anthropic 消息角色正确映射到 JetBrains 消息类型
func anthropicToJetbrainsMessages(anthMessages []AnthropicMessage) []JetbrainsMessage {
	var jetbrainsMessages []JetbrainsMessage

	// 第一遍：建立工具 ID 到工具名称的映射
	toolIDToName := make(map[string]string)
	for _, msg := range anthMessages {
		if msg.Role == RoleAssistant && hasToolUse(msg.Content) {
			if contentArray, ok := msg.Content.([]any); ok {
				for _, block := range contentArray {
					if blockMap, ok := block.(map[string]any); ok {
						if blockType, _ := blockMap["type"].(string); blockType == ContentBlockTypeToolUse {
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

	// 第二遍：转换消息
	for _, msg := range anthMessages {
		// 特殊处理：检查是否为包含 tool_result 的混合内容消息
		if msg.Role == RoleUser && hasToolResult(msg.Content) {
			// 提取并分别处理 tool_result 和常规文本内容
			toolMessages, textContent := extractMixedContent(msg.Content, toolIDToName)

			// 添加 tool_message
			jetbrainsMessages = append(jetbrainsMessages, toolMessages...)

			// 如果还有文本内容，添加为 user_message
			if textContent != "" {
				jetbrainsMessages = append(jetbrainsMessages, JetbrainsMessage{
					Type:    JetBrainsMessageTypeUser,
					Content: textContent,
				})
			}
			continue
		}

		// 常规消息处理
		var messageType string
		switch msg.Role {
		case RoleUser:
			messageType = JetBrainsMessageTypeUser
		case RoleAssistant:
			// 检查是否包含工具调用
			if hasToolUse(msg.Content) {
				// 处理多工具调用：为每个 tool_use 生成一条 JetbrainsMessage
				toolInfos := extractAllToolUse(msg.Content)
				for _, toolInfo := range toolInfos {
					jetbrainsMessages = append(jetbrainsMessages, JetbrainsMessage{
						Type:     JetBrainsMessageTypeAssistantTool,
						ID:       toolInfo.ID,
						ToolName: toolInfo.Name,
						Content:  "", // tool_use 不需要 content
					})
				}
				continue // 已处理完毕，跳过后续通用逻辑
			} else {
				messageType = JetBrainsMessageTypeAssistant
			}
		case RoleTool:
			messageType = JetBrainsMessageTypeTool
		default:
			messageType = JetBrainsMessageTypeUser // 默认为用户消息
		}

		jetbrainsMessage := JetbrainsMessage{
			Type:    messageType,
			Content: extractStringContent(msg.Content),
		}

		// 如果是 tool_message，需要添加额外字段
		if messageType == JetBrainsMessageTypeTool {
			// 从内容中提取工具信息
			if toolInfo := extractToolInfo(msg.Content); toolInfo != nil {
				jetbrainsMessage.ID = toolInfo.ID
				jetbrainsMessage.ToolName = toolInfo.Name
				jetbrainsMessage.Result = toolInfo.Result
				// tool_message 不需要 content 字段，只需要 result
				jetbrainsMessage.Content = ""
			}
		}

		jetbrainsMessages = append(jetbrainsMessages, jetbrainsMessage)
	}

	return jetbrainsMessages
}

// anthropicToJetbrainsTools 直接转换工具定义
func anthropicToJetbrainsTools(anthTools []AnthropicTool) []JetbrainsToolDefinition {
	var jetbrainsTools []JetbrainsToolDefinition

	for _, tool := range anthTools {
		jetbrainsTools = append(jetbrainsTools, JetbrainsToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters: JetbrainsToolParametersWrapper{
				Schema: tool.InputSchema,
			},
		})
	}

	return jetbrainsTools
}

// callJetbrainsAPIDirect 直接调用 JetBrains API（Server 方法）
// KISS: 简化调用链，消除中间转换
func (s *Server) callJetbrainsAPIDirect(anthReq *AnthropicMessagesRequest, jetbrainsMessages []JetbrainsMessage, data []JetbrainsData, account *JetbrainsAccount, startTime time.Time, accountIdentifier string) (*http.Response, int, error) {
	internalModel := getInternalModelName(s.modelsConfig, anthReq.Model)
	payload := JetbrainsPayload{
		Prompt:  JetBrainsChatPrompt,
		Profile: internalModel,
		Chat:    JetbrainsChat{Messages: jetbrainsMessages},
	}

	// 只有当有数据时才设置 Parameters
	if len(data) > 0 {
		payload.Parameters = &JetbrainsParameters{Data: data}
	}

	payloadBytes, err := marshalJSON(payload)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to marshal request")
	}

	Debug("=== JetBrains API Request Debug (Direct) ===")
	Debug("Model: %s -> %s", anthReq.Model, internalModel)
	Debug("Messages converted: %d", len(jetbrainsMessages))
	Debug("Tools attached: %d", len(data))
	Debug("Payload size: %d bytes", len(payloadBytes))
	Debug("=== Complete Upstream Payload ===")
	Debug("%s", string(payloadBytes))
	Debug("=== End Upstream Payload ===")
	Debug("=== End Debug ===")

	req, err := http.NewRequest(http.MethodPost, JetBrainsChatEndpoint, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to create request")
	}

	req.Header.Set(HeaderAccept, ContentTypeEventStream)
	req.Header.Set(HeaderContentType, ContentTypeJSON)
	req.Header.Set(HeaderCacheControl, CacheControlNoCache)
	setJetbrainsHeaders(req, account.JWT)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to make request")
	}

	Debug("JetBrains API Response Status: %d", resp.StatusCode)

	if resp.StatusCode == JetBrainsStatusQuotaExhausted {
		Warn("Account %s has no quota (received 477)", getTokenDisplayName(account))
		account.HasQuota = false
		account.LastQuotaCheck = float64(time.Now().Unix())
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBodySize))
		resp.Body.Close() // 关闭原始 body，避免资源泄漏
		errorMsg := string(body)
		Error("JetBrains API Error: Status %d, Body: %s", resp.StatusCode, errorMsg)

		// 返回 nil response，因为调用者在错误情况下不会使用响应
		return nil, resp.StatusCode, fmt.Errorf("JetBrains API error: %d - %s", resp.StatusCode, errorMsg)
	}

	return resp, http.StatusOK, nil
}

// extractStringContent 提取字符串内容 (KISS: 简单实用)
func extractStringContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		// 处理多块内容，提取文本
		var textParts []string
		for _, block := range v {
			if blockMap, ok := block.(map[string]any); ok {
				if blockType, _ := blockMap["type"].(string); blockType == ContentBlockTypeText {
					if text, _ := blockMap["text"].(string); text != "" {
						textParts = append(textParts, text)
					}
				} else if blockType == ContentBlockTypeToolUse {
					// 对于工具使用，返回JSON格式的参数
					if input, ok := blockMap["input"]; ok {
						if inputJSON, err := marshalJSON(input); err == nil {
							return string(inputJSON)
						}
					}
				}
			}
		}
		if len(textParts) > 0 {
			return textParts[0] // 简化：只取第一段文本
		}
	}
	return fmt.Sprintf("%v", content)
}

// hasToolUse 检查消息内容是否包含工具调用
func hasToolUse(content any) bool {
	if contentArray, ok := content.([]any); ok {
		for _, block := range contentArray {
			if blockMap, ok := block.(map[string]any); ok {
				if blockType, _ := blockMap["type"].(string); blockType == ContentBlockTypeToolUse {
					return true
				}
			}
		}
	}
	return false
}

// hasToolResult 检查消息内容是否包含工具结果
func hasToolResult(content any) bool {
	if contentArray, ok := content.([]any); ok {
		for _, block := range contentArray {
			if blockMap, ok := block.(map[string]any); ok {
				if blockType, _ := blockMap["type"].(string); blockType == ContentBlockTypeToolResult {
					return true
				}
			}
		}
	}
	return false
}

// extractMixedContent 从混合内容中分别提取工具结果和文本内容
func extractMixedContent(content any, toolIDToName map[string]string) ([]JetbrainsMessage, string) {
	var toolMessages []JetbrainsMessage
	var textParts []string

	if contentArray, ok := content.([]any); ok {
		for _, block := range contentArray {
			if blockMap, ok := block.(map[string]any); ok {
				blockType, _ := blockMap["type"].(string)

				if blockType == ContentBlockTypeToolResult {
					// 创建 tool_message
					toolMsg := JetbrainsMessage{
						Type:    JetBrainsMessageTypeTool,
						Content: "",
					}

					// 提取工具 ID
					if toolUseID, ok := blockMap["tool_use_id"].(string); ok {
						toolMsg.ID = toolUseID
						// 从映射中获取工具名称
						if toolName, exists := toolIDToName[toolUseID]; exists {
							toolMsg.ToolName = toolName
						} else {
							toolMsg.ToolName = "Unknown"
						}
					}

					// 提取工具结果
					if result, ok := blockMap["content"]; ok {
						if resultStr, ok := result.(string); ok {
							toolMsg.Result = resultStr
						} else if resultArray, ok := result.([]any); ok {
							// 处理数组形式的 content
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

				} else if blockType == ContentBlockTypeText {
					// 提取文本内容
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

// ToolInfo 工具信息结构
type ToolInfo struct {
	ID     string
	Name   string
	Result string
}

// extractToolInfo 从消息内容中提取工具信息
func extractToolInfo(content any) *ToolInfo {
	if contentArray, ok := content.([]any); ok {
		for _, block := range contentArray {
			if blockMap, ok := block.(map[string]any); ok {
				blockType, _ := blockMap["type"].(string)

				if blockType == ContentBlockTypeToolUse {
					toolInfo := &ToolInfo{}
					if id, ok := blockMap["id"].(string); ok {
						toolInfo.ID = id
					}
					if name, ok := blockMap["name"].(string); ok {
						toolInfo.Name = name
					}
					return toolInfo
				} else if blockType == ContentBlockTypeToolResult {
					toolInfo := &ToolInfo{}
					if id, ok := blockMap["tool_use_id"].(string); ok {
						toolInfo.ID = id
					}
					// 从 tool_result 的 content 中提取结果
					if result, ok := blockMap["content"]; ok {
						if resultStr, ok := result.(string); ok {
							toolInfo.Result = resultStr
						} else if resultArray, ok := result.([]any); ok {
							// 处理数组形式的 content
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

// extractAllToolUse 从消息内容中提取所有 tool_use blocks
// 用于处理包含多个工具调用的 assistant 消息
func extractAllToolUse(content any) []ToolInfo {
	var toolInfos []ToolInfo
	if contentArray, ok := content.([]any); ok {
		for _, block := range contentArray {
			if blockMap, ok := block.(map[string]any); ok {
				if blockType, _ := blockMap["type"].(string); blockType == ContentBlockTypeToolUse {
					toolInfo := ToolInfo{}
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
