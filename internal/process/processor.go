package process

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"jetbrainsai2api/internal/account"
	"jetbrainsai2api/internal/cache"
	"jetbrainsai2api/internal/convert"
	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/util"
	"jetbrainsai2api/internal/validate"
)

// RequestProcessor handles request processing
type RequestProcessor struct {
	modelsConfig core.ModelsConfig
	httpClient   *http.Client
	cache        core.Cache
	metrics      core.MetricsCollector
	logger       core.Logger
}

// NewRequestProcessor creates a new request processor
func NewRequestProcessor(modelsConfig core.ModelsConfig, httpClient *http.Client, c core.Cache, metrics core.MetricsCollector, logger core.Logger) *RequestProcessor {
	return &RequestProcessor{
		modelsConfig: modelsConfig,
		httpClient:   httpClient,
		cache:        c,
		metrics:      metrics,
		logger:       logger,
	}
}

// ProcessMessagesResult message processing result
type ProcessMessagesResult struct {
	JetbrainsMessages []core.JetbrainsMessage
	CacheHit          bool
}

// ProcessMessages processes message conversion with cache
func (p *RequestProcessor) ProcessMessages(messages []core.ChatMessage) ProcessMessagesResult {
	cacheKey := cache.GenerateMessagesCacheKey(messages)

	if cachedAny, found := p.cache.Get(cacheKey); found {
		if jetbrainsMessages, ok := cachedAny.([]core.JetbrainsMessage); ok {
			p.metrics.RecordCacheHit()
			return ProcessMessagesResult{
				JetbrainsMessages: jetbrainsMessages,
				CacheHit:          true,
			}
		}
		p.logger.Warn("Cache format mismatch for messages (key: %s), regenerating", cache.TruncateCacheKey(cacheKey, 16))
	}

	p.metrics.RecordCacheMiss()
	jetbrainsMessages := convert.OpenAIToJetbrainsMessages(messages)

	p.cache.Set(cacheKey, jetbrainsMessages, core.MessageConversionCacheTTL)

	return ProcessMessagesResult{
		JetbrainsMessages: jetbrainsMessages,
		CacheHit:          false,
	}
}

// ProcessToolsResult tool processing result
type ProcessToolsResult struct {
	Data          []core.JetbrainsData
	ValidatedDone bool
	Error         error
}

// ProcessTools processes tool validation and conversion
func (p *RequestProcessor) ProcessTools(request *core.ChatCompletionRequest) ProcessToolsResult {
	if len(request.Tools) == 0 {
		return ProcessToolsResult{
			Data:          []core.JetbrainsData{},
			ValidatedDone: true,
		}
	}

	toolsCacheKey := cache.GenerateToolsCacheKey(request.Tools)
	if cachedAny, found := p.cache.Get(toolsCacheKey); found {
		if validatedTools, ok := cachedAny.([]core.Tool); ok {
			p.metrics.RecordCacheHit()
			data, err := p.buildToolsData(validatedTools)
			if err != nil {
				return ProcessToolsResult{Error: err}
			}
			return ProcessToolsResult{
				Data:          data,
				ValidatedDone: true,
			}
		}
		p.logger.Warn("Cache format mismatch for tools (key: %s), revalidating", cache.TruncateCacheKey(toolsCacheKey, 16))
	}

	p.metrics.RecordCacheMiss()
	validationStart := time.Now()
	validatedTools, err := validate.ValidateAndTransformTools(request.Tools, p.logger)
	validationDuration := time.Since(validationStart)
	p.metrics.RecordToolValidation(validationDuration)

	if err != nil {
		return ProcessToolsResult{
			Error: fmt.Errorf("tool validation failed: %w", err),
		}
	}

	p.cache.Set(toolsCacheKey, validatedTools, core.ToolsValidationCacheTTL)

	data, err := p.buildToolsData(validatedTools)
	if err != nil {
		return ProcessToolsResult{Error: fmt.Errorf("failed to build tools data: %w", err)}
	}

	return ProcessToolsResult{
		Data:          data,
		ValidatedDone: true,
	}
}

func (p *RequestProcessor) buildToolsData(validatedTools []core.Tool) ([]core.JetbrainsData, error) {
	if len(validatedTools) == 0 {
		return []core.JetbrainsData{}, nil
	}

	var jetbrainsTools []core.JetbrainsToolDefinition
	for _, tool := range validatedTools {
		jetbrainsTools = append(jetbrainsTools, core.JetbrainsToolDefinition{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Parameters: core.JetbrainsToolParametersWrapper{
				Schema: tool.Function.Parameters,
			},
		})
	}

	toolsJSON, err := util.MarshalJSON(jetbrainsTools)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tools: %w", err)
	}

	p.logger.Debug("Transformed tools for JetBrains API: %s", string(toolsJSON))

	modifiedTime := time.Now().UnixMilli()
	data := []core.JetbrainsData{
		{Type: "json", FQDN: "llm.parameters.tools"},
		{
			Type:     "json",
			Value:    string(toolsJSON),
			Modified: modifiedTime,
		},
	}

	return data, nil
}

// BuildJetbrainsPayload builds JetBrains API payload from an OpenAI request
func (p *RequestProcessor) BuildJetbrainsPayload(
	request *core.ChatCompletionRequest,
	messages []core.JetbrainsMessage,
	data []core.JetbrainsData,
) ([]byte, error) {
	return p.buildPayload(request.Model, messages, data, len(request.Tools))
}

// BuildPayloadDirect builds JetBrains API payload from pre-converted messages and data.
// Used by the Anthropic path where messages/tools are already in JetBrains format.
func (p *RequestProcessor) BuildPayloadDirect(
	model string,
	messages []core.JetbrainsMessage,
	data []core.JetbrainsData,
) ([]byte, error) {
	return p.buildPayload(model, messages, data, len(data)/2)
}

func (p *RequestProcessor) buildPayload(
	model string,
	messages []core.JetbrainsMessage,
	data []core.JetbrainsData,
	toolCount int,
) ([]byte, error) {
	internalModel := GetInternalModelName(p.modelsConfig, model)

	payload := core.JetbrainsPayload{
		Prompt:  core.JetBrainsChatPrompt,
		Profile: internalModel,
		Chat:    core.JetbrainsChat{Messages: messages},
	}

	if len(data) > 0 {
		payload.Parameters = &core.JetbrainsParameters{Data: data}
	}

	payloadBytes, err := util.MarshalJSON(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	p.logger.Debug("JetBrains payload: model=%s->%s, messages=%d, tools=%d, size=%d",
		model, internalModel, len(messages), toolCount, len(payloadBytes))

	return payloadBytes, nil
}

// SendUpstreamRequest sends upstream request to the given endpoint
func (p *RequestProcessor) SendUpstreamRequest(
	ctx context.Context,
	endpoint string,
	payloadBytes []byte,
	acct *core.JetbrainsAccount,
) (*http.Response, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewBuffer(payloadBytes),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set(core.HeaderAccept, core.ContentTypeEventStream)
	req.Header.Set(core.HeaderContentType, core.ContentTypeJSON)
	req.Header.Set(core.HeaderCacheControl, core.CacheControlNoCache)
	account.SetJetbrainsHeaders(req, acct.JWT)

	if err := util.ValidateJetBrainsRequestTarget(req, "upstream"); err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req) //nolint:gosec // Request target is restricted by util.ValidateJetBrainsRequestTarget.
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}

	p.logger.Debug("JetBrains API Response Status: %d", resp.StatusCode)

	if resp.StatusCode == core.JetBrainsStatusQuotaExhausted {
		p.logger.Warn("Account %s has no quota (received 477)", util.GetTokenDisplayName(acct))
		account.MarkAccountNoQuota(acct)
	}

	return resp, nil
}

// GetInternalModelName gets internal model name by config mapping
func GetInternalModelName(config core.ModelsConfig, modelID string) string {
	if internalModel, exists := config.Models[modelID]; exists {
		return internalModel
	}
	return modelID
}

// ResolveEndpoint returns the appropriate JetBrains API endpoint for the given model.
// Codex models use the Responses endpoint; all others use the Chat endpoint.
func ResolveEndpoint(config core.ModelsConfig, model string) string {
	internal := GetInternalModelName(config, model)
	if strings.Contains(internal, "-codex") {
		return core.JetBrainsResponsesEndpoint
	}
	return core.JetBrainsChatEndpoint
}
