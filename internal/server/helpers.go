package server

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/metrics"

	"github.com/gin-gonic/gin"
)

// setStreamingHeaders sets streaming response HTTP headers
func setStreamingHeaders(c *gin.Context, format string) {
	c.Header(core.HeaderContentType, core.ContentTypeEventStream)
	c.Header(core.HeaderCacheControl, core.CacheControlNoCache)
	c.Header(core.HeaderConnection, core.ConnectionKeepAlive)
	if format == core.APIFormatAnthropic {
		c.Header("Access-Control-Allow-Origin", "*")
	}
}

// writeSSEData writes SSE format data
func writeSSEData(w io.Writer, data []byte) (int, error) {
	return fmt.Fprintf(w, "%s%s\n\n", core.StreamChunkPrefix, string(data))
}

// writeSSEDone writes SSE end marker
func writeSSEDone(w io.Writer) (int, error) {
	return fmt.Fprintf(w, "%s%s\n\n", core.StreamChunkPrefix, core.StreamChunkDoneMessage)
}

// respondWithOpenAIError returns OpenAI format error response
func respondWithOpenAIError(c *gin.Context, code int, message string) {
	c.JSON(code, gin.H{"error": message})
}

// respondWithAnthropicError returns Anthropic format error response
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

// trackPerformanceWithMetrics records performance metrics
func trackPerformanceWithMetrics(m *metrics.MetricsService, startTime time.Time) func() {
	return func() {
		duration := time.Since(startTime)
		m.RecordHTTPRequest(duration)
	}
}

// recordRequestResultWithMetrics records request result
func recordRequestResultWithMetrics(m *metrics.MetricsService, success bool, startTime time.Time, model, account string) {
	if success {
		metrics.RecordSuccessWithMetrics(m, startTime, model, account)
	} else {
		metrics.RecordFailureWithMetrics(m, startTime, model, account)
	}
}

// getModelConfigOrErrorWithMetrics gets model config or returns error
func getModelConfigOrErrorWithMetrics(c *gin.Context, m *metrics.MetricsService, modelsData core.ModelsData, modelName string, startTime time.Time, errorFormat string) *core.ModelInfo {
	for _, model := range modelsData.Data {
		if model.ID == modelName {
			return &model
		}
	}

	metrics.RecordFailureWithMetrics(m, startTime, modelName, "")

	if errorFormat == core.APIFormatAnthropic {
		respondWithAnthropicError(c, http.StatusNotFound, core.AnthropicErrorModelNotFound,
			fmt.Sprintf("Model %s not found", modelName))
	} else {
		respondWithOpenAIError(c, http.StatusNotFound, fmt.Sprintf("Model %s not found", modelName))
	}
	return nil
}

// withPanicRecoveryWithMetrics wraps handler with panic recovery
func withPanicRecoveryWithMetrics(
	c *gin.Context,
	m *metrics.MetricsService,
	startTime time.Time,
	resp **http.Response,
	errorFormat string,
	logger core.Logger,
) func() {
	return func() {
		if r := recover(); r != nil {
			logger.Error("Panic in handler: %v", r)

			if resp != nil && *resp != nil && (*resp).Body != nil {
				_ = (*resp).Body.Close()
			}

			metrics.RecordFailureWithMetrics(m, startTime, "", "")

			if errorFormat == core.APIFormatAnthropic {
				respondWithAnthropicError(c, http.StatusInternalServerError, core.AnthropicErrorAPI, "internal server error")
			} else {
				respondWithOpenAIError(c, http.StatusInternalServerError, "internal server error")
			}
		}
	}
}
