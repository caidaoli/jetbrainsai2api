package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/metrics"
	"jetbrainsai2api/internal/util"
	"jetbrainsai2api/internal/validate"

	"github.com/bytedance/sonic"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// MapJetbrainsToOpenAIFinishReason maps JetBrains finish reason to OpenAI format
func MapJetbrainsToOpenAIFinishReason(jetbrainsReason string) string {
	switch jetbrainsReason {
	case core.JetBrainsFinishReasonToolCall:
		return core.FinishReasonToolCalls
	case core.JetBrainsFinishReasonLength:
		return core.FinishReasonLength
	case core.JetBrainsFinishReasonStop:
		return core.FinishReasonStop
	default:
		return core.FinishReasonStop
	}
}

// ProcessJetbrainsStream processes the event stream from the JetBrains API
func ProcessJetbrainsStream(ctx context.Context, body io.Reader, logger core.Logger, onEvent func(event map[string]any) bool) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, core.MaxScannerBufferSize), core.MaxScannerBufferSize)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, core.StreamChunkPrefix) {
			continue
		}

		dataStr := strings.TrimSpace(strings.TrimPrefix(line, core.StreamChunkPrefix))
		if dataStr == core.StreamEndMarker || dataStr == core.StreamChunkDoneMessage {
			break
		}
		if dataStr == core.StreamNullValue || dataStr == "" {
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

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stream read error: %w", err)
	}

	return nil
}

func handleStreamingResponseWithMetrics(c *gin.Context, resp *http.Response, request core.ChatCompletionRequest, startTime time.Time, accountIdentifier string, m *metrics.MetricsService, logger core.Logger) {
	setStreamingHeaders(c, core.APIFormatOpenAI)

	streamID := core.ResponseIDPrefix + uuid.New().String()
	created := time.Now().Unix()
	firstChunkSent := false
	var currentTool map[string]any
	var toolCalls []any
	streamFinished := false

	finalizeCurrentTool := func() {
		if currentTool == nil {
			return
		}

		if funcMap, ok := currentTool["function"].(map[string]any); ok {
			if args, ok := funcMap["arguments"].(string); ok && args != "" {
				var argsTest map[string]any
				if err := sonic.Unmarshal([]byte(args), &argsTest); err != nil {
					logger.Warn("Tool call arguments are not valid JSON: %v", err)
				}
			}
		}

		toolCalls = append(toolCalls, currentTool)
		currentTool = nil
	}

	ctx := c.Request.Context()

	err := ProcessJetbrainsStream(ctx, resp.Body, logger, func(data map[string]any) bool {
		eventType, _ := data["type"].(string)

		switch eventType {
		case core.JetBrainsEventTypeContent:
			content, _ := data["content"].(string)
			if content == "" {
				return true
			}

			var delta core.StreamDelta
			if !firstChunkSent {
				delta = core.StreamDelta{
					Role:    core.RoleAssistant,
					Content: &content,
				}
				firstChunkSent = true
			} else {
				delta = core.StreamDelta{
					Content: &content,
				}
			}

			streamResp := core.StreamResponse{
				ID:      streamID,
				Object:  core.ChatCompletionChunkObjectType,
				Created: created,
				Model:   request.Model,
				Choices: []core.StreamChoice{{Delta: delta}},
			}

			respJSON, err := util.MarshalJSON(streamResp)
			if err != nil {
				logger.Warn("Failed to marshal stream response: %v", err)
				return true
			}
			_, _ = writeSSEData(c.Writer, respJSON)
			c.Writer.Flush()
		case core.JetBrainsEventTypeToolCall:
			if upstreamID, ok := data["id"].(string); ok && upstreamID != "" {
				finalizeCurrentTool()

				if name, ok := data["name"].(string); ok && name != "" {
					currentTool = map[string]any{
						"index": len(toolCalls),
						"id":    upstreamID,
						"function": map[string]any{
							"arguments": "",
							"name":      name,
						},
						"type": core.ToolTypeFunction,
					}
					logger.Debug("Started new tool call with upstream ID: %s, name: %s", upstreamID, name)
				}
			} else if currentTool != nil {
				if content, ok := data["content"].(string); ok {
					if funcMap, ok := currentTool["function"].(map[string]any); ok {
						currentArgs, _ := funcMap["arguments"].(string)
						funcMap["arguments"] = currentArgs + content
					}
				}
			}
		case core.JetBrainsEventTypeFunctionCall:
			funcNameInterface := data["name"]
			funcArgs, _ := data["content"].(string)

			var funcName string
			if funcNameInterface == nil {
				funcName = ""
			} else {
				funcName, _ = funcNameInterface.(string)
			}

			if funcName != "" {
				finalizeCurrentTool()

				currentTool = map[string]any{
					"index": len(toolCalls),
					"id":    util.GenerateRandomID(core.ToolCallIDPrefix),
					"function": map[string]any{
						"arguments": "",
						"name":      funcName,
					},
					"type": core.ToolTypeFunction,
				}
			} else if currentTool != nil {
				if funcMap, ok := currentTool["function"].(map[string]any); ok {
					currentArgs, _ := funcMap["arguments"].(string)
					funcMap["arguments"] = currentArgs + funcArgs
				}
			}
		case core.JetBrainsEventTypeFinishMetadata:
			finalizeCurrentTool()

			finishReason := core.FinishReasonStop
			if reason, ok := data["reason"].(string); ok && reason != "" {
				finishReason = MapJetbrainsToOpenAIFinishReason(reason)
			} else if len(toolCalls) > 0 {
				finishReason = core.FinishReasonToolCalls
			}

			if len(toolCalls) > 0 {
				delta := core.StreamDelta{
					ToolCalls: toolCalls,
				}
				if !firstChunkSent {
					delta.Role = core.RoleAssistant
					firstChunkSent = true
				}
				streamResp := core.StreamResponse{
					ID:      streamID,
					Object:  core.ChatCompletionChunkObjectType,
					Created: time.Now().Unix(),
					Model:   request.Model,
					Choices: []core.StreamChoice{{Delta: delta}},
				}
				respJSON, err := util.MarshalJSON(streamResp)
				if err != nil {
					logger.Warn("Failed to marshal tool call response: %v", err)
					return true
				}
				_, _ = writeSSEData(c.Writer, respJSON)
				c.Writer.Flush()
			}

			finalResp := core.StreamResponse{
				ID:      streamID,
				Object:  core.ChatCompletionChunkObjectType,
				Created: time.Now().Unix(),
				Model:   request.Model,
				Choices: []core.StreamChoice{{Delta: core.StreamDelta{}, FinishReason: stringPtr(finishReason)}},
			}

			respJSON, err := util.MarshalJSON(finalResp)
			if err != nil {
				logger.Warn("Failed to marshal final response: %v", err)
			} else {
				_, _ = writeSSEData(c.Writer, respJSON)
			}
			_, _ = writeSSEDone(c.Writer)
			c.Writer.Flush()
			streamFinished = true
			return false
		}
		return true
	})

	if err != nil {
		if ctx.Err() != nil {
			logger.Debug("Client disconnected during streaming: %v", err)
		} else {
			logger.Error("Stream processing error: %v", err)
		}
	}

	if err == nil && !streamFinished {
		finalizeCurrentTool()

		if len(toolCalls) > 0 {
			delta := core.StreamDelta{
				ToolCalls: toolCalls,
			}
			if !firstChunkSent {
				delta.Role = core.RoleAssistant
				firstChunkSent = true
			}
			streamResp := core.StreamResponse{
				ID:      streamID,
				Object:  core.ChatCompletionChunkObjectType,
				Created: time.Now().Unix(),
				Model:   request.Model,
				Choices: []core.StreamChoice{{Delta: delta}},
			}
			respJSON, marshalErr := util.MarshalJSON(streamResp)
			if marshalErr != nil {
				logger.Warn("Failed to marshal fallback tool call response: %v", marshalErr)
			} else {
				_, _ = writeSSEData(c.Writer, respJSON)
			}
		}

		finishReason := core.FinishReasonStop
		if len(toolCalls) > 0 {
			finishReason = core.FinishReasonToolCalls
		}
		finalResp := core.StreamResponse{
			ID:      streamID,
			Object:  core.ChatCompletionChunkObjectType,
			Created: time.Now().Unix(),
			Model:   request.Model,
			Choices: []core.StreamChoice{{Delta: core.StreamDelta{}, FinishReason: stringPtr(finishReason)}},
		}
		respJSON, marshalErr := util.MarshalJSON(finalResp)
		if marshalErr != nil {
			logger.Warn("Failed to marshal fallback final response: %v", marshalErr)
		} else {
			_, _ = writeSSEData(c.Writer, respJSON)
		}
		_, _ = writeSSEDone(c.Writer)
		c.Writer.Flush()
	}

	m.RecordRequest(err == nil, time.Since(startTime).Milliseconds(), request.Model, accountIdentifier)
}

func handleNonStreamingResponseWithMetrics(c *gin.Context, resp *http.Response, request core.ChatCompletionRequest, startTime time.Time, accountIdentifier string, m *metrics.MetricsService, logger core.Logger) {
	var contentBuilder strings.Builder
	var toolCalls []core.ToolCall
	var currentFuncName string
	var currentFuncArgs string
	var upstreamFinishReason string

	finalizeLegacyFunctionCall := func(reason string) {
		if currentFuncName == "" {
			return
		}

		toolCalls = append(toolCalls, core.ToolCall{
			ID:   util.GenerateRandomID(core.ToolCallIDPrefix),
			Type: core.ToolTypeFunction,
			Function: core.Function{
				Name:      currentFuncName,
				Arguments: currentFuncArgs,
			},
		})
		logger.Warn("Used fallback tool ID generation for legacy function call: %s (%s)", currentFuncName, reason)
		currentFuncName = ""
		currentFuncArgs = ""
	}

	ctx := c.Request.Context()

	err := ProcessJetbrainsStream(ctx, resp.Body, logger, func(data map[string]any) bool {
		eventType, _ := data["type"].(string)

		switch eventType {
		case core.JetBrainsEventTypeContent:
			if content, ok := data["content"].(string); ok {
				contentBuilder.WriteString(content)
			}
		case core.JetBrainsEventTypeToolCall:
			if upstreamID, ok := data["id"].(string); ok && upstreamID != "" {
				finalizeLegacyFunctionCall("switch_to_tool_call")

				if name, ok := data["name"].(string); ok && name != "" {
					toolCalls = append(toolCalls, core.ToolCall{
						ID:   upstreamID,
						Type: core.ToolTypeFunction,
						Function: core.Function{
							Name:      name,
							Arguments: "",
						},
					})
					logger.Debug("Started new tool call with upstream ID: %s, name: %s", upstreamID, name)
				}
			} else if content, ok := data["content"].(string); ok {
				if len(toolCalls) > 0 {
					toolCalls[len(toolCalls)-1].Function.Arguments += content
				} else {
					currentFuncArgs += content
				}
			}
		case core.JetBrainsEventTypeFunctionCall:
			funcNameInterface := data["name"]
			funcArgs, _ := data["content"].(string)

			var funcName string
			if funcNameInterface == nil {
				funcName = ""
			} else {
				funcName, _ = funcNameInterface.(string)
			}

			if funcName != "" {
				finalizeLegacyFunctionCall("next_function_call")
				currentFuncName = funcName
				currentFuncArgs = ""
			}
			if currentFuncName != "" {
				currentFuncArgs += funcArgs
			}
		case core.JetBrainsEventTypeFinishMetadata:
			if reason, ok := data["reason"].(string); ok && reason != "" {
				upstreamFinishReason = reason
			}

			finalizeLegacyFunctionCall("finish_metadata")
			return false
		}
		return true
	})

	if err != nil {
		if ctx.Err() != nil {
			logger.Debug("Client disconnected during non-streaming response: %v", err)
		} else {
			logger.Error("Stream processing error in non-streaming handler: %v", err)
		}
	}

	if currentFuncName != "" {
		finalizeLegacyFunctionCall("missing_finish_metadata")
	}
	if len(toolCalls) > 0 {
		for i := range toolCalls {
			if validateErr := validate.ValidateToolCallResponse(toolCalls[i]); validateErr != nil {
				logger.Warn("Invalid tool call response: %v", validateErr)
			}
		}
	}

	message := core.ChatMessage{
		Role:    core.RoleAssistant,
		Content: contentBuilder.String(),
	}

	finishReason := core.FinishReasonStop
	if upstreamFinishReason != "" {
		finishReason = MapJetbrainsToOpenAIFinishReason(upstreamFinishReason)
	} else if len(toolCalls) > 0 {
		finishReason = core.FinishReasonToolCalls
	}

	if len(toolCalls) > 0 {
		message.ToolCalls = toolCalls
	}

	response := core.ChatCompletionResponse{
		ID:      core.ResponseIDPrefix + uuid.New().String(),
		Object:  core.ChatCompletionObjectType,
		Created: time.Now().Unix(),
		Model:   request.Model,
		Choices: []core.ChatCompletionChoice{{
			Message:      message,
			Index:        0,
			FinishReason: finishReason,
		}},
	}

	m.RecordRequest(err == nil, time.Since(startTime).Milliseconds(), request.Model, accountIdentifier)
	c.JSON(http.StatusOK, response)
}

func stringPtr(s string) *string {
	return &s
}
