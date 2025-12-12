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

// trackPerformance 记录性能指标
// 使用 defer 模式自动记录 HTTP 请求耗时
// 使用方式:
//
//	defer trackPerformance(time.Now())()
func trackPerformance(startTime time.Time) func() {
	return func() {
		duration := time.Since(startTime)
		RecordHTTPRequest(duration)
	}
}

// recordRequestResult 统一记录请求结果（成功或失败）
// KISS: 简化统计记录逻辑
func recordRequestResult(success bool, startTime time.Time, model, account string) {
	if success {
		recordSuccess(startTime, model, account)
	} else {
		recordFailureWithTimer(startTime, model, account)
		RecordHTTPError()
	}
}

// getModelConfigOrError 获取模型配置,如果不存在返回错误
// DRY: 消除重复的模型验证代码
func getModelConfigOrError(c *gin.Context, modelsData ModelsData, modelName string, startTime time.Time, errorFormat string) *ModelInfo {
	modelConfig := getModelItem(modelsData, modelName)
	if modelConfig == nil {
		recordFailureWithTimer(startTime, modelName, "")

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

// withPanicRecovery 包装 handler 函数，提供统一的 panic 恢复机制
// SRP: 单一职责 - 只负责错误恢复和资源清理
// OCP: 开闭原则 - 可扩展的资源清理逻辑
//
// 使用方式:
//
//	defer withPanicRecovery(c, startTime, &account, &resp, APIFormatOpenAI)()
func withPanicRecovery(
	c *gin.Context,
	startTime time.Time,
	account **JetbrainsAccount,
	resp **http.Response,
	accountManager AccountManager,
	errorFormat string, // "openai" 或 "anthropic"
) func() {
	return func() {
		if r := recover(); r != nil {
			Error("Panic in handler: %v", r)

			// 确保资源释放
			if resp != nil && *resp != nil && (*resp).Body != nil {
				(*resp).Body.Close()
			}
			if account != nil && *account != nil && accountManager != nil {
				accountManager.ReleaseAccount(*account)
			}

			recordFailureWithTimer(startTime, "", "")
			RecordHTTPError()

			// 根据错误格式返回不同的响应
			if errorFormat == APIFormatAnthropic {
				respondWithAnthropicError(c, http.StatusInternalServerError, AnthropicErrorAPI, "internal server error")
			} else {
				respondWithOpenAIError(c, http.StatusInternalServerError, "internal server error")
			}
		}
	}
}
