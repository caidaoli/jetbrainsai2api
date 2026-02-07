package main

import (
	"bufio"
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
func handleAnthropicStreamingResponseWithMetrics(c *gin.Context, resp *http.Response, anthReq *AnthropicMessagesRequest, startTime time.Time, accountIdentifier string, metrics *MetricsService) {
	defer func() { _ = resp.Body.Close() }()

	setStreamingHeaders(c, APIFormatAnthropic)

	messageStartData := generateAnthropicStreamResponse(StreamEventTypeMessageStart, "", 0)
	if err := writeAnthropicSSEEvent(c, StreamEventTypeMessageStart, messageStartData); err != nil {
		recordFailureWithMetrics(metrics, startTime, anthReq.Model, accountIdentifier)
		Debug("Failed to write message_start: %v", err)
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, MaxScannerBufferSize), MaxScannerBufferSize)

	var fullContent strings.Builder
	var hasContent bool
	lineCount := 0
	textBlockOpen := false
	textBlockIndex := -1
	nextContentBlockIndex := 0
	toolState := &anthropicStreamingToolState{}

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

	Debug("=== JetBrains Streaming Response Debug ===")

	ctx := c.Request.Context()
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			Debug("Client disconnected during streaming, stopping")
			return
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		lineCount++
		Debug("Line %d: '%s'", lineCount, line)

		if line == "" || !strings.HasPrefix(line, StreamChunkPrefix) {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, StreamChunkPrefix))
		Debug("Line %d: SSE data = '%s'", lineCount, data)

		if data == "" || data == StreamNullValue {
			continue
		}
		if data == StreamChunkDoneMessage || data == StreamEndMarker {
			break
		}

		var streamData map[string]any
		if err := sonic.Unmarshal([]byte(data), &streamData); err != nil {
			Debug("Line %d: Failed to parse stream data: %v", lineCount, err)
			continue
		}

		eventType, _ := streamData["type"].(string)
		switch eventType {
		case JetBrainsEventTypeContent:
			if toolState.id != "" && !toolState.stopped {
				if err := flushCurrentTool(); err != nil {
					Debug("Line %d: Failed to flush pending tool before text: %v", lineCount, err)
					return
				}
			}

			content, _ := streamData["content"].(string)
			if content == "" {
				continue
			}
			if err := startTextBlock(); err != nil {
				Debug("Line %d: Failed to start text block: %v", lineCount, err)
				return
			}

			hasContent = true
			fullContent.WriteString(content)

			contentBlockDeltaData := generateAnthropicStreamResponse(StreamEventTypeContentBlockDelta, content, textBlockIndex)
			if err := writeAnthropicSSEEvent(c, StreamEventTypeContentBlockDelta, contentBlockDeltaData); err != nil {
				Debug("Line %d: Failed to write content delta: %v", lineCount, err)
				return
			}

		case JetBrainsEventTypeToolCall:
			if upstreamID, ok := streamData["id"].(string); ok && upstreamID != "" {
				if toolName, ok := streamData["name"].(string); ok && toolName != "" {
					if err := flushCurrentTool(); err != nil {
						Debug("Line %d: Failed to flush previous tool block: %v", lineCount, err)
						return
					}
					toolState.reset(upstreamID, toolName, nextContentBlockIndex)
					nextContentBlockIndex++
				}
			} else if contentPart, ok := streamData["content"].(string); ok {
				toolState.appendArgs(contentPart)
			}

		case JetBrainsEventTypeFinishMetadata:
			if err := flushCurrentTool(); err != nil {
				Debug("Line %d: Failed to flush tool block at finish: %v", lineCount, err)
				return
			}
		}
	}

	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			Debug("Client disconnected during streaming: %v", err)
		} else {
			Error("Stream read error: %v", err)
		}
	}

	if err := flushCurrentTool(); err != nil {
		recordFailureWithMetrics(metrics, startTime, anthReq.Model, accountIdentifier)
		Debug("Failed to flush trailing tool block: %v", err)
		return
	}

	Debug("=== Streaming Response Summary ===")
	Debug("Total lines processed: %d", lineCount)
	Debug("Has content: %v", hasContent)
	Debug("Full aggregated content: '%s'", fullContent.String())
	Debug("===================================")

	if err := closeTextBlock(); err != nil {
		recordFailureWithMetrics(metrics, startTime, anthReq.Model, accountIdentifier)
		Debug("Failed to write content_block_stop: %v", err)
		return
	}

	messageStopData := generateAnthropicStreamResponse(StreamEventTypeMessageStop, "", 0)
	if err := writeAnthropicSSEEvent(c, StreamEventTypeMessageStop, messageStopData); err != nil {
		recordFailureWithMetrics(metrics, startTime, anthReq.Model, accountIdentifier)
		Debug("Failed to write message_stop: %v", err)
		return
	}

	if hasContent || toolState.started {
		recordSuccessWithMetrics(metrics, startTime, anthReq.Model, accountIdentifier)
		Debug("Anthropic streaming response completed successfully")
	} else {
		recordFailureWithMetrics(metrics, startTime, anthReq.Model, accountIdentifier)
		Warn("Anthropic streaming response completed with no content")
	}
}

// handleAnthropicNonStreamingResponseWithMetrics 处理非流式响应 (Anthropic 格式，带注入的 MetricsService)
func handleAnthropicNonStreamingResponseWithMetrics(c *gin.Context, resp *http.Response, anthReq *AnthropicMessagesRequest, startTime time.Time, accountIdentifier string, metrics *MetricsService) {
	defer func() { _ = resp.Body.Close() }()

	// 读取完整响应
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBodySize))
	if err != nil {
		recordFailureWithMetrics(metrics, startTime, anthReq.Model, accountIdentifier)
		respondWithAnthropicError(c, http.StatusInternalServerError, AnthropicErrorAPI,
			"Failed to read response body")
		return
	}

	Debug("JetBrains API Response Body: %s", string(body))

	// 直接转换 JetBrains 响应为 Anthropic 格式 (KISS: 消除中间转换)
	anthResp, err := parseJetbrainsToAnthropicDirect(body, anthReq.Model)
	if err != nil {
		recordFailureWithMetrics(metrics, startTime, anthReq.Model, accountIdentifier)
		respondWithAnthropicError(c, http.StatusInternalServerError, AnthropicErrorAPI,
			fmt.Sprintf("Failed to parse response: %v", err))
		return
	}

	recordSuccessWithMetrics(metrics, startTime, anthReq.Model, accountIdentifier)
	c.JSON(http.StatusOK, anthResp)

	Debug("Anthropic non-streaming response completed successfully: id=%s", anthResp.ID)
}

// parseJetbrainsStreamData 解析 JetBrains 流式数据
// KISS: 保持简单的解析逻辑
func parseJetbrainsStreamData(data string) (string, error) {
	if data == "" || data == StreamNullValue {
		return "", nil
	}

	// 尝试解析 JSON 数据
	var streamData map[string]any
	if err := sonic.Unmarshal([]byte(data), &streamData); err != nil {
		// 如果不是 JSON，可能是纯文本
		return data, nil
	}

	// 提取内容：优先处理 JetBrains API 格式
	if eventType, ok := streamData["type"].(string); ok && eventType == JetBrainsEventTypeContent {
		if content, ok := streamData["content"].(string); ok {
			return content, nil
		}
	}

	// 兼容 OpenAI 格式 (保留原有逻辑)
	if choices, ok := streamData["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if delta, ok := choice["delta"].(map[string]any); ok {
				if content, ok := delta["content"].(string); ok {
					return content, nil
				}
			}
		}
	}

	// 检查是否是直接的内容响应
	if content, ok := streamData["content"].(string); ok {
		return content, nil
	}

	return "", nil
}

// parseJetbrainsNonStreamResponse 解析 JetBrains 非流式响应
// 兼容处理：JetBrains API 总是返回流式格式，需要聚合数据
func parseJetbrainsNonStreamResponse(body []byte, model string) (*ChatCompletionResponse, error) {
	bodyStr := string(body)

	// 检查是否是流式响应格式 (以 "data:" 开头)
	if strings.HasPrefix(strings.TrimSpace(bodyStr), "data:") {
		return parseAndAggregateStreamResponse(bodyStr, model)
	}

	// 尝试解析为完整的聊天响应 (保留原有逻辑)
	var jetbrainsResp map[string]any
	if err := sonic.Unmarshal(body, &jetbrainsResp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// 提取内容
	var content string
	if contentField, exists := jetbrainsResp["content"]; exists {
		if contentStr, ok := contentField.(string); ok {
			content = contentStr
		} else if contentArray, ok := contentField.([]any); ok {
			// 处理数组格式的内容
			var contentParts []string
			for _, part := range contentArray {
				if partStr, ok := part.(string); ok {
					contentParts = append(contentParts, partStr)
				}
			}
			content = strings.Join(contentParts, "")
		}
	}

	// 构建 OpenAI 格式响应 (DRY: 复用响应构建逻辑)
	openAIResp := &ChatCompletionResponse{
		ID:      generateResponseID(),
		Object:  ChatCompletionObjectType,
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []ChatCompletionChoice{
			{
				Message: ChatMessage{
					Role:    RoleAssistant,
					Content: content,
				},
				Index:        0,
				FinishReason: FinishReasonStop,
			},
		},
		Usage: map[string]int{
			"prompt_tokens":     estimateTokenCount(content) / 4, // 粗略估算
			"completion_tokens": estimateTokenCount(content),
			"total_tokens":      estimateTokenCount(content) * 5 / 4,
		},
	}

	return openAIResp, nil
}

// generateResponseID 生成响应 ID (KISS: 简单的 ID 生成)
func generateResponseID() string {
	return generateID(ResponseIDPrefix)
}

// estimateTokenCount 估算 token 数量 (KISS: 简单估算)
func estimateTokenCount(text string) int {
	// 简单估算：平均每个 token 约 4 个字符
	return len(text) / 4
}

// parseAndAggregateStreamResponse 解析并聚合流式响应数据
// 处理 JetBrains API 的流式格式，聚合所有内容片段
func parseAndAggregateStreamResponse(bodyStr, model string) (*ChatCompletionResponse, error) {
	lines := strings.Split(bodyStr, "\n")
	var contentParts []string
	var finishReason string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// 跳过空行和结束标记
		if line == "" || line == StreamEndLine {
			continue
		}

		// 处理 "data: " 前缀的行
		if strings.HasPrefix(line, StreamChunkPrefix) {
			jsonData := strings.TrimPrefix(line, StreamChunkPrefix)

			// 解析 JSON 数据
			var streamData map[string]any
			if err := sonic.Unmarshal([]byte(jsonData), &streamData); err != nil {
				Debug("Failed to parse stream JSON: %v, data: %s", err, jsonData)
				continue
			}

			// 提取内容类型
			eventType, _ := streamData["type"].(string)

			switch eventType {
			case JetBrainsEventTypeContent:
				// 提取内容片段
				if content, ok := streamData["content"].(string); ok {
					contentParts = append(contentParts, content)
				}
			case JetBrainsEventTypeFinishMetadata:
				// 提取结束原因
				if reason, ok := streamData["reason"].(string); ok {
					finishReason = reason
				}
			}
		}
	}

	// 聚合所有内容片段
	fullContent := strings.Join(contentParts, "")

	if finishReason == "" {
		finishReason = FinishReasonStop // 默认结束原因
	}

	// 构建完整的 OpenAI 格式响应
	response := &ChatCompletionResponse{
		ID:      generateResponseID(),
		Object:  ChatCompletionObjectType,
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []ChatCompletionChoice{
			{
				Index: 0,
				Message: ChatMessage{
					Role:    RoleAssistant,
					Content: fullContent,
				},
				FinishReason: finishReason,
			},
		},
		Usage: map[string]int{
			"prompt_tokens":     0, // JetBrains API 通常不返回 token 计数
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	}

	Debug("Successfully aggregated stream response: %d content parts, finish_reason=%s",
		len(contentParts), finishReason)

	return response, nil
}
