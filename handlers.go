package main

import (
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// listModels 列出可用模型（Server 方法）
func (s *Server) listModels(c *gin.Context) {
	modelList := ModelList{
		Object: ModelListObjectType,
		Data:   s.modelsData.Data,
	}
	c.JSON(http.StatusOK, modelList)
}

// chatCompletions handles chat completion requests（Server 方法）
func (s *Server) chatCompletions(c *gin.Context) {
	startTime := time.Now()

	// Panic 恢复机制和性能追踪
	var account *JetbrainsAccount
	var resp *http.Response
	defer withPanicRecovery(c, startTime, &account, &resp, s.accountManager, APIFormatOpenAI)()
	defer trackPerformance(startTime)()

	var request ChatCompletionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		recordRequestResult(false, startTime, "", "")
		respondWithOpenAIError(c, http.StatusBadRequest, err.Error())
		return
	}

	modelConfig := getModelConfigOrError(c, s.modelsData, request.Model, startTime, APIFormatOpenAI)
	if modelConfig == nil {
		return
	}

	// 使用 AccountManager 获取账户
	var err error
	account, err = s.accountManager.AcquireAccount(c.Request.Context())
	if err != nil {
		recordRequestResult(false, startTime, request.Model, "")
		respondWithOpenAIError(c, http.StatusTooManyRequests, err.Error())
		return
	}
	defer s.accountManager.ReleaseAccount(account)

	accountIdentifier := getTokenDisplayName(account)

	// 步骤 1: 处理消息转换（使用缓存）
	// SRP: 职责分离 - 消息处理由 RequestProcessor 负责
	messagesResult := s.requestProcessor.ProcessMessages(request.Messages)
	jetbrainsMessages := messagesResult.JetbrainsMessages

	// 步骤 2: 处理工具验证和转换（使用缓存）
	// SRP: 职责分离 - 工具处理由 RequestProcessor 负责
	toolsResult := s.requestProcessor.ProcessTools(&request)
	if toolsResult.Error != nil {
		recordRequestResult(false, startTime, request.Model, accountIdentifier)
		respondWithOpenAIError(c, http.StatusBadRequest, toolsResult.Error.Error())
		return
	}

	// 步骤 3: 构建 JetBrains API payload
	// SRP: 职责分离 - payload 构建由 RequestProcessor 负责
	payloadBytes, err := s.requestProcessor.BuildJetbrainsPayload(&request, jetbrainsMessages, toolsResult.Data)
	if err != nil {
		recordRequestResult(false, startTime, request.Model, accountIdentifier)
		respondWithOpenAIError(c, http.StatusInternalServerError, err.Error())
		return
	}

	// 步骤 4: 发送上游请求
	// SRP: 职责分离 - HTTP 请求发送由 RequestProcessor 负责
	resp, err = s.requestProcessor.SendUpstreamRequest(c.Request.Context(), payloadBytes, account)
	if err != nil {
		recordRequestResult(false, startTime, request.Model, accountIdentifier)
		respondWithOpenAIError(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer resp.Body.Close()

	// 步骤 5: 检查响应状态
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		errorMsg := string(body)
		Error("JetBrains API Error: Status %d, Body: %s", resp.StatusCode, errorMsg)
		recordRequestResult(false, startTime, request.Model, accountIdentifier)
		c.JSON(resp.StatusCode, gin.H{"error": errorMsg})
		return
	}

	if request.Stream {
		handleStreamingResponse(c, resp, request, startTime, accountIdentifier)
	} else {
		handleNonStreamingResponse(c, resp, request, startTime, accountIdentifier)
	}
}
