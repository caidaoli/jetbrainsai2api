package server

import (
	"io"
	"net/http"
	"time"

	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/util"

	"github.com/gin-gonic/gin"
)

func (s *Server) listModels(c *gin.Context) {
	modelList := core.ModelList{
		Object: core.ModelListObjectType,
		Data:   s.modelsData.Data,
	}
	c.JSON(http.StatusOK, modelList)
}

func (s *Server) chatCompletions(c *gin.Context) {
	startTime := time.Now()

	var account *core.JetbrainsAccount
	var resp *http.Response
	defer withPanicRecoveryWithMetrics(c, s.metricsService, startTime, &resp, core.APIFormatOpenAI, s.config.Logger)()
	defer trackPerformanceWithMetrics(s.metricsService, startTime)()

	var request core.ChatCompletionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, "", "")
		respondWithOpenAIError(c, http.StatusBadRequest, err.Error())
		return
	}

	modelConfig := getModelConfigOrErrorWithMetrics(c, s.metricsService, s.modelsData, request.Model, startTime, core.APIFormatOpenAI)
	if modelConfig == nil {
		return
	}

	var err error
	account, err = s.accountManager.AcquireAccount(c.Request.Context())
	if err != nil {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, request.Model, "")
		respondWithOpenAIError(c, http.StatusTooManyRequests, err.Error())
		return
	}
	defer s.accountManager.ReleaseAccount(account)

	accountIdentifier := util.GetTokenDisplayName(account)

	messagesResult := s.requestProcessor.ProcessMessages(request.Messages)
	jetbrainsMessages := messagesResult.JetbrainsMessages

	if len(request.Tools) > 0 && request.ToolChoice == nil {
		request.ToolChoice = core.ToolChoiceAny
		s.config.Logger.Debug("Setting tool_choice to '%s' for tool usage guarantee", core.ToolChoiceAny)
	}

	toolsResult := s.requestProcessor.ProcessTools(&request)
	if toolsResult.Error != nil {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, request.Model, accountIdentifier)
		respondWithOpenAIError(c, http.StatusBadRequest, toolsResult.Error.Error())
		return
	}

	payloadBytes, err := s.requestProcessor.BuildJetbrainsPayload(&request, jetbrainsMessages, toolsResult.Data)
	if err != nil {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, request.Model, accountIdentifier)
		respondWithOpenAIError(c, http.StatusInternalServerError, err.Error())
		return
	}

	//nolint:bodyclose // resp.Body closed below via defer
	resp, err = s.requestProcessor.SendUpstreamRequest(c.Request.Context(), payloadBytes, account)
	if err != nil {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, request.Model, accountIdentifier)
		respondWithOpenAIError(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, core.MaxResponseBodySize))
		errorMsg := string(body)
		s.config.Logger.Error("JetBrains API Error: Status %d, Body: %s", resp.StatusCode, errorMsg)
		recordRequestResultWithMetrics(s.metricsService, false, startTime, request.Model, accountIdentifier)
		c.JSON(resp.StatusCode, gin.H{"error": errorMsg})
		return
	}

	if request.Stream {
		handleStreamingResponseWithMetrics(c, resp, request, startTime, accountIdentifier, s.metricsService, s.config.Logger)
	} else {
		handleNonStreamingResponseWithMetrics(c, resp, request, startTime, accountIdentifier, s.metricsService, s.config.Logger)
	}
}
