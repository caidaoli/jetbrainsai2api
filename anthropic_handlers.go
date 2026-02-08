package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// anthropicMessages 处理 Anthropic Messages API 请求（Server 方法）
// SRP: 专门处理 Anthropic 协议的单一职责
func (s *Server) anthropicMessages(c *gin.Context) {
	startTime := time.Now()
	logger := s.config.Logger

	// Panic 恢复机制和性能追踪
	var account *JetbrainsAccount
	var resp *http.Response
	defer withPanicRecoveryWithMetrics(c, s.metricsService, startTime, &resp, APIFormatAnthropic, logger)()
	defer trackPerformanceWithMetrics(s.metricsService, startTime)()

	var anthReq AnthropicMessagesRequest
	if err := c.ShouldBindJSON(&anthReq); err != nil {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, "", "")
		respondWithAnthropicError(c, http.StatusBadRequest, AnthropicErrorInvalidRequest, err.Error())
		return
	}

	logger.Debug("Received Anthropic Messages request: model=%s, max_tokens=%d, messages=%d",
		anthReq.Model, anthReq.MaxTokens, len(anthReq.Messages))

	// 记录完整的客户端请求详情 (debug模式下)
	if requestBytes, err := marshalJSON(&anthReq); err == nil {
		logger.Debug("=== Client Request Debug (Anthropic Messages) ===")
		logger.Debug("Request size: %d bytes", len(requestBytes))
		logger.Debug("Complete request payload: %s", string(requestBytes))
		logger.Debug("=== End Client Request Debug ===")
	}

	// 验证必填字段
	if anthReq.Model == "" {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, anthReq.Model, "")
		respondWithAnthropicError(c, http.StatusBadRequest, AnthropicErrorInvalidRequest, "model is required")
		return
	}

	if anthReq.MaxTokens <= 0 {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, anthReq.Model, "")
		respondWithAnthropicError(c, http.StatusBadRequest, AnthropicErrorInvalidRequest, "max_tokens must be positive")
		return
	}

	if len(anthReq.Messages) == 0 {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, anthReq.Model, "")
		respondWithAnthropicError(c, http.StatusBadRequest, AnthropicErrorInvalidRequest, "messages cannot be empty")
		return
	}

	// 检查模型是否存在
	modelConfig := getModelConfigOrErrorWithMetrics(c, s.metricsService, s.modelsData, anthReq.Model, startTime, APIFormatAnthropic)
	if modelConfig == nil {
		return
	}

	// 使用 AccountManager 获取账户
	var err error
	account, err = s.accountManager.AcquireAccount(c.Request.Context())
	if err != nil {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, anthReq.Model, "")
		respondWithAnthropicError(c, http.StatusTooManyRequests, AnthropicErrorRateLimit, err.Error())
		return
	}
	defer s.accountManager.ReleaseAccount(account)

	accountIdentifier := getTokenDisplayName(account)

	// 直接转换 Anthropic → JetBrains
	jetbrainsMessages := anthropicToJetbrainsMessages(anthReq.Messages)

	// 处理 system 字段 - Anthropic 的 system 是单独字段，需要转换为 system_message
	if anthReq.System != "" {
		systemMsg := JetbrainsMessage{
			Type:    JetBrainsMessageTypeSystem,
			Content: string(anthReq.System),
		}
		// 将 system_message 插入到消息数组开头
		jetbrainsMessages = append([]JetbrainsMessage{systemMsg}, jetbrainsMessages...)
		logger.Debug("Added system_message from Anthropic system field")
	}

	// 处理工具定义
	var data []JetbrainsData
	if len(anthReq.Tools) > 0 {
		jetbrainsTools := anthropicToJetbrainsTools(anthReq.Tools)
		data = append(data, JetbrainsData{Type: "json", FQDN: "llm.parameters.tools"})

		toolsJSON, marshalErr := marshalJSON(jetbrainsTools)
		if marshalErr != nil {
			recordRequestResultWithMetrics(s.metricsService, false, startTime, anthReq.Model, accountIdentifier)
			respondWithAnthropicError(c, http.StatusInternalServerError, AnthropicErrorAPI, "Failed to marshal tools")
			return
		}
		data = append(data, JetbrainsData{Type: "json", Value: string(toolsJSON)})
	}

	// 直接调用 JetBrains API
	var statusCode int
	//nolint:bodyclose // resp.Body 在 handleAnthropicStreamingResponseWithMetrics 和 handleAnthropicNonStreamingResponseWithMetrics 中关闭
	resp, statusCode, err = s.callJetbrainsAPIDirect(c.Request.Context(), &anthReq, jetbrainsMessages, data, account)
	if err != nil {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, anthReq.Model, accountIdentifier)
		respondWithAnthropicError(c, statusCode, AnthropicErrorAPI, err.Error())
		return
	}

	// 根据是否流式处理响应
	isStream := anthReq.Stream != nil && *anthReq.Stream
	if isStream {
		handleAnthropicStreamingResponseWithMetrics(c, resp, &anthReq, startTime, accountIdentifier, s.metricsService, logger)
	} else {
		handleAnthropicNonStreamingResponseWithMetrics(c, resp, &anthReq, startTime, accountIdentifier, s.metricsService, logger)
	}
}

// callJetbrainsAPIDirect 直接调用 JetBrains API
// KISS: 简化调用链，消除中间转换
func (s *Server) callJetbrainsAPIDirect(ctx context.Context, anthReq *AnthropicMessagesRequest, jetbrainsMessages []JetbrainsMessage, data []JetbrainsData, account *JetbrainsAccount) (*http.Response, int, error) {
	logger := s.config.Logger
	internalModel := getInternalModelName(s.modelsConfig, anthReq.Model)
	payload := JetbrainsPayload{
		Prompt:  JetBrainsChatPrompt,
		Profile: internalModel,
		Chat:    JetbrainsChat{Messages: jetbrainsMessages},
	}

	// 只有当有数据时才设置 Parameters
	if len(data) > 0 {
		payload.Parameters = &JetbrainsParameters{Data: data}
	}

	payloadBytes, err := marshalJSON(payload)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to marshal request: %w", err)
	}

	logger.Debug("=== JetBrains API Request Debug (Direct) ===")
	logger.Debug("Model: %s -> %s", anthReq.Model, internalModel)
	logger.Debug("Messages converted: %d", len(jetbrainsMessages))
	logger.Debug("Tools attached: %d", len(data))
	logger.Debug("Payload size: %d bytes", len(payloadBytes))
	logger.Debug("=== Complete Upstream Payload ===")
	logger.Debug("%s", string(payloadBytes))
	logger.Debug("=== End Upstream Payload ===")
	logger.Debug("=== End Debug ===")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, JetBrainsChatEndpoint, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set(HeaderAccept, ContentTypeEventStream)
	req.Header.Set(HeaderContentType, ContentTypeJSON)
	req.Header.Set(HeaderCacheControl, CacheControlNoCache)
	setJetbrainsHeaders(req, account.JWT)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to make request: %w", err)
	}

	logger.Debug("JetBrains API Response Status: %d", resp.StatusCode)

	if resp.StatusCode == JetBrainsStatusQuotaExhausted {
		logger.Warn("Account %s has no quota (received 477)", getTokenDisplayName(account))
		markAccountNoQuota(account)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBodySize))
		_ = resp.Body.Close() // 关闭原始 body，避免资源泄漏
		errorMsg := string(body)
		logger.Error("JetBrains API Error: Status %d, Body: %s", resp.StatusCode, errorMsg)

		// 返回 nil response，因为调用者在错误情况下不会使用响应
		return nil, resp.StatusCode, fmt.Errorf("JetBrains API error: %d - %s", resp.StatusCode, errorMsg)
	}

	return resp, http.StatusOK, nil
}
