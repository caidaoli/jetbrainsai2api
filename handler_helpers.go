package main

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// ============================================================================
// Handler 辅助函数
// DRY: 消除 handlers.go 和 anthropic_handlers.go 之间的重复代码
// KISS: 简化 handler 逻辑，提供统一的资源管理和错误处理
// ============================================================================

// setStreamingHeaders 设置流式响应的 HTTP 头
// DRY: 消除 response_handler.go 和 anthropic_response_handler.go 的重复代码
func setStreamingHeaders(c *gin.Context, format string) {
	c.Header(HeaderContentType, ContentTypeEventStream)
	c.Header(HeaderCacheControl, CacheControlNoCache)
	c.Header(HeaderConnection, ConnectionKeepAlive)
	if format == APIFormatAnthropic {
		c.Header("Access-Control-Allow-Origin", "*")
	}
}

// writeSSEData 写入 SSE 格式数据
// DRY: 统一 SSE 数据写入格式 "data: {json}\n\n"
func writeSSEData(w io.Writer, data []byte) (int, error) {
	return fmt.Fprintf(w, "%s%s\n\n", StreamChunkPrefix, string(data))
}

// writeSSEDone 写入 SSE 结束标记
// DRY: 统一 SSE 结束标记写入
func writeSSEDone(w io.Writer) (int, error) {
	return fmt.Fprintf(w, "%s%s\n\n", StreamChunkPrefix, StreamChunkDoneMessage)
}

// respondWithOpenAIError 返回 OpenAI 格式的错误响应
// 格式: {"error": "message"}
func respondWithOpenAIError(c *gin.Context, code int, message string) {
	c.JSON(code, gin.H{"error": message})
}

// respondWithAnthropicError 返回 Anthropic 格式的错误响应
// 格式: {"type": "error", "error": {"type": "...", "message": "..."}}
func respondWithAnthropicError(c *gin.Context, statusCode int, errorType, message string) {
	errorResp := gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errorType,
			"message": message,
		},
	}
	c.JSON(statusCode, errorResp)
}

// trackPerformanceWithMetrics 记录性能指标（使用注入的 MetricsService）
// 使用方式:
//
//	defer trackPerformanceWithMetrics(s.metricsService, time.Now())()
func trackPerformanceWithMetrics(metrics *MetricsService, startTime time.Time) func() {
	return func() {
		duration := time.Since(startTime)
		metrics.RecordHTTPRequest(duration)
	}
}

// recordRequestResultWithMetrics 统一记录请求结果（使用注入的 MetricsService）
func recordRequestResultWithMetrics(metrics *MetricsService, success bool, startTime time.Time, model, account string) {
	if success {
		recordSuccessWithMetrics(metrics, startTime, model, account)
	} else {
		recordFailureWithMetrics(metrics, startTime, model, account)
	}
}

// getModelConfigOrErrorWithMetrics 获取模型配置（使用注入的 MetricsService）
func getModelConfigOrErrorWithMetrics(c *gin.Context, metrics *MetricsService, modelsData ModelsData, modelName string, startTime time.Time, errorFormat string) *ModelInfo {
	modelConfig := getModelItem(modelsData, modelName)
	if modelConfig == nil {
		recordFailureWithMetrics(metrics, startTime, modelName, "")

		// 根据错误格式返回不同的响应
		if errorFormat == APIFormatAnthropic {
			respondWithAnthropicError(c, http.StatusNotFound, AnthropicErrorModelNotFound,
				fmt.Sprintf("Model %s not found", modelName))
		} else {
			respondWithOpenAIError(c, http.StatusNotFound, fmt.Sprintf("Model %s not found", modelName))
		}
		return nil
	}
	return modelConfig
}

// withPanicRecoveryWithMetrics 包装 handler 函数（使用注入的 MetricsService）
func withPanicRecoveryWithMetrics(
	c *gin.Context,
	metrics *MetricsService,
	startTime time.Time,
	resp **http.Response,
	errorFormat string,
	logger Logger,
) func() {
	return func() {
		if r := recover(); r != nil {
			logger.Error("Panic in handler: %v", r)

			// 确保资源释放
			if resp != nil && *resp != nil && (*resp).Body != nil {
				_ = (*resp).Body.Close()
			}

			recordFailureWithMetrics(metrics, startTime, "", "")

			// 根据错误格式返回不同的响应
			if errorFormat == APIFormatAnthropic {
				respondWithAnthropicError(c, http.StatusInternalServerError, AnthropicErrorAPI, "internal server error")
			} else {
				respondWithOpenAIError(c, http.StatusInternalServerError, "internal server error")
			}
		}
	}
}
