package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gin-gonic/gin"
)

func TestProcessJetbrainsStream_ShouldIgnoreDoneMarkerWithoutError(t *testing.T) {
	streamBody := strings.Join([]string{
		"data: {\"type\":\"Content\",\"content\":\"hello\"}",
		" data: [DONE] ",
		"data: [DONE]",
		"",
	}, "\n")

	resp := &http.Response{Body: io.NopCloser(strings.NewReader(streamBody))}
	var events int
	logger := &NopLogger{}

	err := processJetbrainsStream(
		context.Background(),
		resp,
		logger,
		func(event map[string]any) bool {
			events++
			return true
		},
	)

	if err != nil {
		t.Fatalf("processJetbrainsStream 不应因 [DONE] 报错: %v", err)
	}

	if events != 1 {
		t.Fatalf("期望只处理 1 个内容事件，实际 %d", events)
	}
}

func TestHandleNonStreamingResponseWithMetrics_MultipleToolCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{}`))
	c.Request = req

	streamBody := strings.Join([]string{
		"data: {\"type\":\"ToolCall\",\"id\":\"toolu_1\",\"name\":\"first\"}",
		"data: {\"type\":\"ToolCall\",\"content\":\"{\"}",
		"data: {\"type\":\"ToolCall\",\"content\":\"\\\"x\\\":1}\"}",
		"data: {\"type\":\"ToolCall\",\"id\":\"toolu_2\",\"name\":\"second\"}",
		"data: {\"type\":\"ToolCall\",\"content\":\"{\"}",
		"data: {\"type\":\"ToolCall\",\"content\":\"\\\"y\\\":2}\"}",
		"data: {\"type\":\"FinishMetadata\",\"reason\":\"tool_call\"}",
		"data: end",
		"",
	}, "\n")

	resp := &http.Response{Body: io.NopCloser(strings.NewReader(streamBody))}
	metrics := NewMetricsService(MetricsConfig{SaveInterval: time.Second, HistorySize: 10, Storage: nil, Logger: &NopLogger{}})
	defer func() { _ = metrics.Close() }()

	handleNonStreamingResponseWithMetrics(
		c,
		resp,
		ChatCompletionRequest{Model: "gpt-4o"},
		time.Now(),
		"acc",
		metrics,
		&NopLogger{},
	)

	body := w.Body.String()
	if !strings.Contains(body, "toolu_1") || !strings.Contains(body, "toolu_2") {
		t.Fatalf("响应应包含两个 tool call ID，实际: %s", body)
	}
}

func TestHandleStreamingResponseWithMetrics_MultipleToolCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{}`))

	streamBody := strings.Join([]string{
		"data: {\"type\":\"ToolCall\",\"id\":\"toolu_1\",\"name\":\"first\"}",
		"data: {\"type\":\"ToolCall\",\"content\":\"{\"}",
		"data: {\"type\":\"ToolCall\",\"content\":\"\\\"x\\\":1}\"}",
		"data: {\"type\":\"ToolCall\",\"id\":\"toolu_2\",\"name\":\"second\"}",
		"data: {\"type\":\"ToolCall\",\"content\":\"{\"}",
		"data: {\"type\":\"ToolCall\",\"content\":\"\\\"y\\\":2}\"}",
		"data: {\"type\":\"FinishMetadata\",\"reason\":\"tool_call\"}",
		"data: end",
		"",
	}, "\n")

	resp := &http.Response{Body: io.NopCloser(strings.NewReader(streamBody))}
	metrics := NewMetricsService(MetricsConfig{SaveInterval: time.Second, HistorySize: 10, Storage: nil, Logger: &NopLogger{}})
	defer func() { _ = metrics.Close() }()

	handleStreamingResponseWithMetrics(
		c,
		resp,
		ChatCompletionRequest{Model: "gpt-4o", Stream: true},
		time.Now(),
		"acc",
		metrics,
		&NopLogger{},
	)

	body := w.Body.String()
	if !strings.Contains(body, "toolu_1") || !strings.Contains(body, "toolu_2") {
		t.Fatalf("流式响应应包含两个 tool call ID，实际: %s", body)
	}
	if !strings.Contains(body, "\"index\":0") || !strings.Contains(body, "\"index\":1") {
		t.Fatalf("流式 tool_calls 应保留连续 index，实际: %s", body)
	}
	if !strings.Contains(body, "\"role\":\"assistant\"") {
		t.Fatalf("工具调用首个 chunk 应包含 assistant 角色，实际: %s", body)
	}
}

func TestHandleStreamingResponseWithMetrics_MissingFinishMetadataShouldSendDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{}`))

	streamBody := strings.Join([]string{
		"data: {\"type\":\"Content\",\"content\":\"hello\"}",
		"data: end",
		"",
	}, "\n")

	resp := &http.Response{Body: io.NopCloser(strings.NewReader(streamBody))}
	metrics := NewMetricsService(MetricsConfig{SaveInterval: time.Second, HistorySize: 10, Storage: nil, Logger: &NopLogger{}})
	defer func() { _ = metrics.Close() }()

	handleStreamingResponseWithMetrics(
		c,
		resp,
		ChatCompletionRequest{Model: "gpt-4o", Stream: true},
		time.Now(),
		"acc",
		metrics,
		&NopLogger{},
	)

	body := w.Body.String()
	if !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("缺失 FinishMetadata 时也应输出 [DONE]，实际: %s", body)
	}
	if !strings.Contains(body, "\"finish_reason\":\"stop\"") {
		t.Fatalf("缺失 FinishMetadata 时应回退 finish_reason=stop，实际: %s", body)
	}
}

func TestHandleNonStreamingResponseWithMetrics_MultipleLegacyFunctionCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{}`))

	streamBody := strings.Join([]string{
		"data: {\"type\":\"FunctionCall\",\"name\":\"legacy_first\",\"content\":\"{\"}",
		"data: {\"type\":\"FunctionCall\",\"content\":\"\\\"x\\\":1}\"}",
		"data: {\"type\":\"FunctionCall\",\"name\":\"legacy_second\",\"content\":\"{\"}",
		"data: {\"type\":\"FunctionCall\",\"content\":\"\\\"y\\\":2}\"}",
		"data: {\"type\":\"FinishMetadata\",\"reason\":\"tool_call\"}",
		"data: end",
		"",
	}, "\n")

	resp := &http.Response{Body: io.NopCloser(strings.NewReader(streamBody))}
	metrics := NewMetricsService(MetricsConfig{SaveInterval: time.Second, HistorySize: 10, Storage: nil, Logger: &NopLogger{}})
	defer func() { _ = metrics.Close() }()

	handleNonStreamingResponseWithMetrics(
		c,
		resp,
		ChatCompletionRequest{Model: "gpt-4o"},
		time.Now(),
		"acc",
		metrics,
		&NopLogger{},
	)

	var parsed ChatCompletionResponse
	if err := sonic.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("响应 JSON 解析失败: %v, body=%s", err, w.Body.String())
	}
	if len(parsed.Choices) != 1 {
		t.Fatalf("期望 1 个 choice，实际 %d", len(parsed.Choices))
	}
	if len(parsed.Choices[0].Message.ToolCalls) != 2 {
		t.Fatalf("期望保留 2 个 legacy function call，实际 %d", len(parsed.Choices[0].Message.ToolCalls))
	}
	if parsed.Choices[0].Message.ToolCalls[0].Function.Name != "legacy_first" {
		t.Fatalf("第一个 legacy function call 名称错误: %s", parsed.Choices[0].Message.ToolCalls[0].Function.Name)
	}
	if parsed.Choices[0].Message.ToolCalls[1].Function.Name != "legacy_second" {
		t.Fatalf("第二个 legacy function call 名称错误: %s", parsed.Choices[0].Message.ToolCalls[1].Function.Name)
	}
	if parsed.Choices[0].FinishReason != FinishReasonToolCalls {
		t.Fatalf("期望 finish_reason=tool_calls，实际 %s", parsed.Choices[0].FinishReason)
	}
}

func TestHandleNonStreamingResponseWithMetrics_FunctionCallWithoutFinishMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{}`))

	streamBody := strings.Join([]string{
		"data: {\"type\":\"FunctionCall\",\"name\":\"legacy_func\",\"content\":\"{\"}",
		"data: {\"type\":\"FunctionCall\",\"content\":\"\\\"x\\\":1}\"}",
		"data: end",
		"",
	}, "\n")

	resp := &http.Response{Body: io.NopCloser(strings.NewReader(streamBody))}
	metrics := NewMetricsService(MetricsConfig{SaveInterval: time.Second, HistorySize: 10, Storage: nil, Logger: &NopLogger{}})
	defer func() { _ = metrics.Close() }()

	handleNonStreamingResponseWithMetrics(
		c,
		resp,
		ChatCompletionRequest{Model: "gpt-4o"},
		time.Now(),
		"acc",
		metrics,
		&NopLogger{},
	)

	body := w.Body.String()
	if !strings.Contains(body, "legacy_func") {
		t.Fatalf("缺失 FinishMetadata 时也应保留 legacy function call，实际: %s", body)
	}
	if !strings.Contains(body, "\"tool_calls\"") {
		t.Fatalf("缺失 FinishMetadata 时应包含 tool_calls，实际: %s", body)
	}
}
