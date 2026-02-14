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

// anthropicStreamWriter manages Anthropic SSE stream state including text blocks and tool use blocks.
type anthropicStreamWriter struct {
	c      *gin.Context
	logger core.Logger

	textBlockOpen  bool
	textBlockIndex int
	nextBlockIndex int

	tool struct {
		id      string
		name    string
		rawArgs strings.Builder
		started bool
		stopped bool
		index   int
	}

	writeErr error
}

func (w *anthropicStreamWriter) writeEvent(eventName string, payload []byte) error {
	if _, err := w.c.Writer.Write([]byte("event: " + eventName + "\n")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w.c.Writer, "%s%s\n\n", core.StreamChunkPrefix, string(payload)); err != nil {
		return err
	}
	w.c.Writer.Flush()
	return nil
}

func (w *anthropicStreamWriter) startTextBlock() error {
	if w.textBlockOpen {
		return nil
	}
	w.textBlockIndex = w.nextBlockIndex
	w.nextBlockIndex++
	payload := convert.GenerateAnthropicStreamResponse(core.StreamEventTypeContentBlockStart, "", w.textBlockIndex)
	if err := w.writeEvent(core.StreamEventTypeContentBlockStart, payload); err != nil {
		return err
	}
	w.textBlockOpen = true
	return nil
}

func (w *anthropicStreamWriter) closeTextBlock() error {
	if !w.textBlockOpen {
		return nil
	}
	payload := convert.GenerateAnthropicStreamResponse(core.StreamEventTypeContentBlockStop, "", w.textBlockIndex)
	if err := w.writeEvent(core.StreamEventTypeContentBlockStop, payload); err != nil {
		return err
	}
	w.textBlockOpen = false
	return nil
}

func (w *anthropicStreamWriter) startToolBlock() error {
	if w.tool.started || w.tool.id == "" || w.tool.name == "" {
		return nil
	}

	startPayload := core.AnthropicStreamResponse{
		Type:  core.StreamEventTypeContentBlockStart,
		Index: &w.tool.index,
		ContentBlock: &core.AnthropicContentBlock{
			Type:  core.ContentBlockTypeToolUse,
			ID:    w.tool.id,
			Name:  w.tool.name,
			Input: map[string]any{},
		},
	}

	data, err := util.MarshalJSON(startPayload)
	if err != nil {
		return err
	}

	if err := w.writeEvent(core.StreamEventTypeContentBlockStart, data); err != nil {
		return err
	}

	w.tool.started = true
	return nil
}

func (w *anthropicStreamWriter) sendToolInputDelta() error {
	if !w.tool.started {
		return nil
	}

	inputJSON := strings.TrimSpace(w.tool.rawArgs.String())
	if inputJSON == "" {
		inputJSON = "{}"
	}

	deltaPayload := map[string]any{
		"type":  core.StreamEventTypeContentBlockDelta,
		"index": w.tool.index,
		"delta": map[string]any{
			"type":         "input_json_delta",
			"partial_json": inputJSON,
		},
	}

	data, err := util.MarshalJSON(deltaPayload)
	if err != nil {
		return err
	}

	return w.writeEvent(core.StreamEventTypeContentBlockDelta, data)
}

func (w *anthropicStreamWriter) stopToolBlock() error {
	if !w.tool.started || w.tool.stopped {
		return nil
	}

	stopPayload := core.AnthropicStreamResponse{
		Type:  core.StreamEventTypeContentBlockStop,
		Index: &w.tool.index,
	}
	data, err := util.MarshalJSON(stopPayload)
	if err != nil {
		return err
	}
	if err := w.writeEvent(core.StreamEventTypeContentBlockStop, data); err != nil {
		return err
	}
	w.tool.stopped = true
	return nil
}

func (w *anthropicStreamWriter) flushCurrentTool() error {
	if w.tool.id == "" {
		return nil
	}
	if err := w.closeTextBlock(); err != nil {
		return err
	}
	if err := w.startToolBlock(); err != nil {
		return err
	}
	if err := w.sendToolInputDelta(); err != nil {
		return err
	}
	if err := w.stopToolBlock(); err != nil {
		return err
	}
	w.tool.id = ""
	return nil
}

func (w *anthropicStreamWriter) resetTool(id, name string) {
	w.tool.id = id
	w.tool.name = name
	w.tool.index = w.nextBlockIndex
	w.nextBlockIndex++
	w.tool.rawArgs.Reset()
	w.tool.started = false
	w.tool.stopped = false
}

func (w *anthropicStreamWriter) appendToolArgs(delta string) {
	if delta == "" {
		return
	}
	w.tool.rawArgs.WriteString(delta)
}

func (w *anthropicStreamWriter) hasToolPending() bool {
	return w.tool.id != "" && !w.tool.stopped
}

func handleAnthropicStreamingResponseWithMetrics(c *gin.Context, resp *http.Response, anthReq *core.AnthropicMessagesRequest, startTime time.Time, accountIdentifier string, m *metrics.MetricsService, logger core.Logger) {
	setStreamingHeaders(c, core.APIFormatAnthropic)

	w := &anthropicStreamWriter{c: c, logger: logger}

	messageStartData := convert.GenerateAnthropicStreamResponse(core.StreamEventTypeMessageStart, "", 0)
	if err := w.writeEvent(core.StreamEventTypeMessageStart, messageStartData); err != nil {
		metrics.RecordFailureWithMetrics(m, startTime, anthReq.Model, accountIdentifier)
		logger.Debug("Failed to write message_start: %v", err)
		return
	}

	var fullContent strings.Builder
	var hasContent bool

	logger.Debug("=== JetBrains Streaming Response Debug ===")

	ctx := c.Request.Context()
	streamErr := ProcessJetbrainsStream(ctx, resp.Body, logger, func(streamData map[string]any) bool {
		eventType, _ := streamData["type"].(string)
		switch eventType {
		case core.JetBrainsEventTypeContent:
			if w.hasToolPending() {
				if err := w.flushCurrentTool(); err != nil {
					logger.Debug("Failed to flush pending tool before text: %v", err)
					w.writeErr = err
					return false
				}
			}

			content, _ := streamData["content"].(string)
			if content == "" {
				return true
			}
			if err := w.startTextBlock(); err != nil {
				logger.Debug("Failed to start text block: %v", err)
				w.writeErr = err
				return false
			}

			hasContent = true
			fullContent.WriteString(content)

			contentBlockDeltaData := convert.GenerateAnthropicStreamResponse(core.StreamEventTypeContentBlockDelta, content, w.textBlockIndex)
			if err := w.writeEvent(core.StreamEventTypeContentBlockDelta, contentBlockDeltaData); err != nil {
				logger.Debug("Failed to write content delta: %v", err)
				w.writeErr = err
				return false
			}

		case core.JetBrainsEventTypeToolCall:
			if upstreamID, ok := streamData["id"].(string); ok && upstreamID != "" {
				if toolName, ok := streamData["name"].(string); ok && toolName != "" {
					if err := w.flushCurrentTool(); err != nil {
						logger.Debug("Failed to flush previous tool block: %v", err)
						w.writeErr = err
						return false
					}
					w.resetTool(upstreamID, toolName)
				}
			} else if contentPart, ok := streamData["content"].(string); ok {
				w.appendToolArgs(contentPart)
			}

		case core.JetBrainsEventTypeFinishMetadata:
			if err := w.flushCurrentTool(); err != nil {
				logger.Debug("Failed to flush tool block at finish: %v", err)
				w.writeErr = err
				return false
			}
		}
		return true
	})

	if w.writeErr != nil {
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

	if err := w.flushCurrentTool(); err != nil {
		metrics.RecordFailureWithMetrics(m, startTime, anthReq.Model, accountIdentifier)
		logger.Debug("Failed to flush trailing tool block: %v", err)
		return
	}

	logger.Debug("=== Streaming Response Summary ===")
	logger.Debug("Has content: %v", hasContent)
	logger.Debug("Full aggregated content: '%s'", fullContent.String())
	logger.Debug("===================================")

	if err := w.closeTextBlock(); err != nil {
		metrics.RecordFailureWithMetrics(m, startTime, anthReq.Model, accountIdentifier)
		logger.Debug("Failed to write content_block_stop: %v", err)
		return
	}

	messageStopData := convert.GenerateAnthropicStreamResponse(core.StreamEventTypeMessageStop, "", 0)
	if err := w.writeEvent(core.StreamEventTypeMessageStop, messageStopData); err != nil {
		metrics.RecordFailureWithMetrics(m, startTime, anthReq.Model, accountIdentifier)
		logger.Debug("Failed to write message_stop: %v", err)
		return
	}

	if hasContent || w.tool.started {
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
