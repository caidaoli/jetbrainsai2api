package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// anthropicMessages 处理 Anthropic Messages API 请求（Server 方法）
// SRP: 专门处理 Anthropic 协议的单一职责
func (s *Server) anthropicMessages(c *gin.Context) {
	startTime := time.Now()

	// Panic 恢复机制和性能追踪
	defer trackPerformance(startTime)()

	var anthReq AnthropicMessagesRequest
	if err := c.ShouldBindJSON(&anthReq); err != nil {
		recordRequestResult(false, startTime, "", "")
		respondWithAnthropicError(c, http.StatusBadRequest, AnthropicErrorInvalidRequest, err.Error())
		return
	}

	Debug("Received Anthropic Messages request: model=%s, max_tokens=%d, messages=%d",
		anthReq.Model, anthReq.MaxTokens, len(anthReq.Messages))

	// 记录完整的客户端请求详情 (debug模式下)
	if requestBytes, err := marshalJSON(&anthReq); err == nil {
		Debug("=== Client Request Debug (Anthropic Messages) ===")
		Debug("Request size: %d bytes", len(requestBytes))
		Debug("Complete request payload: %s", string(requestBytes))
		Debug("=== End Client Request Debug ===")
	}

	// 验证必填字段 (KISS: 简单验证逻辑)
	if anthReq.Model == "" {
		recordRequestResult(false, startTime, anthReq.Model, "")
		respondWithAnthropicError(c, http.StatusBadRequest, AnthropicErrorInvalidRequest, "model is required")
		return
	}

	if anthReq.MaxTokens <= 0 {
		recordRequestResult(false, startTime, anthReq.Model, "")
		respondWithAnthropicError(c, http.StatusBadRequest, AnthropicErrorInvalidRequest, "max_tokens must be positive")
		return
	}

	if len(anthReq.Messages) == 0 {
		recordRequestResult(false, startTime, anthReq.Model, "")
		respondWithAnthropicError(c, http.StatusBadRequest, AnthropicErrorInvalidRequest, "messages cannot be empty")
		return
	}

	// 检查模型是否存在
	modelConfig := getModelConfigOrError(c, s.modelsData, anthReq.Model, startTime, APIFormatAnthropic)
	if modelConfig == nil {
		return
	}

	// 使用 AccountManager 获取账户
	account, err := s.accountManager.AcquireAccount(c.Request.Context())
	if err != nil {
		recordRequestResult(false, startTime, anthReq.Model, "")
		respondWithAnthropicError(c, http.StatusTooManyRequests, AnthropicErrorRateLimit, err.Error())
		return
	}
	defer s.accountManager.ReleaseAccount(account)

	accountIdentifier := getTokenDisplayName(account)

	// KISS: 直接转换 Anthropic → JetBrains，消除中间层
	jetbrainsMessages := anthropicToJetbrainsMessages(anthReq.Messages)

	// 处理 system 字段 - Anthropic 的 system 是单独字段，需要转换为 system_message
	if anthReq.System != "" {
		systemMsg := JetbrainsMessage{
			Type:    JetBrainsMessageTypeSystem,
			Content: string(anthReq.System),
		}
		// 将 system_message 插入到消息数组开头
		jetbrainsMessages = append([]JetbrainsMessage{systemMsg}, jetbrainsMessages...)
		Debug("Added system_message from Anthropic system field")
	}

	// 处理工具定义
	var data []JetbrainsData
	if len(anthReq.Tools) > 0 {
		jetbrainsTools := anthropicToJetbrainsTools(anthReq.Tools)
		data = append(data, JetbrainsData{Type: "json", FQDN: "llm.parameters.tools"})

		toolsJSON, marshalErr := marshalJSON(jetbrainsTools)
		if marshalErr != nil {
			recordRequestResult(false, startTime, anthReq.Model, accountIdentifier)
			respondWithAnthropicError(c, http.StatusInternalServerError, AnthropicErrorAPI, "Failed to marshal tools")
			return
		}
		data = append(data, JetbrainsData{Type: "json", Value: string(toolsJSON)})
	}

	// 直接调用 JetBrains API
	jetbrainsResponse, statusCode, err := s.callJetbrainsAPIDirect(&anthReq, jetbrainsMessages, data, account, startTime, accountIdentifier)
	if err != nil {
		recordRequestResult(false, startTime, anthReq.Model, accountIdentifier)
		respondWithAnthropicError(c, statusCode, AnthropicErrorAPI, err.Error())
		return
	}

	// 根据是否流式处理响应
	isStream := anthReq.Stream != nil && *anthReq.Stream
	if isStream {
		handleAnthropicStreamingResponse(c, jetbrainsResponse, &anthReq, startTime, accountIdentifier)
	} else {
		handleAnthropicNonStreamingResponse(c, jetbrainsResponse, &anthReq, startTime, accountIdentifier)
	}
}
