package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bytedance/sonic"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// mapJetbrainsToOpenAIFinishReason maps JetBrains finish reason to OpenAI format
func mapJetbrainsToOpenAIFinishReason(jetbrainsReason string) string {
	switch jetbrainsReason {
	case JetBrainsFinishReasonToolCall:
		return FinishReasonToolCalls
	case JetBrainsFinishReasonLength:
		return FinishReasonLength
	case JetBrainsFinishReasonStop:
		return FinishReasonStop
	default:
		return FinishReasonStop
	}
}

// processJetbrainsStream processes the event stream from the JetBrains API.
// It calls the provided onEvent function for each event in the stream.
// Returns error if stream reading fails or context is cancelled.
func processJetbrainsStream(ctx context.Context, body io.Reader, logger Logger, onEvent func(event map[string]any) bool) error {
	scanner := bufio.NewScanner(body)
	// 增大缓冲区以处理大型工具调用参数（默认64KB不足）
	scanner.Buffer(make([]byte, MaxScannerBufferSize), MaxScannerBufferSize)
	for scanner.Scan() {
		// 检查 context 是否已取消（客户端断开连接）
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, StreamChunkPrefix) {
			continue
		}

		dataStr := strings.TrimSpace(strings.TrimPrefix(line, StreamChunkPrefix))
		if dataStr == StreamEndMarker || dataStr == StreamChunkDoneMessage {
			break
		}
		if dataStr == StreamNullValue || dataStr == "" {
			continue
		}

		var data map[string]any
		if err := sonic.Unmarshal([]byte(dataStr), &data); err != nil {
			logger.Error("Error unmarshalling stream event: %v", err)
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
func handleStreamingResponseWithMetrics(c *gin.Context, resp *http.Response, request ChatCompletionRequest, startTime time.Time, accountIdentifier string, metrics *MetricsService, logger Logger) {
	setStreamingHeaders(c, APIFormatOpenAI)

	streamID := ResponseIDPrefix + uuid.New().String()
	created := time.Now().Unix() // 预计算时间戳，避免每个 chunk 都调用 time.Now()
	firstChunkSent := false
	var currentTool *map[string]any
	var toolCalls []map[string]any
	streamFinished := false

	finalizeCurrentTool := func() {
		if currentTool == nil {
			return
		}

		if funcMap, ok := (*currentTool)["function"].(map[string]any); ok {
			if args, ok := funcMap["arguments"].(string); ok && args != "" {
				var argsTest map[string]any
				if err := sonic.Unmarshal([]byte(args), &argsTest); err != nil {
					logger.Warn("Tool call arguments are not valid JSON: %v", err)
				}
			}
		}

		toolCalls = append(toolCalls, *currentTool)
		currentTool = nil
	}

	// 使用请求的 context 来检测客户端断开
	ctx := c.Request.Context()

	err := processJetbrainsStream(ctx, resp.Body, logger, func(data map[string]any) bool {
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
				Created: created,
				Model:   request.Model,
				Choices: []StreamChoice{{Delta: deltaPayload}},
			}

			respJSON, err := marshalJSON(streamResp)
			if err != nil {
				logger.Warn("Failed to marshal stream response: %v", err)
				return true // Continue processing next event
			}
			_, _ = writeSSEData(c.Writer, respJSON)
			c.Writer.Flush()
		case JetBrainsEventTypeToolCall:
			// 处理新的ToolCall格式 - 使用上游提供的ID
			if upstreamID, ok := data["id"].(string); ok && upstreamID != "" {
				// 开始新的工具调用前先收敛前一个，避免多工具调用丢失
				finalizeCurrentTool()

				if name, ok := data["name"].(string); ok && name != "" {
					currentTool = &map[string]any{
						"index": len(toolCalls),
						"id":    upstreamID, // 使用上游提供的ID
						"function": map[string]any{
							"arguments": "",
							"name":      name,
						},
						"type": ToolTypeFunction,
					}
					logger.Debug("Started new tool call with upstream ID: %s, name: %s", upstreamID, name)
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
				// 旧格式 FunctionCall 也可能出现多个，保持与 ToolCall 一致的收敛逻辑
				finalizeCurrentTool()

				currentTool = &map[string]any{
					"index": len(toolCalls),
					"id":    generateRandomID(ToolCallIDPrefix),
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
			// 收敛最后一个工具调用
			finalizeCurrentTool()

			// 解析上游 finish_reason
			finishReason := FinishReasonStop
			if reason, ok := data["reason"].(string); ok && reason != "" {
				finishReason = mapJetbrainsToOpenAIFinishReason(reason)
			} else if len(toolCalls) > 0 {
				// 兼容: 如果有工具调用但没有 reason 字段，默认为 tool_calls
				finishReason = FinishReasonToolCalls
			}

			if len(toolCalls) > 0 {
				deltaPayload := map[string]any{
					"tool_calls": toolCalls,
				}
				if !firstChunkSent {
					deltaPayload["role"] = RoleAssistant
					firstChunkSent = true
				}
				streamResp := StreamResponse{
					ID:      streamID,
					Object:  ChatCompletionChunkObjectType,
					Created: time.Now().Unix(),
					Model:   request.Model,
					Choices: []StreamChoice{{Delta: deltaPayload}},
				}
				respJSON, err := marshalJSON(streamResp)
				if err != nil {
					logger.Warn("Failed to marshal tool call response: %v", err)
					return true // Continue processing next event
				}
				_, _ = writeSSEData(c.Writer, respJSON)
				c.Writer.Flush()
			}

			finalResp := StreamResponse{
				ID:      streamID,
				Object:  ChatCompletionChunkObjectType,
				Created: time.Now().Unix(),
				Model:   request.Model,
				Choices: []StreamChoice{{Delta: map[string]any{}, FinishReason: stringPtr(finishReason)}},
			}

			respJSON, err := marshalJSON(finalResp)
			if err != nil {
				logger.Warn("Failed to marshal final response: %v", err)
			} else {
				_, _ = writeSSEData(c.Writer, respJSON)
			}
			_, _ = writeSSEDone(c.Writer)
			c.Writer.Flush()
			streamFinished = true
			return false // Stop processing
		}
		return true // Continue processing
	})

	// 处理流处理过程中的错误
	if err != nil {
		if ctx.Err() != nil {
			// 客户端断开连接，记录但不报错
			logger.Debug("Client disconnected during streaming: %v", err)
		} else {
			// 其他流处理错误
			logger.Error("Stream processing error: %v", err)
		}
	}

	if err == nil && !streamFinished {
		// 上游缺失 FinishMetadata 时兜底收尾，确保客户端能收到 [DONE]
		finalizeCurrentTool()

		if len(toolCalls) > 0 {
			deltaPayload := map[string]any{
				"tool_calls": toolCalls,
			}
			if !firstChunkSent {
				deltaPayload["role"] = RoleAssistant
				firstChunkSent = true
			}
			streamResp := StreamResponse{
				ID:      streamID,
				Object:  ChatCompletionChunkObjectType,
				Created: time.Now().Unix(),
				Model:   request.Model,
				Choices: []StreamChoice{{Delta: deltaPayload}},
			}
			respJSON, marshalErr := marshalJSON(streamResp)
			if marshalErr != nil {
				logger.Warn("Failed to marshal fallback tool call response: %v", marshalErr)
			} else {
				_, _ = writeSSEData(c.Writer, respJSON)
			}
		}

		finishReason := FinishReasonStop
		if len(toolCalls) > 0 {
			finishReason = FinishReasonToolCalls
		}
		finalResp := StreamResponse{
			ID:      streamID,
			Object:  ChatCompletionChunkObjectType,
			Created: time.Now().Unix(),
			Model:   request.Model,
			Choices: []StreamChoice{{Delta: map[string]any{}, FinishReason: stringPtr(finishReason)}},
		}
		respJSON, marshalErr := marshalJSON(finalResp)
		if marshalErr != nil {
			logger.Warn("Failed to marshal fallback final response: %v", marshalErr)
		} else {
			_, _ = writeSSEData(c.Writer, respJSON)
		}
		_, _ = writeSSEDone(c.Writer)
		c.Writer.Flush()
	}

	metrics.RecordRequest(true, time.Since(startTime).Milliseconds(), request.Model, accountIdentifier)
}

// handleNonStreamingResponseWithMetrics handles non-streaming responses (with injected MetricsService)
func handleNonStreamingResponseWithMetrics(c *gin.Context, resp *http.Response, request ChatCompletionRequest, startTime time.Time, accountIdentifier string, metrics *MetricsService, logger Logger) {
	var contentBuilder strings.Builder
	var toolCalls []ToolCall
	var currentFuncName string
	var currentFuncArgs string
	var upstreamFinishReason string // 存储上游的 finish reason

	finalizeLegacyFunctionCall := func(reason string) {
		if currentFuncName == "" {
			return
		}

		toolCalls = append(toolCalls, ToolCall{
			ID:   generateRandomID(ToolCallIDPrefix),
			Type: ToolTypeFunction,
			Function: Function{
				Name:      currentFuncName,
				Arguments: currentFuncArgs,
			},
		})
		logger.Warn("Used fallback tool ID generation for legacy function call: %s (%s)", currentFuncName, reason)
		currentFuncName = ""
		currentFuncArgs = ""
	}

	// 使用请求的 context 来检测客户端断开
	ctx := c.Request.Context()

	err := processJetbrainsStream(ctx, resp.Body, logger, func(data map[string]any) bool {
		eventType, _ := data["type"].(string)

		switch eventType {
		case JetBrainsEventTypeContent:
			if content, ok := data["content"].(string); ok {
				contentBuilder.WriteString(content)
			}
		case JetBrainsEventTypeToolCall:
			// 处理新的ToolCall格式 - 使用上游提供的ID
			if upstreamID, ok := data["id"].(string); ok && upstreamID != "" {
				// 如果此前在走旧 FunctionCall 流，先收敛，避免混合事件导致丢失
				finalizeLegacyFunctionCall("switch_to_tool_call")

				// 开始新的工具调用 - 记录上游ID
				if name, ok := data["name"].(string); ok && name != "" {
					// 每个上游 tool_call 都应保留，避免多工具调用丢失
					toolCalls = append(toolCalls, ToolCall{
						ID:   upstreamID, // 使用上游提供的ID
						Type: ToolTypeFunction,
						Function: Function{
							Name:      name,
							Arguments: "",
						},
					})
					logger.Debug("Started new tool call with upstream ID: %s, name: %s", upstreamID, name)
				}
			} else if content, ok := data["content"].(string); ok {
				// 累积参数内容 (当ID为null时)
				// 新格式追加到最后一个 tool call，旧格式追加到 legacy 缓冲
				if len(toolCalls) > 0 {
					toolCalls[len(toolCalls)-1].Function.Arguments += content
				} else {
					currentFuncArgs += content
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
				// 旧格式可能出现多个函数调用，遇到新 name 时先收敛前一个
				finalizeLegacyFunctionCall("next_function_call")
				currentFuncName = funcName
				currentFuncArgs = ""
			}
			if currentFuncName != "" {
				currentFuncArgs += funcArgs
			}
		case JetBrainsEventTypeFinishMetadata:
			// 解析上游 finish_reason（与流式处理保持一致）
			if reason, ok := data["reason"].(string); ok && reason != "" {
				upstreamFinishReason = reason
			}

			// 结束时收敛旧格式 function call
			finalizeLegacyFunctionCall("finish_metadata")
			return false // Stop processing
		}
		return true // Continue processing
	})

	// 处理流处理过程中的错误
	if err != nil {
		if ctx.Err() != nil {
			logger.Debug("Client disconnected during non-streaming response: %v", err)
		} else {
			logger.Error("Stream processing error in non-streaming handler: %v", err)
		}
	}

	// 上游缺失 FinishMetadata 时兜底收敛 legacy function call
	if currentFuncName != "" {
		finalizeLegacyFunctionCall("missing_finish_metadata")
	}
	if len(toolCalls) > 0 {
		for i := range toolCalls {
			if validateErr := validateToolCallResponse(toolCalls[i]); validateErr != nil {
				logger.Warn("Invalid tool call response: %v", validateErr)
			}
		}
	}

	message := ChatMessage{
		Role:    RoleAssistant,
		Content: contentBuilder.String(),
	}

	// 确定 finish reason（与流式处理保持一致）
	finishReason := FinishReasonStop
	if upstreamFinishReason != "" {
		finishReason = mapJetbrainsToOpenAIFinishReason(upstreamFinishReason)
	} else if len(toolCalls) > 0 {
		// 兼容：如果有工具调用但没有 reason 字段，默认为 tool_calls
		finishReason = FinishReasonToolCalls
	}

	if len(toolCalls) > 0 {
		message.ToolCalls = toolCalls
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
