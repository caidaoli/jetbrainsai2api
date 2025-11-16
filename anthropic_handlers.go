package main

import (
	"bytes"
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

	// 记录性能指标开始
	defer func() {
		duration := time.Since(startTime)
		RecordHTTPRequest(duration)
	}()

	var anthReq AnthropicMessagesRequest
	if err := c.ShouldBindJSON(&anthReq); err != nil {
		recordFailureWithTimer(startTime, "", "")
		RecordHTTPError()
		respondWithAnthropicError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
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
		recordFailureWithTimer(startTime, anthReq.Model, "")
		respondWithAnthropicError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}

	if anthReq.MaxTokens <= 0 {
		recordFailureWithTimer(startTime, anthReq.Model, "")
		respondWithAnthropicError(c, http.StatusBadRequest, "invalid_request_error", "max_tokens must be positive")
		return
	}

	if len(anthReq.Messages) == 0 {
		recordFailureWithTimer(startTime, anthReq.Model, "")
		respondWithAnthropicError(c, http.StatusBadRequest, "invalid_request_error", "messages cannot be empty")
		return
	}

	// 检查模型是否存在
	modelConfig := getModelItem(s.modelsData, anthReq.Model)
	if modelConfig == nil {
		recordFailureWithTimer(startTime, anthReq.Model, "")
		respondWithAnthropicError(c, http.StatusNotFound, "model_not_found_error",
			fmt.Sprintf("Model %s not found", anthReq.Model))
		return
	}

	// 使用 AccountManager 获取账户
	account, err := s.accountManager.AcquireAccount(c.Request.Context())
	if err != nil {
		recordFailureWithTimer(startTime, anthReq.Model, "")
		respondWithAnthropicError(c, http.StatusTooManyRequests, "rate_limit_error", err.Error())
		return
	}
	defer s.accountManager.ReleaseAccount(account)

	accountIdentifier := getTokenDisplayName(account)

	// KISS: 直接转换 Anthropic → JetBrains，消除中间层
	jetbrainsMessages := anthropicToJetbrainsMessages(anthReq.Messages)

	// 处理 system 字段 - Anthropic 的 system 是单独字段，需要转换为 system_message
	if anthReq.System != "" {
		systemMsg := JetbrainsMessage{
			Type:    "system_message",
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
			recordFailureWithTimer(startTime, anthReq.Model, accountIdentifier)
			respondWithAnthropicError(c, http.StatusInternalServerError, "api_error", "Failed to marshal tools")
			return
		}
		data = append(data, JetbrainsData{Type: "json", Value: string(toolsJSON)})
	}

	// 直接调用 JetBrains API
	jetbrainsResponse, statusCode, err := s.callJetbrainsAPIDirect(&anthReq, jetbrainsMessages, data, account, startTime, accountIdentifier)
	if err != nil {
		recordFailureWithTimer(startTime, anthReq.Model, accountIdentifier)
		respondWithAnthropicError(c, statusCode, "api_error", err.Error())
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

// respondWithAnthropicError 返回 Anthropic 格式的错误响应
// SRP: 专门处理错误响应格式
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

// callJetbrainsAPI 调用 JetBrains API（Server 方法）
func (s *Server) callJetbrainsAPI(openAIReq *ChatCompletionRequest, account *JetbrainsAccount, startTime time.Time, accountIdentifier string) (*http.Response, int, error) {
	// 这里复用现有的 chatCompletions 中的 JetBrains API 调用逻辑
	// 为了保持 DRY 原则，我们需要重构现有代码以提取公共部分

	// Convert OpenAI format to JetBrains format with caching
	messagesCacheKey := generateMessagesCacheKey(openAIReq.Messages)
	jetbrainsMessagesAny, found := messageConversionCache.Get(messagesCacheKey)
	var jetbrainsMessages []JetbrainsMessage
	if found {
		jetbrainsMessages = jetbrainsMessagesAny.([]JetbrainsMessage)
		RecordCacheHit()
	} else {
		jetbrainsMessages = openAIToJetbrainsMessages(openAIReq.Messages)
		messageConversionCache.Set(messagesCacheKey, jetbrainsMessages, 10*time.Minute)
		RecordCacheMiss()
	}

	// CRITICAL FIX: Force tool usage when tools are provided
	if len(openAIReq.Tools) > 0 {
		if openAIReq.ToolChoice == nil {
			openAIReq.ToolChoice = "any"
			Debug("FORCING tool_choice to 'any' for tool usage guarantee")
		}
	}

	var data []JetbrainsData
	if len(openAIReq.Tools) > 0 {
		toolsCacheKey := generateToolsCacheKey(openAIReq.Tools)
		validatedToolsAny, found := toolsValidationCache.Get(toolsCacheKey)
		var validatedTools []Tool
		if found {
			validatedTools = validatedToolsAny.([]Tool)
			RecordCacheHit()
		} else {
			validationStart := time.Now()
			var validationErr error
			validatedTools, validationErr = validateAndTransformTools(openAIReq.Tools)
			validationDuration := time.Since(validationStart)
			RecordToolValidation(validationDuration)

			if validationErr != nil {
				return nil, http.StatusBadRequest, fmt.Errorf("tool validation failed: %w", validationErr)
			}
			toolsValidationCache.Set(toolsCacheKey, validatedTools, 30*time.Minute)
			RecordCacheMiss()
		}

		if len(validatedTools) > 0 {
			data = append(data, JetbrainsData{Type: "json", FQDN: "llm.parameters.tools"})
			// 转换为JetBrains格式
			var jetbrainsTools []JetbrainsToolDefinition
			for _, tool := range validatedTools {
				jetbrainsTools = append(jetbrainsTools, JetbrainsToolDefinition{
					Name:        tool.Function.Name,
					Description: tool.Function.Description,
					Parameters: JetbrainsToolParametersWrapper{
						Schema: tool.Function.Parameters,
					},
				})
			}
			toolsJSON, marshalErr := marshalJSON(jetbrainsTools)
			if marshalErr != nil {
				return nil, http.StatusInternalServerError, fmt.Errorf("failed to marshal tools")
			}
			Debug("Transformed tools for JetBrains API: %s", string(toolsJSON))
			data = append(data, JetbrainsData{Type: "json", Value: string(toolsJSON)})
			// 添加modified字段，模拟preview.json的格式
			modifiedTime := time.Now().UnixMilli()
			if len(data) > 1 {
				// 为第二个data项（工具定义）添加modified字段
				lastIndex := len(data) - 1
				modifiedData := JetbrainsData{
					Type:     data[lastIndex].Type,
					Value:    data[lastIndex].Value,
					Modified: modifiedTime,
				}
				data[lastIndex] = modifiedData
			}
			if shouldForceToolUse(*openAIReq) {
				jetbrainsMessages = openAIToJetbrainsMessages(openAIReq.Messages)
				Debug("Using original messages for tool usage")
			}
		}
	}
	if data == nil {
		data = []JetbrainsData{}
	}

	internalModel := getInternalModelName(s.modelsConfig, openAIReq.Model)
	payload := JetbrainsPayload{
		Prompt:  "ij.chat.request.new-chat-on-start",
		Profile: internalModel,
		Chat:    JetbrainsChat{Messages: jetbrainsMessages},
	}

	// 只有当有数据时才设置 Parameters
	if len(data) > 0 {
		payload.Parameters = &JetbrainsParameters{Data: data}
	}

	payloadBytes, err := marshalJSON(payload)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to marshal request")
	}

	Debug("=== JetBrains API Request Debug (Anthropic) ===")
	Debug("Model: %s -> %s", openAIReq.Model, internalModel)
	Debug("Payload size: %d bytes", len(payloadBytes))
	Debug("Complete payload: %s", string(payloadBytes))
	Debug("=== End Debug ===")

	req, err := createJetbrainsStreamRequest(payloadBytes, account.JWT)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to create request")
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to make request")
	}

	Debug("JetBrains API Response Status: %d", resp.StatusCode)

	if resp.StatusCode == 477 {
		Warn("Account %s has no quota (received 477)", getTokenDisplayName(account))
		account.HasQuota = false
		account.LastQuotaCheck = float64(time.Now().Unix())
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		errorMsg := string(body)
		Error("JetBrains API Error: Status %d, Body: %s", resp.StatusCode, errorMsg)

		// 重新创建 response body reader，以便后续处理
		resp.Body = io.NopCloser(bytes.NewReader(body))

		return resp, resp.StatusCode, fmt.Errorf("JetBrains API error: %d", resp.StatusCode)
	}

	return resp, http.StatusOK, nil
}
