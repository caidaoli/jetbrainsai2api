package server

import (
	"io"
	"net/http"
	"time"

	"jetbrainsai2api/internal/convert"
	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/util"

	"github.com/gin-gonic/gin"
)

func (s *Server) anthropicMessages(c *gin.Context) {
	startTime := time.Now()
	logger := s.config.Logger

	var acct *core.JetbrainsAccount
	var resp *http.Response
	defer withPanicRecoveryWithMetrics(c, s.metricsService, startTime, &resp, core.APIFormatAnthropic, logger)()
	defer trackPerformanceWithMetrics(s.metricsService, startTime)()

	var anthReq core.AnthropicMessagesRequest
	if err := c.ShouldBindJSON(&anthReq); err != nil {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, "", "")
		respondWithAnthropicError(c, http.StatusBadRequest, core.AnthropicErrorInvalidRequest, "invalid request body")
		return
	}

	logger.Debug("Anthropic request: model=%s, messages=%d, tools=%d, stream=%v",
		anthReq.Model, len(anthReq.Messages), len(anthReq.Tools), anthReq.Stream)

	if anthReq.Model == "" {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, anthReq.Model, "")
		respondWithAnthropicError(c, http.StatusBadRequest, core.AnthropicErrorInvalidRequest, "model is required")
		return
	}

	if anthReq.MaxTokens <= 0 {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, anthReq.Model, "")
		respondWithAnthropicError(c, http.StatusBadRequest, core.AnthropicErrorInvalidRequest, "max_tokens must be positive")
		return
	}

	if len(anthReq.Messages) == 0 {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, anthReq.Model, "")
		respondWithAnthropicError(c, http.StatusBadRequest, core.AnthropicErrorInvalidRequest, "messages cannot be empty")
		return
	}

	modelConfig := getModelConfigOrErrorWithMetrics(c, s.metricsService, s.modelsData, anthReq.Model, startTime, core.APIFormatAnthropic)
	if modelConfig == nil {
		return
	}

	var err error
	acct, err = s.accountManager.AcquireAccount(c.Request.Context())
	if err != nil {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, anthReq.Model, "")
		respondWithAnthropicError(c, http.StatusTooManyRequests, core.AnthropicErrorRateLimit, "no available accounts")
		return
	}
	defer s.accountManager.ReleaseAccount(acct)

	accountIdentifier := util.GetTokenDisplayName(acct)

	jetbrainsMessages := convert.AnthropicToJetbrainsMessages(anthReq.Messages)

	if anthReq.System != "" {
		systemMsg := core.JetbrainsMessage{
			Type:    core.JetBrainsMessageTypeSystem,
			Content: string(anthReq.System),
		}
		jetbrainsMessages = append([]core.JetbrainsMessage{systemMsg}, jetbrainsMessages...)
	}

	var data []core.JetbrainsData
	if len(anthReq.Tools) > 0 {
		jetbrainsTools := convert.AnthropicToJetbrainsTools(anthReq.Tools)

		toolsJSON, marshalErr := util.MarshalJSON(jetbrainsTools)
		if marshalErr != nil {
			recordRequestResultWithMetrics(s.metricsService, false, startTime, anthReq.Model, accountIdentifier)
			respondWithAnthropicError(c, http.StatusInternalServerError, core.AnthropicErrorAPI, "failed to marshal tools")
			return
		}
		data = append(data,
			core.JetbrainsData{Type: "json", FQDN: "llm.parameters.tools"},
			core.JetbrainsData{Type: "json", Value: string(toolsJSON)},
		)
	}

	payloadBytes, err := s.requestProcessor.BuildPayloadDirect(anthReq.Model, jetbrainsMessages, data)
	if err != nil {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, anthReq.Model, accountIdentifier)
		logger.Error("Failed to build payload: %v", err)
		respondWithAnthropicError(c, http.StatusInternalServerError, core.AnthropicErrorAPI, "internal server error")
		return
	}

	//nolint:bodyclose // resp.Body closed below via defer
	resp, err = s.requestProcessor.SendUpstreamRequest(c.Request.Context(), payloadBytes, acct)
	if err != nil {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, anthReq.Model, accountIdentifier)
		logger.Error("Upstream request failed: %v", err)
		respondWithAnthropicError(c, http.StatusInternalServerError, core.AnthropicErrorAPI, "upstream service error")
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, core.MaxResponseBodySize))
		logger.Error("JetBrains API Error: status=%d, body=%s", resp.StatusCode, string(body))
		recordRequestResultWithMetrics(s.metricsService, false, startTime, anthReq.Model, accountIdentifier)
		respondWithAnthropicError(c, resp.StatusCode, core.AnthropicErrorAPI, "upstream service error")
		return
	}

	isStream := anthReq.Stream != nil && *anthReq.Stream
	if isStream {
		handleAnthropicStreamingResponseWithMetrics(c, resp, &anthReq, startTime, accountIdentifier, s.metricsService, logger)
	} else {
		handleAnthropicNonStreamingResponseWithMetrics(c, resp, &anthReq, startTime, accountIdentifier, s.metricsService, logger)
	}
}
