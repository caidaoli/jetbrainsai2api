package process

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
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
			data := p.buildToolsData(validatedTools)
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

	data := p.buildToolsData(validatedTools)

	return ProcessToolsResult{
		Data:          data,
		ValidatedDone: true,
	}
}

func (p *RequestProcessor) buildToolsData(validatedTools []core.Tool) []core.JetbrainsData {
	if len(validatedTools) == 0 {
		return []core.JetbrainsData{}
	}

	data := []core.JetbrainsData{
		{Type: "json", FQDN: "llm.parameters.tools"},
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
		p.logger.Warn("Failed to marshal tools: %v", err)
		return data
	}

	p.logger.Debug("Transformed tools for JetBrains API: %s", string(toolsJSON))

	modifiedTime := time.Now().UnixMilli()
	data = append(data, core.JetbrainsData{
		Type:     "json",
		Value:    string(toolsJSON),
		Modified: modifiedTime,
	})

	return data
}

// BuildJetbrainsPayload builds JetBrains API payload
func (p *RequestProcessor) BuildJetbrainsPayload(
	request *core.ChatCompletionRequest,
	messages []core.JetbrainsMessage,
	data []core.JetbrainsData,
) ([]byte, error) {
	internalModel := GetInternalModelName(p.modelsConfig, request.Model)

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

	p.logger.Debug("=== JetBrains API Request Debug ===")
	p.logger.Debug("Model: %s -> %s", request.Model, internalModel)
	p.logger.Debug("Messages processed: %d", len(messages))
	p.logger.Debug("Tools processed: %d", len(request.Tools))
	p.logger.Debug("Payload size: %d bytes", len(payloadBytes))
	p.logger.Debug("=== Complete Upstream Payload ===")
	p.logger.Debug("%s", string(payloadBytes))
	p.logger.Debug("=== End Upstream Payload ===")

	return payloadBytes, nil
}

// SendUpstreamRequest sends upstream request
func (p *RequestProcessor) SendUpstreamRequest(
	ctx context.Context,
	payloadBytes []byte,
	acct *core.JetbrainsAccount,
) (*http.Response, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		core.JetBrainsChatEndpoint,
		bytes.NewBuffer(payloadBytes),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set(core.HeaderAccept, core.ContentTypeEventStream)
	req.Header.Set(core.HeaderContentType, core.ContentTypeJSON)
	req.Header.Set(core.HeaderCacheControl, core.CacheControlNoCache)
	account.SetJetbrainsHeaders(req, acct.JWT)

	resp, err := p.httpClient.Do(req)
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
