package core

import "time"

// Timeout and time constants
const (
	QuotaCacheTime = 1 * time.Hour
	JWTRefreshTime = 12 * time.Hour
)

// HTTP client config constants
const (
	HTTPMaxIdleConns          = 500
	HTTPMaxIdleConnsPerHost   = 100
	HTTPMaxConnsPerHost       = 200
	HTTPIdleConnTimeout       = 600 * time.Second
	HTTPTLSHandshakeTimeout   = 30 * time.Second
	HTTPResponseHeaderTimeout = 30 * time.Second
	HTTPExpectContinueTimeout = 5 * time.Second
	HTTPRequestTimeout        = 5 * time.Minute
)

// Cache config constants
const (
	CacheDefaultCapacity      = 1000
	CacheCleanupInterval      = 5 * time.Minute
	MessageConversionCacheTTL = 10 * time.Minute
	ToolsValidationCacheTTL   = 30 * time.Minute
	CacheKeyVersion           = "v1"
)

// Stats and monitoring constants
const (
	StatsFilePath        = "stats.json"
	MinSaveInterval      = 5 * time.Second
	HistoryBufferSize    = 1000
	HistoryBatchSize     = 100
	HistoryFlushInterval = 100 * time.Millisecond
)

// Account management constants
const (
	AccountAcquireTimeout    = 60 * time.Second
	AccountExpiryWarningTime = 24 * time.Hour
	JWTExpiryCheckTime       = 1 * time.Hour
)

// Image validation constants
const (
	MaxImageSizeBytes = 10 * 1024 * 1024
	ImageFormatPNG    = "image/png"
	ImageFormatJPEG   = "image/jpeg"
	ImageFormatGIF    = "image/gif"
	ImageFormatWebP   = "image/webp"
)

// Response body size limits
const (
	MaxResponseBodySize  = 10 * 1024 * 1024
	MaxScannerBufferSize = 1024 * 1024
)

// SupportedImageFormats supported image format list
var SupportedImageFormats = []string{ImageFormatPNG, ImageFormatJPEG, ImageFormatGIF, ImageFormatWebP}

// Tool validation constants
const (
	MaxParamNameLength                       = 64
	ParamNamePattern                         = "^[a-zA-Z0-9_.-]{1,64}$"
	MaxPropertiesBeforeSimplification        = 15
	MaxNestingDepth                          = 5
	MaxPreservedPropertiesInSimplifiedSchema = 5
)

// JSON Schema type constants
const (
	SchemaTypeObject = "object"
	SchemaTypeArray  = "array"
	SchemaTypeString = "string"
)

// Logging config constants
const (
	MaxDebugFilePathLength = 260
)

// File permission constants
const (
	FilePermissionReadWrite = 0644
)

// HTTP status code constants
const (
	HTTPStatusUnauthorized = 401
)

// Account status display constants
const (
	AccountStatusNormal   = "正常"
	AccountStatusNoQuota  = "配额不足"
	AccountStatusExpiring = "即将过期"
)

// Time format constants
const (
	TimeFormatDateTime = "2006-01-02 15:04:05"
)

// Default config constants
const (
	DefaultPort             = "7860"
	DefaultGinMode          = "release"
	DefaultModelsConfigPath = "models.json"
	CORSMaxAge              = "86400"
)

// API constants
const (
	ModelObjectType               = "model"
	ModelOwner                    = "jetbrains-ai"
	ChatCompletionObjectType      = "chat.completion"
	ChatCompletionChunkObjectType = "chat.completion.chunk"
	ModelListObjectType           = "list"
	StreamChunkDoneMessage        = "[DONE]"
	StreamChunkPrefix             = "data: "
	ContentTypeEventStream        = "text/event-stream"
	ContentTypeJSON               = "application/json"
	CacheControlNoCache           = "no-cache"
	ConnectionKeepAlive           = "keep-alive"
	HeaderContentType             = "Content-Type"
	HeaderAuthorization           = "Authorization"
	HeaderAccept                  = "Accept"
	HeaderCacheControl            = "Cache-Control"
	HeaderConnection              = "Connection"
	HeaderXAPIKey                 = "x-api-key"
	AuthBearerPrefix              = "Bearer "
)

// JetBrains API Header constants
const (
	JetBrainsHeaderUserAgent   = "ktor-client"
	JetBrainsHeaderGrazieAgent = `{"name":"aia:pycharm","version":"251.26094.80.13:251.26094.141"}`
	HeaderGrazieAgent          = "grazie-agent"
	HeaderGrazieAuthJWT        = "grazie-authenticate-jwt"
	HeaderAcceptCharset        = "Accept-Charset"
	CharsetUTF8                = "UTF-8"
)

// JetBrains API endpoint constants
const (
	JetBrainsAPIBaseURL           = "https://api.jetbrains.ai"
	JetBrainsJWTEndpoint          = JetBrainsAPIBaseURL + "/auth/jetbrains-jwt/provide-access/license/v2"
	JetBrainsQuotaEndpoint        = JetBrainsAPIBaseURL + "/user/v5/quota/get"
	JetBrainsChatEndpoint         = JetBrainsAPIBaseURL + "/user/v5/llm/chat/stream/v8"
	JetBrainsStatusQuotaExhausted = 477
	JetBrainsChatPrompt           = "ij.chat.request.new-chat-on-start"
)

// JetBrains stream event type constants
const (
	JetBrainsEventTypeContent        = "Content"
	JetBrainsEventTypeToolCall       = "ToolCall"
	JetBrainsEventTypeFunctionCall   = "FunctionCall"
	JetBrainsEventTypeFinishMetadata = "FinishMetadata"
)

// JetBrains message type constants
const (
	JetBrainsMessageTypeUser          = "user_message"
	JetBrainsMessageTypeAssistant     = "assistant_message"
	JetBrainsMessageTypeAssistantText = "assistant_message_text"
	JetBrainsMessageTypeAssistantTool = "assistant_message_tool"
	JetBrainsMessageTypeSystem        = "system_message"
	JetBrainsMessageTypeTool          = "tool_message"
	JetBrainsMessageTypeMedia         = "media_message"
)

// ID prefix constants
const (
	ResponseIDPrefix = "chatcmpl-"
	MessageIDPrefix  = "msg_"
	ToolCallIDPrefix = "toolu_"
)

// OpenAI tool type constants
const (
	ToolTypeFunction = "function"
)

// OpenAI finish reason constants
const (
	FinishReasonStop      = "stop"
	FinishReasonToolCalls = "tool_calls"
	FinishReasonLength    = "length"
)

// JetBrains finish reason constants
const (
	JetBrainsFinishReasonToolCall = "tool_call"
	JetBrainsFinishReasonStop     = "stop"
	JetBrainsFinishReasonLength   = "length"
)

// Role constants
const (
	RoleAssistant = "assistant"
	RoleUser      = "user"
	RoleSystem    = "system"
	RoleTool      = "tool"
)

// Anthropic response type constants
const (
	AnthropicTypeMessage       = "message"
	AnthropicDeltaTypeText     = "text_delta"
	StopReasonEndTurn          = "end_turn"
	StopReasonToolUse          = "tool_use"
	StopReasonMaxTokens        = "max_tokens"
	ContentBlockTypeToolUse    = "tool_use"
	ContentBlockTypeToolResult = "tool_result"
	ContentBlockTypeText       = "text"
)

// Anthropic error type constants
const (
	AnthropicErrorInvalidRequest = "invalid_request_error"
	AnthropicErrorRateLimit      = "rate_limit_error"
	AnthropicErrorAPI            = "api_error"
	AnthropicErrorModelNotFound  = "model_not_found_error"
)

// API format identifier constants
const (
	APIFormatOpenAI    = "openai"
	APIFormatAnthropic = "anthropic"
)

// Anthropic stream event type constants
const (
	StreamEventTypeMessageStart      = "message_start"
	StreamEventTypeMessageStop       = "message_stop"
	StreamEventTypeContentBlockStart = "content_block_start"
	StreamEventTypeContentBlockDelta = "content_block_delta"
	StreamEventTypeContentBlockStop  = "content_block_stop"
)

// Tool choice constants
const (
	ToolChoiceAny = "any"
)

// SSE stream end marker constants
const (
	StreamEndMarker = "end"
	StreamNullValue = "null"
	StreamEndLine   = StreamChunkPrefix + StreamEndMarker
)
