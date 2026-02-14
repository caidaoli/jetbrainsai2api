package server

import (
	"net/http"
	"time"

	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/util"

	"github.com/gin-gonic/gin"
)

func (s *Server) listModels(c *gin.Context) {
	c.JSON(http.StatusOK, s.modelsData)
}

func (s *Server) chatCompletions(c *gin.Context) {
	startTime := time.Now()

	var resp *http.Response
	defer withPanicRecoveryWithMetrics(c, s.metricsService, startTime, &resp, core.APIFormatOpenAI, s.config.Logger)()
	defer trackPerformanceWithMetrics(s.metricsService, startTime)()

	var request core.ChatCompletionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, "", "")
		respondWithOpenAIError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	modelConfig := getModelConfigOrErrorWithMetrics(c, s.metricsService, s.modelsData, request.Model, startTime, core.APIFormatOpenAI)
	if modelConfig == nil {
		return
	}

	// Phase 1: Build payload — no account needed
	messagesResult := s.requestProcessor.ProcessMessages(request.Messages)
	jetbrainsMessages := messagesResult.JetbrainsMessages

	if len(request.Tools) > 0 && request.ToolChoice == nil {
		request.ToolChoice = core.ToolChoiceAny
		s.config.Logger.Debug("Setting tool_choice to '%s' for tool usage guarantee", core.ToolChoiceAny)
	}

	toolsResult := s.requestProcessor.ProcessTools(&request)
	if toolsResult.Error != nil {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, request.Model, "")
		respondWithOpenAIError(c, http.StatusBadRequest, "invalid tool parameters")
		return
	}

	payloadBytes, err := s.requestProcessor.BuildJetbrainsPayload(&request, jetbrainsMessages, toolsResult.Data)
	if err != nil {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, request.Model, "")
		s.config.Logger.Error("Failed to build payload: %v", err)
		respondWithOpenAIError(c, http.StatusInternalServerError, "internal server error")
		return
	}

	// Phase 2: Send with retry on 477 quota exhaustion
	var account *core.JetbrainsAccount
	//nolint:bodyclose // resp.Body closed below via defer
	resp, account, err = s.sendWithRetry(c.Request.Context(), payloadBytes, s.config.Logger)
	if err != nil {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, request.Model, "")
		respondWithOpenAIError(c, http.StatusTooManyRequests, "no available accounts with quota")
		return
	}
	defer s.accountManager.ReleaseAccount(account)
	defer func() { _ = resp.Body.Close() }()

	accountIdentifier := util.GetTokenDisplayName(account)

	// Phase 3: Handle upstream response
	if resp.StatusCode != http.StatusOK {
		errMsg := extractUpstreamErrorMessage(resp, s.config.Logger)
		recordRequestResultWithMetrics(s.metricsService, false, startTime, request.Model, accountIdentifier)
		respondWithOpenAIError(c, resp.StatusCode, errMsg)
		return
	}

	if request.Stream {
		handleStreamingResponseWithMetrics(c, resp, request, startTime, accountIdentifier, s.metricsService, s.config.Logger)
	} else {
		handleNonStreamingResponseWithMetrics(c, resp, request, startTime, accountIdentifier, s.metricsService, s.config.Logger)
	}
}
