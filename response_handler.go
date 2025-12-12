package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bytedance/sonic"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// generateShortToolCallID generates a tool call ID in Anthropic format (toolu_xxx)
func generateShortToolCallID() string {
	// Generate 10 random bytes and encode as hex (20 chars) + "toolu_" prefix (6 chars) = 26 chars total
	// Anthropic format: toolu_01G4sznjWs4orN79KqRAsQ5E (typically 22-26 chars)
	bytes := make([]byte, 10)
	rand.Read(bytes)
	return fmt.Sprintf("%s%s", ToolCallIDPrefix, hex.EncodeToString(bytes))
}

// processJetbrainsStream processes the event stream from the JetBrains API.
// It calls the provided onEvent function for each event in the stream.
// Returns error if stream reading fails or context is cancelled.
func processJetbrainsStream(ctx context.Context, resp *http.Response, onEvent func(event map[string]any) bool) error {
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		// 检查 context 是否已取消（客户端断开连接）
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()

		if !strings.HasPrefix(line, StreamChunkPrefix) || line == StreamEndLine {
			continue
		}

		dataStr := line[6:]
		var data map[string]any
		if err := sonic.Unmarshal([]byte(dataStr), &data); err != nil {
			Error("Error unmarshalling stream event: %v", err)
			continue
		}

		if !onEvent(data) {
			break
		}
	}

	// 检查 scanner 是否遇到错误
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stream read error: %w", err)
	}

	return nil
}

// handleStreamingResponseWithMetrics handles streaming responses (with injected MetricsService)
func handleStreamingResponseWithMetrics(c *gin.Context, resp *http.Response, request ChatCompletionRequest, startTime time.Time, accountIdentifier string, metrics *MetricsService) {
	setStreamingHeaders(c, APIFormatOpenAI)

	streamID := ResponseIDPrefix + uuid.New().String()
	firstChunkSent := false
	var currentTool *map[string]any

	// 使用请求的 context 来检测客户端断开
	ctx := c.Request.Context()

	err := processJetbrainsStream(ctx, resp, func(data map[string]any) bool {
		eventType, _ := data["type"].(string)

		switch eventType {
		case JetBrainsEventTypeContent:
			content, _ := data["content"].(string)
			if content == "" {
				return true // Continue processing
			}

			var deltaPayload map[string]any
			if !firstChunkSent {
				deltaPayload = map[string]any{
					"role":    RoleAssistant,
					"content": content,
				}
				firstChunkSent = true
			} else {
				deltaPayload = map[string]any{
					"content": content,
				}
			}

			streamResp := StreamResponse{
				ID:      streamID,
				Object:  ChatCompletionChunkObjectType,
				Created: time.Now().Unix(),
				Model:   request.Model,
				Choices: []StreamChoice{{Delta: deltaPayload}},
			}

			respJSON, _ := marshalJSON(streamResp)
			writeSSEData(c.Writer, respJSON)
			c.Writer.Flush()
		case JetBrainsEventTypeToolCall:
			// 处理新的ToolCall格式 - 使用上游提供的ID
			if upstreamID, ok := data["id"].(string); ok && upstreamID != "" {
				// 开始新的工具调用 - 使用上游提供的ID
				if name, ok := data["name"].(string); ok && name != "" {
					currentTool = &map[string]any{
						"index": 0,
						"id":    upstreamID, // 使用上游提供的ID
						"function": map[string]any{
							"arguments": "",
							"name":      name,
						},
						"type": ToolTypeFunction,
					}
					Debug("Started new tool call with upstream ID: %s, name: %s", upstreamID, name)
				}
			} else if currentTool != nil {
				// 累积参数内容 (当ID为null时)
				if content, ok := data["content"].(string); ok {
					if funcMap, ok := (*currentTool)["function"].(map[string]any); ok {
						currentArgs, _ := funcMap["arguments"].(string)
						funcMap["arguments"] = currentArgs + content
					}
				}
			}
		case JetBrainsEventTypeFunctionCall:
			funcNameInterface := data["name"]
			funcArgs, _ := data["content"].(string)

			var funcName string
			if funcNameInterface == nil {
				funcName = ""
			} else {
				funcName, _ = funcNameInterface.(string)
			}

			if funcName != "" {
				currentTool = &map[string]any{
					"index": 0,
					"id":    generateShortToolCallID(),
					"function": map[string]any{
						"arguments": "",
						"name":      funcName,
					},
					"type": ToolTypeFunction,
				}
			} else if currentTool != nil {
				if funcMap, ok := (*currentTool)["function"].(map[string]any); ok {
					currentArgs, _ := funcMap["arguments"].(string)
					funcMap["arguments"] = currentArgs + funcArgs
				}
			}
		case JetBrainsEventTypeFinishMetadata:
			if currentTool != nil {
				// Validate the tool call arguments before sending
				if funcMap, ok := (*currentTool)["function"].(map[string]any); ok {
					if args, ok := funcMap["arguments"].(string); ok && args != "" {
						// Try to validate JSON format
						var argsTest map[string]any
						if err := sonic.Unmarshal([]byte(args), &argsTest); err != nil {
							Warn("Tool call arguments are not valid JSON: %v", err)
						}
					}
				}

				deltaPayload := map[string]any{
					"tool_calls": []map[string]any{*currentTool},
				}
				streamResp := StreamResponse{
					ID:      streamID,
					Object:  ChatCompletionChunkObjectType,
					Created: time.Now().Unix(),
					Model:   request.Model,
					Choices: []StreamChoice{{Delta: deltaPayload}},
				}
				respJSON, _ := marshalJSON(streamResp)
				writeSSEData(c.Writer, respJSON)
				c.Writer.Flush()
			}

			finalResp := StreamResponse{
				ID:      streamID,
				Object:  ChatCompletionChunkObjectType,
				Created: time.Now().Unix(),
				Model:   request.Model,
				Choices: []StreamChoice{{Delta: map[string]any{}, FinishReason: stringPtr(FinishReasonToolCalls)}},
			}

			respJSON, _ := marshalJSON(finalResp)
			writeSSEData(c.Writer, respJSON)
			writeSSEDone(c.Writer)
			c.Writer.Flush()
			return false // Stop processing
		}
		return true // Continue processing
	})

	// 处理流处理过程中的错误
	if err != nil {
		if ctx.Err() != nil {
			// 客户端断开连接，记录但不报错
			Debug("Client disconnected during streaming: %v", err)
		} else {
			// 其他流处理错误
			Error("Stream processing error: %v", err)
		}
	}

	metrics.RecordRequest(true, time.Since(startTime).Milliseconds(), request.Model, accountIdentifier)
}

// handleNonStreamingResponseWithMetrics handles non-streaming responses (with injected MetricsService)
func handleNonStreamingResponseWithMetrics(c *gin.Context, resp *http.Response, request ChatCompletionRequest, startTime time.Time, accountIdentifier string, metrics *MetricsService) {
	var contentBuilder strings.Builder
	var toolCalls []ToolCall
	var currentFuncName string
	var currentFuncArgs string

	// 使用请求的 context 来检测客户端断开
	ctx := c.Request.Context()

	err := processJetbrainsStream(ctx, resp, func(data map[string]any) bool {
		eventType, _ := data["type"].(string)

		switch eventType {
		case JetBrainsEventTypeContent:
			if content, ok := data["content"].(string); ok {
				contentBuilder.WriteString(content)
			}
		case JetBrainsEventTypeToolCall:
			// 处理新的ToolCall格式 - 使用上游提供的ID
			if upstreamID, ok := data["id"].(string); ok && upstreamID != "" {
				// 开始新的工具调用 - 记录上游ID
				if name, ok := data["name"].(string); ok && name != "" {
					currentFuncName = name
					currentFuncArgs = ""
					// 存储上游ID供后续使用
					if len(toolCalls) == 0 {
						toolCalls = append(toolCalls, ToolCall{
							ID:   upstreamID, // 使用上游提供的ID
							Type: ToolTypeFunction,
							Function: Function{
								Name:      name,
								Arguments: "",
							},
						})
					}
					Debug("Started new tool call with upstream ID: %s, name: %s", upstreamID, name)
				}
			} else if content, ok := data["content"].(string); ok {
				// 累积参数内容 (当ID为null时)
				currentFuncArgs += content
				// 更新toolCalls中的参数
				if len(toolCalls) > 0 {
					toolCalls[len(toolCalls)-1].Function.Arguments += content
				}
			}
		case JetBrainsEventTypeFunctionCall:
			funcNameInterface := data["name"]
			funcArgs, _ := data["content"].(string)

			var funcName string
			if funcNameInterface == nil {
				funcName = ""
			} else {
				funcName, _ = funcNameInterface.(string)
			}

			if funcName != "" {
				currentFuncName = funcName
				currentFuncArgs = ""
			}
			currentFuncArgs += funcArgs
		case JetBrainsEventTypeFinishMetadata:
			// 完成工具调用参数收集 - toolCalls已在ToolCall事件中创建
			if len(toolCalls) > 0 {
				// 验证最后一个工具调用
				lastToolCall := &toolCalls[len(toolCalls)-1]
				if err := validateToolCallResponse(*lastToolCall); err != nil {
					Warn("Invalid tool call response: %v", err)
				}
				Debug("Completed tool call with ID: %s, args: %s", lastToolCall.ID, lastToolCall.Function.Arguments)
			} else if currentFuncName != "" {
				// 后备方案：如果没有通过ToolCall事件创建，则创建一个
				toolCall := ToolCall{
					ID:   generateShortToolCallID(), // 后备方案
					Type: ToolTypeFunction,
					Function: Function{
						Name:      currentFuncName,
						Arguments: currentFuncArgs,
					},
				}
				toolCalls = append(toolCalls, toolCall)
				Warn("Used fallback tool ID generation for: %s", currentFuncName)
			}
			return false // Stop processing
		}
		return true // Continue processing
	})

	// 处理流处理过程中的错误
	if err != nil {
		if ctx.Err() != nil {
			Debug("Client disconnected during non-streaming response: %v", err)
		} else {
			Error("Stream processing error in non-streaming handler: %v", err)
		}
	}

	message := ChatMessage{
		Role:    RoleAssistant,
		Content: contentBuilder.String(),
	}

	finishReason := FinishReasonStop
	if len(toolCalls) > 0 {
		message.ToolCalls = toolCalls
		finishReason = FinishReasonToolCalls
	}

	response := ChatCompletionResponse{
		ID:      ResponseIDPrefix + uuid.New().String(),
		Object:  ChatCompletionObjectType,
		Created: time.Now().Unix(),
		Model:   request.Model,
		Choices: []ChatCompletionChoice{{
			Message:      message,
			Index:        0,
			FinishReason: finishReason,
		}},
		Usage: map[string]int{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	}

	metrics.RecordRequest(true, time.Since(startTime).Milliseconds(), request.Model, accountIdentifier)
	c.JSON(http.StatusOK, response)
}

// stringPtr returns a pointer to a string
func stringPtr(s string) *string {
	return &s
}
