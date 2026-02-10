package server

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"jetbrainsai2api/internal/convert"
	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/metrics"
	"jetbrainsai2api/internal/util"

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

func writeAnthropicSSEEvent(c *gin.Context, eventName string, payload []byte) error {
	if _, err := c.Writer.Write([]byte("event: " + eventName + "\n")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.Writer, "%s%s\n\n", core.StreamChunkPrefix, string(payload)); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}

func handleAnthropicStreamingResponseWithMetrics(c *gin.Context, resp *http.Response, anthReq *core.AnthropicMessagesRequest, startTime time.Time, accountIdentifier string, m *metrics.MetricsService, logger core.Logger) {
	setStreamingHeaders(c, core.APIFormatAnthropic)

	messageStartData := convert.GenerateAnthropicStreamResponse(core.StreamEventTypeMessageStart, "", 0)
	if err := writeAnthropicSSEEvent(c, core.StreamEventTypeMessageStart, messageStartData); err != nil {
		metrics.RecordFailureWithMetrics(m, startTime, anthReq.Model, accountIdentifier)
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
		payload := convert.GenerateAnthropicStreamResponse(core.StreamEventTypeContentBlockStart, "", textBlockIndex)
		if err := writeAnthropicSSEEvent(c, core.StreamEventTypeContentBlockStart, payload); err != nil {
			return err
		}
		textBlockOpen = true
		return nil
	}

	closeTextBlock := func() error {
		if !textBlockOpen {
			return nil
		}
		payload := convert.GenerateAnthropicStreamResponse(core.StreamEventTypeContentBlockStop, "", textBlockIndex)
		if err := writeAnthropicSSEEvent(c, core.StreamEventTypeContentBlockStop, payload); err != nil {
			return err
		}
		textBlockOpen = false
		return nil
	}

	startToolBlock := func() error {
		if toolState.started || toolState.id == "" || toolState.name == "" {
			return nil
		}

		startPayload := core.AnthropicStreamResponse{
			Type:  core.StreamEventTypeContentBlockStart,
			Index: &toolState.index,
			ContentBlock: &core.AnthropicContentBlock{
				Type:  core.ContentBlockTypeToolUse,
				ID:    toolState.id,
				Name:  toolState.name,
				Input: map[string]any{},
			},
		}

		data, err := util.MarshalJSON(startPayload)
		if err != nil {
			return err
		}

		if err := writeAnthropicSSEEvent(c, core.StreamEventTypeContentBlockStart, data); err != nil {
			return err
		}

		toolState.started = true
		return nil
	}

	sendToolInputDelta := func() error {
		if !toolState.started {
			return nil
		}

		inputJSON := strings.TrimSpace(toolState.rawArgs.String())
		if inputJSON == "" {
			inputJSON = "{}"
		}

		deltaPayload := map[string]any{
			"type":  core.StreamEventTypeContentBlockDelta,
			"index": toolState.index,
			"delta": map[string]any{
				"type":         "input_json_delta",
				"partial_json": inputJSON,
			},
		}

		data, err := util.MarshalJSON(deltaPayload)
		if err != nil {
			return err
		}

		return writeAnthropicSSEEvent(c, core.StreamEventTypeContentBlockDelta, data)
	}

	stopToolBlock := func() error {
		if !toolState.started || toolState.stopped {
			return nil
		}

		stopPayload := core.AnthropicStreamResponse{
			Type:  core.StreamEventTypeContentBlockStop,
			Index: &toolState.index,
		}
		data, err := util.MarshalJSON(stopPayload)
		if err != nil {
			return err
		}
		if err := writeAnthropicSSEEvent(c, core.StreamEventTypeContentBlockStop, data); err != nil {
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
		if err := sendToolInputDelta(); err != nil {
			return err
		}
		if err := stopToolBlock(); err != nil {
			return err
		}
		toolState.id = ""
		return nil
	}

	logger.Debug("=== JetBrains Streaming Response Debug ===")

	ctx := c.Request.Context()
	streamErr := ProcessJetbrainsStream(ctx, resp.Body, logger, func(streamData map[string]any) bool {
		eventType, _ := streamData["type"].(string)
		switch eventType {
		case core.JetBrainsEventTypeContent:
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

			contentBlockDeltaData := convert.GenerateAnthropicStreamResponse(core.StreamEventTypeContentBlockDelta, content, textBlockIndex)
			if err := writeAnthropicSSEEvent(c, core.StreamEventTypeContentBlockDelta, contentBlockDeltaData); err != nil {
				logger.Debug("Failed to write content delta: %v", err)
				writeErr = err
				return false
			}

		case core.JetBrainsEventTypeToolCall:
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

		case core.JetBrainsEventTypeFinishMetadata:
			if err := flushCurrentTool(); err != nil {
				logger.Debug("Failed to flush tool block at finish: %v", err)
				writeErr = err
				return false
			}
		}
		return true
	})

	if writeErr != nil {
		metrics.RecordFailureWithMetrics(m, startTime, anthReq.Model, accountIdentifier)
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
		metrics.RecordFailureWithMetrics(m, startTime, anthReq.Model, accountIdentifier)
		logger.Debug("Failed to flush trailing tool block: %v", err)
		return
	}

	logger.Debug("=== Streaming Response Summary ===")
	logger.Debug("Has content: %v", hasContent)
	logger.Debug("Full aggregated content: '%s'", fullContent.String())
	logger.Debug("===================================")

	if err := closeTextBlock(); err != nil {
		metrics.RecordFailureWithMetrics(m, startTime, anthReq.Model, accountIdentifier)
		logger.Debug("Failed to write content_block_stop: %v", err)
		return
	}

	messageStopData := convert.GenerateAnthropicStreamResponse(core.StreamEventTypeMessageStop, "", 0)
	if err := writeAnthropicSSEEvent(c, core.StreamEventTypeMessageStop, messageStopData); err != nil {
		metrics.RecordFailureWithMetrics(m, startTime, anthReq.Model, accountIdentifier)
		logger.Debug("Failed to write message_stop: %v", err)
		return
	}

	if hasContent || toolState.started {
		metrics.RecordSuccessWithMetrics(m, startTime, anthReq.Model, accountIdentifier)
		logger.Debug("Anthropic streaming response completed successfully")
	} else {
		metrics.RecordFailureWithMetrics(m, startTime, anthReq.Model, accountIdentifier)
		logger.Warn("Anthropic streaming response completed with no content")
	}
}

func handleAnthropicNonStreamingResponseWithMetrics(c *gin.Context, resp *http.Response, anthReq *core.AnthropicMessagesRequest, startTime time.Time, accountIdentifier string, m *metrics.MetricsService, logger core.Logger) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, core.MaxResponseBodySize))
	if err != nil {
		metrics.RecordFailureWithMetrics(m, startTime, anthReq.Model, accountIdentifier)
		respondWithAnthropicError(c, http.StatusInternalServerError, core.AnthropicErrorAPI,
			"Failed to read response body")
		return
	}

	logger.Debug("JetBrains API Response Body: %s", string(body))

	anthResp, err := convert.ParseJetbrainsToAnthropicDirect(body, anthReq.Model, logger)
	if err != nil {
		metrics.RecordFailureWithMetrics(m, startTime, anthReq.Model, accountIdentifier)
		logger.Error("Failed to parse response: %v", err)
		respondWithAnthropicError(c, http.StatusInternalServerError, core.AnthropicErrorAPI,
			"internal server error")
		return
	}

	metrics.RecordSuccessWithMetrics(m, startTime, anthReq.Model, accountIdentifier)
	c.JSON(http.StatusOK, anthResp)

	logger.Debug("Anthropic non-streaming response completed successfully: id=%s", anthResp.ID)
}
