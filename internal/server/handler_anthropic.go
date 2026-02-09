package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"jetbrainsai2api/internal/account"
	"jetbrainsai2api/internal/convert"
	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/process"
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
		respondWithAnthropicError(c, http.StatusBadRequest, core.AnthropicErrorInvalidRequest, err.Error())
		return
	}

	logger.Debug("Received Anthropic Messages request: model=%s, max_tokens=%d, messages=%d",
		anthReq.Model, anthReq.MaxTokens, len(anthReq.Messages))

	if requestBytes, err := util.MarshalJSON(&anthReq); err == nil {
		logger.Debug("=== Client Request Debug (Anthropic Messages) ===")
		logger.Debug("Request size: %d bytes", len(requestBytes))
		logger.Debug("Complete request payload: %s", string(requestBytes))
		logger.Debug("=== End Client Request Debug ===")
	}

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
		respondWithAnthropicError(c, http.StatusTooManyRequests, core.AnthropicErrorRateLimit, err.Error())
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
		logger.Debug("Added system_message from Anthropic system field")
	}

	var data []core.JetbrainsData
	if len(anthReq.Tools) > 0 {
		jetbrainsTools := convert.AnthropicToJetbrainsTools(anthReq.Tools)
		data = append(data, core.JetbrainsData{Type: "json", FQDN: "llm.parameters.tools"})

		toolsJSON, marshalErr := util.MarshalJSON(jetbrainsTools)
		if marshalErr != nil {
			recordRequestResultWithMetrics(s.metricsService, false, startTime, anthReq.Model, accountIdentifier)
			respondWithAnthropicError(c, http.StatusInternalServerError, core.AnthropicErrorAPI, "Failed to marshal tools")
			return
		}
		data = append(data, core.JetbrainsData{Type: "json", Value: string(toolsJSON)})
	}

	//nolint:bodyclose // resp.Body closed in response handler functions
	resp, statusCode, err := s.callJetbrainsAPIDirect(c.Request.Context(), &anthReq, jetbrainsMessages, data, acct)
	if err != nil {
		recordRequestResultWithMetrics(s.metricsService, false, startTime, anthReq.Model, accountIdentifier)
		respondWithAnthropicError(c, statusCode, core.AnthropicErrorAPI, err.Error())
		return
	}

	isStream := anthReq.Stream != nil && *anthReq.Stream
	if isStream {
		handleAnthropicStreamingResponseWithMetrics(c, resp, &anthReq, startTime, accountIdentifier, s.metricsService, logger)
	} else {
		handleAnthropicNonStreamingResponseWithMetrics(c, resp, &anthReq, startTime, accountIdentifier, s.metricsService, logger)
	}
}

func (s *Server) callJetbrainsAPIDirect(ctx context.Context, anthReq *core.AnthropicMessagesRequest, jetbrainsMessages []core.JetbrainsMessage, data []core.JetbrainsData, acct *core.JetbrainsAccount) (*http.Response, int, error) {
	logger := s.config.Logger
	internalModel := process.GetInternalModelName(s.modelsConfig, anthReq.Model)
	payload := core.JetbrainsPayload{
		Prompt:  core.JetBrainsChatPrompt,
		Profile: internalModel,
		Chat:    core.JetbrainsChat{Messages: jetbrainsMessages},
	}

	if len(data) > 0 {
		payload.Parameters = &core.JetbrainsParameters{Data: data}
	}

	payloadBytes, err := util.MarshalJSON(payload)
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, core.JetBrainsChatEndpoint, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set(core.HeaderAccept, core.ContentTypeEventStream)
	req.Header.Set(core.HeaderContentType, core.ContentTypeJSON)
	req.Header.Set(core.HeaderCacheControl, core.CacheControlNoCache)
	account.SetJetbrainsHeaders(req, acct.JWT)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to make request: %w", err)
	}

	logger.Debug("JetBrains API Response Status: %d", resp.StatusCode)

	if resp.StatusCode == core.JetBrainsStatusQuotaExhausted {
		logger.Warn("Account %s has no quota (received 477)", util.GetTokenDisplayName(acct))
		account.MarkAccountNoQuota(acct)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, core.MaxResponseBodySize))
		_ = resp.Body.Close()
		errorMsg := string(body)
		logger.Error("JetBrains API Error: Status %d, Body: %s", resp.StatusCode, errorMsg)

		return nil, resp.StatusCode, fmt.Errorf("JetBrains API error: %d - %s", resp.StatusCode, errorMsg)
	}

	return resp, http.StatusOK, nil
}
