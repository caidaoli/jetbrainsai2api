package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// authenticateClient 客户端认证中间件（Server 方法）
func (s *Server) authenticateClient(c *gin.Context) {
	if len(s.validClientKeys) == 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Service unavailable: no client API keys configured"})
		c.Abort()
		return
	}

	authHeader := c.GetHeader("Authorization")
	apiKey := c.GetHeader("x-api-key")

	// Check x-api-key first
	if apiKey != "" {
		if s.validClientKeys[apiKey] {
			return
		}
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid client API key (x-api-key)"})
		c.Abort()
		return
	}

	// Check Authorization header
	if authHeader != "" {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if s.validClientKeys[token] {
			return
		}
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid client API key (Bearer token)"})
		c.Abort()
		return
	}

	c.JSON(http.StatusUnauthorized, gin.H{"error": "API key required in Authorization header (Bearer) or x-api-key header"})
	c.Abort()
}

// listModels 列出可用模型（Server 方法）
func (s *Server) listModels(c *gin.Context) {
	modelList := ModelList{
		Object: "list",
		Data:   s.modelsData.Data,
	}
	c.JSON(http.StatusOK, modelList)
}

// chatCompletions handles chat completion requests（Server 方法）
func (s *Server) chatCompletions(c *gin.Context) {
	startTime := time.Now()

	// Panic 恢复机制，确保资源正确释放
	var account *JetbrainsAccount
	var resp *http.Response
	defer func() {
		if r := recover(); r != nil {
			Error("Panic in chatCompletions: %v", r)
			// 确保资源释放
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
			if account != nil {
				s.accountManager.ReleaseAccount(account)
			}
			recordFailureWithTimer(startTime, "", "")
			RecordHTTPError()
			respondWithError(c, http.StatusInternalServerError, "internal server error")
		}
	}()

	// 记录性能指标开始
	defer func() {
		duration := time.Since(startTime)
		RecordHTTPRequest(duration)
	}()

	var request ChatCompletionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		recordFailureWithTimer(startTime, "", "")
		RecordHTTPError()
		respondWithError(c, http.StatusBadRequest, err.Error())
		return
	}

	modelConfig := getModelItem(s.modelsData, request.Model)
	if modelConfig == nil {
		recordFailureWithTimer(startTime, request.Model, "")
		respondWithError(c, http.StatusNotFound, fmt.Sprintf("Model %s not found", request.Model))
		return
	}

	// 使用 AccountManager 获取账户
	var err error
	account, err = s.accountManager.AcquireAccount(c.Request.Context())
	if err != nil {
		recordFailureWithTimer(startTime, request.Model, "")
		respondWithError(c, http.StatusTooManyRequests, err.Error())
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
		recordFailureWithTimer(startTime, request.Model, accountIdentifier)
		RecordHTTPError()
		respondWithError(c, http.StatusBadRequest, toolsResult.Error.Error())
		return
	}

	// 步骤 3: 构建 JetBrains API payload
	// SRP: 职责分离 - payload 构建由 RequestProcessor 负责
	payloadBytes, err := s.requestProcessor.BuildJetbrainsPayload(&request, jetbrainsMessages, toolsResult.Data)
	if err != nil {
		recordFailureWithTimer(startTime, request.Model, accountIdentifier)
		respondWithError(c, http.StatusInternalServerError, err.Error())
		return
	}

	// 步骤 4: 发送上游请求
	// SRP: 职责分离 - HTTP 请求发送由 RequestProcessor 负责
	resp, err = s.requestProcessor.SendUpstreamRequest(c.Request.Context(), payloadBytes, account)
	if err != nil {
		recordFailureWithTimer(startTime, request.Model, accountIdentifier)
		respondWithError(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer resp.Body.Close()

	// 步骤 5: 检查响应状态
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		errorMsg := string(body)
		Error("JetBrains API Error: Status %d, Body: %s", resp.StatusCode, errorMsg)
		recordFailureWithTimer(startTime, request.Model, accountIdentifier)
		c.JSON(resp.StatusCode, gin.H{"error": errorMsg})
		return
	}

	if request.Stream {
		handleStreamingResponse(c, resp, request, startTime, accountIdentifier)
	} else {
		handleNonStreamingResponse(c, resp, request, startTime, accountIdentifier)
	}
}
