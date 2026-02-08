package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gin-gonic/gin"
)

type anthropicStreamingToolState struct {
	id      string
	name    string
	rawArgs strings.Builder

	started bool
	stopped bool

	index int
}

func (s *anthropicStreamingToolState) reset(id, name string, index int) {
	s.id = id
	s.name = name
	s.index = index
	s.rawArgs.Reset()
	s.started = false
	s.stopped = false
}

func (s *anthropicStreamingToolState) appendArgs(delta string) {
	if delta == "" {
		return
	}
	s.rawArgs.WriteString(delta)
}

func (s *anthropicStreamingToolState) parsedInput() map[string]any {
	args := strings.TrimSpace(s.rawArgs.String())
	if args == "" {
		return map[string]any{}
	}

	var parsed map[string]any
	if err := sonic.Unmarshal([]byte(args), &parsed); err != nil {
		return map[string]any{"arguments": args}
	}
	return parsed
}

func writeAnthropicSSEEvent(c *gin.Context, eventName string, payload []byte) error {
	if _, err := c.Writer.Write([]byte("event: " + eventName + "\n")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.Writer, "%s%s\n\n", StreamChunkPrefix, string(payload)); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}

// handleAnthropicStreamingResponseWithMetrics 处理流式响应 (Anthropic 格式，带注入的 MetricsService)
func handleAnthropicStreamingResponseWithMetrics(c *gin.Context, resp *http.Response, anthReq *AnthropicMessagesRequest, startTime time.Time, accountIdentifier string, metrics *MetricsService, logger Logger) {
	defer func() { _ = resp.Body.Close() }()

	setStreamingHeaders(c, APIFormatAnthropic)

	messageStartData := generateAnthropicStreamResponse(StreamEventTypeMessageStart, "", 0)
	if err := writeAnthropicSSEEvent(c, StreamEventTypeMessageStart, messageStartData); err != nil {
		recordFailureWithMetrics(metrics, startTime, anthReq.Model, accountIdentifier)
		logger.Debug("Failed to write message_start: %v", err)
		return
	}

	var fullContent strings.Builder
	var hasContent bool
	textBlockOpen := false
	textBlockIndex := -1
	nextContentBlockIndex := 0
	toolState := &anthropicStreamingToolState{}
	var writeErr error

	startTextBlock := func() error {
		if textBlockOpen {
			return nil
		}
		textBlockIndex = nextContentBlockIndex
		nextContentBlockIndex++
		payload := generateAnthropicStreamResponse(StreamEventTypeContentBlockStart, "", textBlockIndex)
		if err := writeAnthropicSSEEvent(c, StreamEventTypeContentBlockStart, payload); err != nil {
			return err
		}
		textBlockOpen = true
		return nil
	}

	closeTextBlock := func() error {
		if !textBlockOpen {
			return nil
		}
		payload := generateAnthropicStreamResponse(StreamEventTypeContentBlockStop, "", textBlockIndex)
		if err := writeAnthropicSSEEvent(c, StreamEventTypeContentBlockStop, payload); err != nil {
			return err
		}
		textBlockOpen = false
		return nil
	}

	startToolBlock := func() error {
		if toolState.started || toolState.id == "" || toolState.name == "" {
			return nil
		}

		startPayload := AnthropicStreamResponse{
			Type:  StreamEventTypeContentBlockStart,
			Index: &toolState.index,
			ContentBlock: &AnthropicContentBlock{
				Type:  ContentBlockTypeToolUse,
				ID:    toolState.id,
				Name:  toolState.name,
				Input: toolState.parsedInput(),
			},
		}

		data, err := marshalJSON(startPayload)
		if err != nil {
			return err
		}

		if err := writeAnthropicSSEEvent(c, StreamEventTypeContentBlockStart, data); err != nil {
			return err
		}

		toolState.started = true
		return nil
	}

	stopToolBlock := func() error {
		if !toolState.started || toolState.stopped {
			return nil
		}

		stopPayload := AnthropicStreamResponse{
			Type:  StreamEventTypeContentBlockStop,
			Index: &toolState.index,
		}
		data, err := marshalJSON(stopPayload)
		if err != nil {
			return err
		}
		if err := writeAnthropicSSEEvent(c, StreamEventTypeContentBlockStop, data); err != nil {
			return err
		}
		toolState.stopped = true
		return nil
	}

	flushCurrentTool := func() error {
		if toolState.id == "" {
			return nil
		}
		if err := closeTextBlock(); err != nil {
			return err
		}
		if err := startToolBlock(); err != nil {
			return err
		}
		if err := stopToolBlock(); err != nil {
			return err
		}
		return nil
	}

	logger.Debug("=== JetBrains Streaming Response Debug ===")

	ctx := c.Request.Context()
	streamErr := processJetbrainsStream(ctx, resp.Body, logger, func(streamData map[string]any) bool {
		eventType, _ := streamData["type"].(string)
		switch eventType {
		case JetBrainsEventTypeContent:
			if toolState.id != "" && !toolState.stopped {
				if err := flushCurrentTool(); err != nil {
					logger.Debug("Failed to flush pending tool before text: %v", err)
					writeErr = err
					return false
				}
			}

			content, _ := streamData["content"].(string)
			if content == "" {
				return true
			}
			if err := startTextBlock(); err != nil {
				logger.Debug("Failed to start text block: %v", err)
				writeErr = err
				return false
			}

			hasContent = true
			fullContent.WriteString(content)

			contentBlockDeltaData := generateAnthropicStreamResponse(StreamEventTypeContentBlockDelta, content, textBlockIndex)
			if err := writeAnthropicSSEEvent(c, StreamEventTypeContentBlockDelta, contentBlockDeltaData); err != nil {
				logger.Debug("Failed to write content delta: %v", err)
				writeErr = err
				return false
			}

		case JetBrainsEventTypeToolCall:
			if upstreamID, ok := streamData["id"].(string); ok && upstreamID != "" {
				if toolName, ok := streamData["name"].(string); ok && toolName != "" {
					if err := flushCurrentTool(); err != nil {
						logger.Debug("Failed to flush previous tool block: %v", err)
						writeErr = err
						return false
					}
					toolState.reset(upstreamID, toolName, nextContentBlockIndex)
					nextContentBlockIndex++
				}
			} else if contentPart, ok := streamData["content"].(string); ok {
				toolState.appendArgs(contentPart)
			}

		case JetBrainsEventTypeFinishMetadata:
			if err := flushCurrentTool(); err != nil {
				logger.Debug("Failed to flush tool block at finish: %v", err)
				writeErr = err
				return false
			}
		}
		return true
	})

	if writeErr != nil {
		recordFailureWithMetrics(metrics, startTime, anthReq.Model, accountIdentifier)
		return
	}
	if streamErr != nil {
		if ctx.Err() != nil {
			logger.Debug("Client disconnected during streaming: %v", streamErr)
		} else {
			logger.Error("Stream read error: %v", streamErr)
		}
	}

	if err := flushCurrentTool(); err != nil {
		recordFailureWithMetrics(metrics, startTime, anthReq.Model, accountIdentifier)
		logger.Debug("Failed to flush trailing tool block: %v", err)
		return
	}

	logger.Debug("=== Streaming Response Summary ===")
	logger.Debug("Has content: %v", hasContent)
	logger.Debug("Full aggregated content: '%s'", fullContent.String())
	logger.Debug("===================================")

	if err := closeTextBlock(); err != nil {
		recordFailureWithMetrics(metrics, startTime, anthReq.Model, accountIdentifier)
		logger.Debug("Failed to write content_block_stop: %v", err)
		return
	}

	messageStopData := generateAnthropicStreamResponse(StreamEventTypeMessageStop, "", 0)
	if err := writeAnthropicSSEEvent(c, StreamEventTypeMessageStop, messageStopData); err != nil {
		recordFailureWithMetrics(metrics, startTime, anthReq.Model, accountIdentifier)
		logger.Debug("Failed to write message_stop: %v", err)
		return
	}

	if hasContent || toolState.started {
		recordSuccessWithMetrics(metrics, startTime, anthReq.Model, accountIdentifier)
		logger.Debug("Anthropic streaming response completed successfully")
	} else {
		recordFailureWithMetrics(metrics, startTime, anthReq.Model, accountIdentifier)
		logger.Warn("Anthropic streaming response completed with no content")
	}
}

// handleAnthropicNonStreamingResponseWithMetrics 处理非流式响应 (Anthropic 格式，带注入的 MetricsService)
func handleAnthropicNonStreamingResponseWithMetrics(c *gin.Context, resp *http.Response, anthReq *AnthropicMessagesRequest, startTime time.Time, accountIdentifier string, metrics *MetricsService, logger Logger) {
	defer func() { _ = resp.Body.Close() }()

	// 读取完整响应
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBodySize))
	if err != nil {
		recordFailureWithMetrics(metrics, startTime, anthReq.Model, accountIdentifier)
		respondWithAnthropicError(c, http.StatusInternalServerError, AnthropicErrorAPI,
			"Failed to read response body")
		return
	}

	logger.Debug("JetBrains API Response Body: %s", string(body))

	// 直接转换 JetBrains 响应为 Anthropic 格式
	anthResp, err := parseJetbrainsToAnthropicDirect(body, anthReq.Model, logger)
	if err != nil {
		recordFailureWithMetrics(metrics, startTime, anthReq.Model, accountIdentifier)
		respondWithAnthropicError(c, http.StatusInternalServerError, AnthropicErrorAPI,
			fmt.Sprintf("Failed to parse response: %v", err))
		return
	}

	recordSuccessWithMetrics(metrics, startTime, anthReq.Model, accountIdentifier)
	c.JSON(http.StatusOK, anthResp)

	logger.Debug("Anthropic non-streaming response completed successfully: id=%s", anthResp.ID)
}
