package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestHandleAnthropicStreamingResponseWithMetrics_ShouldEmitToolUseBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	streamBody := strings.Join([]string{
		"data: {\"type\":\"ToolCall\",\"id\":\"toolu_abc\",\"name\":\"get_weather\"}",
		"data: {\"type\":\"ToolCall\",\"content\":\"{\"}",
		"data: {\"type\":\"ToolCall\",\"content\":\"\\\"city\\\":\\\"Beijing\\\"}\"}",
		"data: {\"type\":\"FinishMetadata\",\"reason\":\"tool_call\"}",
		"data: end",
		"",
	}, "\n")

	resp := &http.Response{Body: io.NopCloser(strings.NewReader(streamBody))}
	metrics := NewMetricsService(MetricsConfig{SaveInterval: time.Second, HistorySize: 10, Storage: nil, Logger: &NopLogger{}})
	defer func() { _ = metrics.Close() }()

	handleAnthropicStreamingResponseWithMetrics(
		c,
		resp,
		&AnthropicMessagesRequest{Model: "gpt-4o"},
		time.Now(),
		"acc",
		metrics,
	)

	body := w.Body.String()
	if !strings.Contains(body, "event: content_block_start") {
		t.Fatalf("应包含 content_block_start 事件，实际: %s", body)
	}
	if !strings.Contains(body, "\"type\":\"tool_use\"") {
		t.Fatalf("应输出 tool_use 内容块，实际: %s", body)
	}
	if !strings.Contains(body, "\"id\":\"toolu_abc\"") {
		t.Fatalf("应包含上游 tool call id，实际: %s", body)
	}
	if !strings.Contains(body, "\"input\":{\"city\":\"Beijing\"}") {
		t.Fatalf("工具参数应聚合为完整 JSON，实际: %s", body)
	}
	if !strings.Contains(body, "event: message_stop") {
		t.Fatalf("应包含 message_stop 事件，实际: %s", body)
	}
}

func TestHandleAnthropicStreamingResponseWithMetrics_MultipleToolsShouldEmitCompleteBlocks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	streamBody := strings.Join([]string{
		"data: {\"type\":\"ToolCall\",\"id\":\"toolu_1\",\"name\":\"get_weather\"}",
		"data: {\"type\":\"ToolCall\",\"content\":\"{\\\"city\\\":\\\"Beijing\\\"}\"}",
		"data: {\"type\":\"ToolCall\",\"id\":\"toolu_2\",\"name\":\"get_weather\"}",
		"data: {\"type\":\"ToolCall\",\"content\":\"{\\\"city\\\":\\\"Shanghai\\\"}\"}",
		"data: {\"type\":\"FinishMetadata\",\"reason\":\"tool_call\"}",
		"data: end",
		"",
	}, "\n")

	resp := &http.Response{Body: io.NopCloser(strings.NewReader(streamBody))}
	metrics := NewMetricsService(MetricsConfig{SaveInterval: time.Second, HistorySize: 10, Storage: nil, Logger: &NopLogger{}})
	defer func() { _ = metrics.Close() }()

	handleAnthropicStreamingResponseWithMetrics(
		c,
		resp,
		&AnthropicMessagesRequest{Model: "gpt-4o"},
		time.Now(),
		"acc",
		metrics,
	)

	body := w.Body.String()
	if strings.Count(body, "\"type\":\"tool_use\"") != 2 {
		t.Fatalf("应输出 2 个 tool_use 内容块，实际: %s", body)
	}
	if !strings.Contains(body, "\"id\":\"toolu_1\"") || !strings.Contains(body, "\"id\":\"toolu_2\"") {
		t.Fatalf("应保留所有上游 tool call id，实际: %s", body)
	}
	if !strings.Contains(body, "\"index\":0") || !strings.Contains(body, "\"index\":1") {
		t.Fatalf("多工具内容块 index 应连续分配，实际: %s", body)
	}
	if !strings.Contains(body, "\"input\":{\"city\":\"Beijing\"}") || !strings.Contains(body, "\"input\":{\"city\":\"Shanghai\"}") {
		t.Fatalf("多工具参数应分别完整聚合，实际: %s", body)
	}
}

func TestHandleAnthropicStreamingResponseWithMetrics_ToolOnlyShouldRecordSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	streamBody := strings.Join([]string{
		"data: {\"type\":\"ToolCall\",\"id\":\"toolu_only\",\"name\":\"get_weather\"}",
		"data: {\"type\":\"ToolCall\",\"content\":\"{\\\"city\\\":\\\"Beijing\\\"}\"}",
		"data: {\"type\":\"FinishMetadata\",\"reason\":\"tool_call\"}",
		"data: end",
		"",
	}, "\n")

	resp := &http.Response{Body: io.NopCloser(strings.NewReader(streamBody))}
	metrics := NewMetricsService(MetricsConfig{SaveInterval: time.Second, HistorySize: 10, Storage: nil, Logger: &NopLogger{}})
	defer func() { _ = metrics.Close() }()

	handleAnthropicStreamingResponseWithMetrics(
		c,
		resp,
		&AnthropicMessagesRequest{Model: "gpt-4o"},
		time.Now(),
		"acc",
		metrics,
	)

	body := w.Body.String()
	if !strings.Contains(body, "\"index\":0") {
		t.Fatalf("仅工具流时首个工具块 index 应为 0，实际: %s", body)
	}

	stats := metrics.GetRequestStats()
	if stats.TotalRequests == 0 {
		t.Fatalf("应记录请求统计")
	}
	if stats.SuccessfulRequests == 0 {
		t.Fatalf("仅工具调用流也应记为成功")
	}
}

func TestHandleAnthropicStreamingResponseWithMetrics_TextThenToolShouldKeepSequentialOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	streamBody := strings.Join([]string{
		"data: {\"type\":\"Content\",\"content\":\"hello\"}",
		"data: {\"type\":\"ToolCall\",\"id\":\"toolu_text_then_tool\",\"name\":\"get_weather\"}",
		"data: {\"type\":\"ToolCall\",\"content\":\"{\\\"city\\\":\\\"Beijing\\\"}\"}",
		"data: {\"type\":\"FinishMetadata\",\"reason\":\"tool_call\"}",
		"data: end",
		"",
	}, "\n")

	resp := &http.Response{Body: io.NopCloser(strings.NewReader(streamBody))}
	metrics := NewMetricsService(MetricsConfig{SaveInterval: time.Second, HistorySize: 10, Storage: nil, Logger: &NopLogger{}})
	defer func() { _ = metrics.Close() }()

	handleAnthropicStreamingResponseWithMetrics(
		c,
		resp,
		&AnthropicMessagesRequest{Model: "gpt-4o"},
		time.Now(),
		"acc",
		metrics,
	)

	body := w.Body.String()
	textStartPos := strings.Index(body, "\"type\":\"content_block_start\",\"index\":0")
	textDeltaPos := strings.Index(body, "\"type\":\"content_block_delta\",\"index\":0")
	textStopPos := strings.Index(body, "\"type\":\"content_block_stop\",\"index\":0")
	toolStartPos := strings.Index(body, "\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_text_then_tool\"")

	if textStartPos == -1 || textDeltaPos == -1 || textStopPos == -1 || toolStartPos == -1 {
		t.Fatalf("应包含文本块(index=0)和工具块(index=1)完整事件，实际: %s", body)
	}
	if textStartPos >= textDeltaPos || textDeltaPos >= textStopPos || textStopPos >= toolStartPos {
		t.Fatalf("文本块应先于工具块输出，实际: %s", body)
	}
}

func TestHandleAnthropicStreamingResponseWithMetrics_TextToolTextShouldUseNewTextBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	streamBody := strings.Join([]string{
		"data: {\"type\":\"Content\",\"content\":\"hello\"}",
		"data: {\"type\":\"ToolCall\",\"id\":\"toolu_mid\",\"name\":\"get_weather\"}",
		"data: {\"type\":\"ToolCall\",\"content\":\"{\\\"city\\\":\\\"Beijing\\\"}\"}",
		"data: {\"type\":\"Content\",\"content\":\"world\"}",
		"data: {\"type\":\"FinishMetadata\",\"reason\":\"stop\"}",
		"data: end",
		"",
	}, "\n")

	resp := &http.Response{Body: io.NopCloser(strings.NewReader(streamBody))}
	metrics := NewMetricsService(MetricsConfig{SaveInterval: time.Second, HistorySize: 10, Storage: nil, Logger: &NopLogger{}})
	defer func() { _ = metrics.Close() }()

	handleAnthropicStreamingResponseWithMetrics(
		c,
		resp,
		&AnthropicMessagesRequest{Model: "gpt-4o"},
		time.Now(),
		"acc",
		metrics,
	)

	body := w.Body.String()
	firstTextDeltaPos := strings.Index(body, "\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}")
	toolStartPos := strings.Index(body, "\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_mid\"")
	secondTextStartPos := strings.Index(body, "\"type\":\"content_block_start\",\"index\":2")
	secondTextDeltaPos := strings.Index(body, "\"type\":\"content_block_delta\",\"index\":2,\"delta\":{\"type\":\"text_delta\",\"text\":\"world\"}")

	if firstTextDeltaPos == -1 || toolStartPos == -1 || secondTextStartPos == -1 || secondTextDeltaPos == -1 {
		t.Fatalf("text->tool->text 场景应输出独立文本块(index=0,2)与工具块(index=1)，实际: %s", body)
	}
	if firstTextDeltaPos >= toolStartPos || toolStartPos >= secondTextStartPos || secondTextStartPos >= secondTextDeltaPos {
		t.Fatalf("text/tool/text 事件顺序错误，实际: %s", body)
	}
}

func TestHandleAnthropicStreamingResponseWithMetrics_ToolThenTextShouldKeepSequentialOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	streamBody := strings.Join([]string{
		"data: {\"type\":\"ToolCall\",\"id\":\"toolu_tool_then_text\",\"name\":\"get_weather\"}",
		"data: {\"type\":\"ToolCall\",\"content\":\"{\\\"city\\\":\\\"Beijing\\\"}\"}",
		"data: {\"type\":\"Content\",\"content\":\"after-tool\"}",
		"data: {\"type\":\"FinishMetadata\",\"reason\":\"stop\"}",
		"data: end",
		"",
	}, "\n")

	resp := &http.Response{Body: io.NopCloser(strings.NewReader(streamBody))}
	metrics := NewMetricsService(MetricsConfig{SaveInterval: time.Second, HistorySize: 10, Storage: nil, Logger: &NopLogger{}})
	defer func() { _ = metrics.Close() }()

	handleAnthropicStreamingResponseWithMetrics(
		c,
		resp,
		&AnthropicMessagesRequest{Model: "gpt-4o"},
		time.Now(),
		"acc",
		metrics,
	)

	body := w.Body.String()
	toolStartPos := strings.Index(body, "\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_tool_then_text\"")
	textStartPos := strings.Index(body, "\"type\":\"content_block_start\",\"index\":1")
	textDeltaPos := strings.Index(body, "\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"text_delta\",\"text\":\"after-tool\"}")

	if toolStartPos == -1 || textStartPos == -1 || textDeltaPos == -1 {
		t.Fatalf("tool->text 场景应输出工具块(index=0)与文本块(index=1)，实际: %s", body)
	}
	if toolStartPos >= textStartPos || textStartPos >= textDeltaPos {
		t.Fatalf("tool->text 事件顺序错误，实际: %s", body)
	}
}
