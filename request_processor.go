package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"
)

// RequestProcessor 请求处理器结构
// 负责将 OpenAI 格式请求转换为 JetBrains API 格式
type RequestProcessor struct {
	modelsConfig ModelsConfig
	httpClient   *http.Client
	cache        Cache            // 注入缓存依赖
	metrics      MetricsCollector // 注入指标收集器
	logger       Logger           // 注入日志依赖
}

// NewRequestProcessor 创建新的请求处理器
func NewRequestProcessor(modelsConfig ModelsConfig, httpClient *http.Client, cache Cache, metrics MetricsCollector, logger Logger) *RequestProcessor {
	return &RequestProcessor{
		modelsConfig: modelsConfig,
		httpClient:   httpClient,
		cache:        cache,
		metrics:      metrics,
		logger:       logger,
	}
}

// ProcessMessagesResult 消息处理结果
type ProcessMessagesResult struct {
	JetbrainsMessages []JetbrainsMessage
	CacheHit          bool
}

// ProcessMessages 处理消息转换和缓存
// SRP: 单一职责 - 只负责消息格式转换
func (p *RequestProcessor) ProcessMessages(messages []ChatMessage) ProcessMessagesResult {
	// 生成缓存键
	cacheKey := generateMessagesCacheKey(messages)

	// 尝试从缓存获取（使用注入的 cache 而非全局变量）
	if cachedAny, found := p.cache.Get(cacheKey); found {
		// 安全的类型断言，防止缓存污染导致panic
		if jetbrainsMessages, ok := cachedAny.([]JetbrainsMessage); ok {
			p.metrics.RecordCacheHit()
			return ProcessMessagesResult{
				JetbrainsMessages: jetbrainsMessages,
				CacheHit:          true,
			}
		}
		// 缓存格式错误，视为缓存失效，记录警告并重新生成
		p.logger.Warn("Cache format mismatch for messages (key: %s), regenerating", truncateCacheKey(cacheKey, 16))
	}

	// 缓存未命中或格式错误，执行转换
	p.metrics.RecordCacheMiss()
	jetbrainsMessages := openAIToJetbrainsMessages(messages)

	// 缓存结果
	p.cache.Set(cacheKey, jetbrainsMessages, MessageConversionCacheTTL)

	return ProcessMessagesResult{
		JetbrainsMessages: jetbrainsMessages,
		CacheHit:          false,
	}
}

// ProcessToolsResult 工具处理结果
type ProcessToolsResult struct {
	Data          []JetbrainsData
	ValidatedDone bool
	Error         error
}

// ProcessTools 处理工具验证和转换
// SRP: 单一职责 - 只负责工具验证和格式转换
func (p *RequestProcessor) ProcessTools(request *ChatCompletionRequest) ProcessToolsResult {
	if len(request.Tools) == 0 {
		return ProcessToolsResult{
			Data:          []JetbrainsData{},
			ValidatedDone: true,
		}
	}

	// 尝试从缓存获取验证结果（使用注入的 cache 而非全局变量）
	toolsCacheKey := generateToolsCacheKey(request.Tools)
	if cachedAny, found := p.cache.Get(toolsCacheKey); found {
		// 安全的类型断言，防止缓存污染导致panic
		if validatedTools, ok := cachedAny.([]Tool); ok {
			p.metrics.RecordCacheHit()
			data := p.buildToolsData(validatedTools)
			return ProcessToolsResult{
				Data:          data,
				ValidatedDone: true,
			}
		}
		// 缓存格式错误，视为缓存失效，记录警告并重新验证
		p.logger.Warn("Cache format mismatch for tools (key: %s), revalidating", truncateCacheKey(toolsCacheKey, 16))
	}

	// 缓存未命中或格式错误，执行验证
	p.metrics.RecordCacheMiss()
	validationStart := time.Now()
	validatedTools, err := validateAndTransformTools(request.Tools, p.logger)
	validationDuration := time.Since(validationStart)
	p.metrics.RecordToolValidation(validationDuration)

	if err != nil {
		return ProcessToolsResult{
			Error: fmt.Errorf("tool validation failed: %w", err),
		}
	}

	// 缓存验证结果
	p.cache.Set(toolsCacheKey, validatedTools, ToolsValidationCacheTTL)

	// 构建工具数据
	data := p.buildToolsData(validatedTools)

	return ProcessToolsResult{
		Data:          data,
		ValidatedDone: true,
	}
}

// buildToolsData 构建工具数据结构
// 辅助函数，封装工具数据构建逻辑
func (p *RequestProcessor) buildToolsData(validatedTools []Tool) []JetbrainsData {
	if len(validatedTools) == 0 {
		return []JetbrainsData{}
	}

	data := []JetbrainsData{
		{Type: "json", FQDN: "llm.parameters.tools"},
	}

	// 转换为 JetBrains 格式
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

	toolsJSON, err := marshalJSON(jetbrainsTools)
	if err != nil {
		p.logger.Warn("Failed to marshal tools: %v", err)
		return data
	}

	p.logger.Debug("Transformed tools for JetBrains API: %s", string(toolsJSON))

	// 添加工具定义数据
	modifiedTime := time.Now().UnixMilli()
	data = append(data, JetbrainsData{
		Type:     "json",
		Value:    string(toolsJSON),
		Modified: modifiedTime,
	})

	return data
}

// BuildJetbrainsPayload 构建 JetBrains API payload
// SRP: 单一职责 - 只负责构建上游 API 的请求体
func (p *RequestProcessor) BuildJetbrainsPayload(
	request *ChatCompletionRequest,
	messages []JetbrainsMessage,
	data []JetbrainsData,
) ([]byte, error) {
	internalModel := getInternalModelName(p.modelsConfig, request.Model)

	payload := JetbrainsPayload{
		Prompt:  JetBrainsChatPrompt,
		Profile: internalModel,
		Chat:    JetbrainsChat{Messages: messages},
	}

	// 只有当有数据时才设置 Parameters
	if len(data) > 0 {
		payload.Parameters = &JetbrainsParameters{Data: data}
	}

	payloadBytes, err := marshalJSON(payload)
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

// SendUpstreamRequest 发送上游请求
// SRP: 单一职责 - 只负责 HTTP 请求发送
func (p *RequestProcessor) SendUpstreamRequest(
	ctx context.Context,
	payloadBytes []byte,
	account *JetbrainsAccount,
) (*http.Response, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		JetBrainsChatEndpoint,
		bytes.NewBuffer(payloadBytes),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set(HeaderAccept, ContentTypeEventStream)
	req.Header.Set(HeaderContentType, ContentTypeJSON)
	req.Header.Set(HeaderCacheControl, CacheControlNoCache)
	setJetbrainsHeaders(req, account.JWT)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}

	p.logger.Debug("JetBrains API Response Status: %d", resp.StatusCode)

	// 检查配额状态
	if resp.StatusCode == JetBrainsStatusQuotaExhausted {
		p.logger.Warn("Account %s has no quota (received 477)", getTokenDisplayName(account))
		account.HasQuota = false
		account.LastQuotaCheck = float64(time.Now().Unix())
	}

	return resp, nil
}
