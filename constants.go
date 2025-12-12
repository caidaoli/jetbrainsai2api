package main

import "time"

// ==================== 超时和时间相关常量 ====================

const (
	// QuotaCacheTime 配额缓存时间
	QuotaCacheTime = 1 * time.Hour

	// JWTRefreshTime JWT刷新提前时间
	// 在JWT过期前12小时自动刷新
	JWTRefreshTime = 12 * time.Hour
)

// ==================== HTTP客户端配置常量 ====================

const (
	// HTTPMaxIdleConns HTTP客户端最大空闲连接数
	HTTPMaxIdleConns = 500

	// HTTPMaxIdleConnsPerHost 每个主机最大空闲连接数
	HTTPMaxIdleConnsPerHost = 100

	// HTTPMaxConnsPerHost 每个主机最大连接数
	HTTPMaxConnsPerHost = 200

	// HTTPIdleConnTimeout 空闲连接超时时间
	HTTPIdleConnTimeout = 600 * time.Second // 10分钟

	// HTTPTLSHandshakeTimeout TLS握手超时时间
	HTTPTLSHandshakeTimeout = 30 * time.Second

	// HTTPResponseHeaderTimeout 响应头超时时间
	HTTPResponseHeaderTimeout = 30 * time.Second

	// HTTPExpectContinueTimeout Expect: 100-continue 超时时间
	HTTPExpectContinueTimeout = 5 * time.Second

	// HTTPRequestTimeout HTTP请求总超时时间
	HTTPRequestTimeout = 5 * time.Minute
)

// ==================== 缓存配置常量 ====================

const (
	// CacheDefaultCapacity 默认缓存容量
	CacheDefaultCapacity = 1000

	// CacheCleanupInterval 缓存清理间隔
	CacheCleanupInterval = 5 * time.Minute

	// MessageConversionCacheTTL 消息转换缓存TTL
	MessageConversionCacheTTL = 10 * time.Minute

	// ToolsValidationCacheTTL 工具验证缓存TTL
	ToolsValidationCacheTTL = 30 * time.Minute

	// CacheKeyVersion 缓存键版本号
	// 当缓存数据格式发生变化时，增加此版本号以避免使用旧格式的缓存数据
	CacheKeyVersion = "v1"
)

// ==================== 统计和性能监控常量 ====================

const (
	// StatsFilePath 统计数据文件路径
	StatsFilePath = "stats.json"

	// MinSaveInterval 最小保存间隔（防抖）
	MinSaveInterval = 5 * time.Second

	// HistoryBufferSize 请求历史记录缓冲区大小
	HistoryBufferSize = 1000

	// HistoryBatchSize 历史记录批处理大小
	HistoryBatchSize = 100

	// HistoryFlushInterval 历史记录刷新间隔
	HistoryFlushInterval = 100 * time.Millisecond

	// MetricsMonitorInterval 性能监控间隔
	MetricsMonitorInterval = 30 * time.Second

	// MetricsWindowDuration 性能监控窗口时长
	MetricsWindowDuration = 5 * time.Minute

	// QPSCalculationWindow QPS计算窗口（1分钟）
	QPSCalculationWindow = 1 * time.Minute

	// QPSCalculationSeconds QPS计算窗口秒数
	QPSCalculationSeconds = 60.0
)

// ==================== 账户管理常量 ====================

const (
	// AccountAcquireMaxRetries 获取账户最大重试次数
	AccountAcquireMaxRetries = 3

	// AccountAcquireTimeout 获取账户超时时间
	AccountAcquireTimeout = 60 * time.Second

	// AccountExpiryWarningTime 账户即将过期警告时间（提前24小时警告）
	AccountExpiryWarningTime = 24 * time.Hour

	// JWTExpiryCheckTime JWT过期检查时间（提前1小时检查）
	JWTExpiryCheckTime = 1 * time.Hour
)

// ==================== 图像验证常量 ====================

const (
	// MaxImageSizeBytes 最大图像大小（10MB）
	MaxImageSizeBytes = 10 * 1024 * 1024

	// ImageFormatPNG PNG 图像格式
	ImageFormatPNG = "image/png"

	// ImageFormatJPEG JPEG 图像格式
	ImageFormatJPEG = "image/jpeg"

	// ImageFormatGIF GIF 图像格式
	ImageFormatGIF = "image/gif"

	// ImageFormatWebP WebP 图像格式
	ImageFormatWebP = "image/webp"
)

// SupportedImageFormats 支持的图像格式列表
var SupportedImageFormats = []string{ImageFormatPNG, ImageFormatJPEG, ImageFormatGIF, ImageFormatWebP}

// ==================== 工具验证常量 ====================

const (
	// MaxParamNameLength 参数名最大长度（JetBrains API限制）
	MaxParamNameLength = 64

	// ParamNamePattern 参数名合法字符模式
	ParamNamePattern = "^[a-zA-Z0-9_.-]{1,64}$"

	// MaxPropertiesBeforeSimplification Schema简化阈值（超过此数量时进行简化）
	MaxPropertiesBeforeSimplification = 15

	// MaxNestingDepth Schema最大嵌套深度
	MaxNestingDepth = 5
)

// ==================== JSON Schema 类型常量 ====================

const (
	// SchemaTypeObject JSON Schema object 类型
	SchemaTypeObject = "object"

	// SchemaTypeArray JSON Schema array 类型
	SchemaTypeArray = "array"

	// SchemaTypeString JSON Schema string 类型
	SchemaTypeString = "string"
)

// ==================== 日志配置常量 ====================

const (
	// MaxDebugFilePathLength 调试日志文件路径最大长度
	MaxDebugFilePathLength = 260
)

// ==================== 文件权限常量 ====================

const (
	// FilePermissionReadWrite 文件读写权限 (0644)
	FilePermissionReadWrite = 0644
)

// ==================== HTTP 状态码常量 ====================

const (
	// HTTPStatusUnauthorized JWT 过期/无效
	HTTPStatusUnauthorized = 401
)

// ==================== 账户状态显示常量 ====================

const (
	// AccountStatusNormal 账户状态正常
	AccountStatusNormal = "正常"

	// AccountStatusNoQuota 账户配额不足
	AccountStatusNoQuota = "配额不足"

	// AccountStatusExpiring 账户即将过期
	AccountStatusExpiring = "即将过期"
)

// ==================== 时间格式常量 ====================

const (
	// TimeFormatDateTime 日期时间格式 (YYYY-MM-DD HH:MM:SS)
	TimeFormatDateTime = "2006-01-02 15:04:05"
)

// ==================== 默认配置常量 ====================

const (
	// DefaultPort 默认服务端口
	DefaultPort = "7860"

	// DefaultGinMode 默认Gin运行模式
	DefaultGinMode = "release"

	// GinModeDebug Gin调试模式
	GinModeDebug = "debug"

	// DefaultModelsConfigPath 默认模型配置文件路径
	DefaultModelsConfigPath = "models.json"

	// CORSMaxAge CORS预检请求缓存时间（秒）
	CORSMaxAge = "86400"
)

// ==================== API相关常量 ====================

const (
	// JetBrains API对象类型
	ModelObjectType = "model"
	ModelOwner      = "jetbrains-ai"

	// OpenAI兼容对象类型
	ChatCompletionObjectType      = "chat.completion"
	ChatCompletionChunkObjectType = "chat.completion.chunk"
	ModelListObjectType           = "list"
	StreamChunkDoneMessage        = "[DONE]"
	StreamChunkPrefix             = "data: "

	// HTTP Content-Type
	ContentTypeEventStream = "text/event-stream"
	ContentTypeJSON        = "application/json"

	// HTTP Cache-Control
	CacheControlNoCache = "no-cache"

	// HTTP Connection
	ConnectionKeepAlive = "keep-alive"

	// HTTP Headers
	HeaderContentType   = "Content-Type"
	HeaderAuthorization = "Authorization"
	HeaderAccept        = "Accept"
	HeaderCacheControl  = "Cache-Control"
	HeaderConnection    = "Connection"
	HeaderXAPIKey       = "x-api-key"

	// Auth prefix
	AuthBearerPrefix = "Bearer "
)

// ==================== JetBrains API Header 常量 ====================

const (
	// JetBrainsHeaderUserAgent JetBrains API User-Agent
	JetBrainsHeaderUserAgent = "ktor-client"

	// JetBrainsHeaderGrazieAgent JetBrains Grazie Agent 标识
	JetBrainsHeaderGrazieAgent = `{"name":"aia:pycharm","version":"251.26094.80.13:251.26094.141"}`

	// HeaderGrazieAgent Grazie Agent header 名
	HeaderGrazieAgent = "grazie-agent"

	// HeaderGrazieAuthJWT Grazie JWT 认证 header 名
	HeaderGrazieAuthJWT = "grazie-authenticate-jwt"

	// HeaderAcceptCharset Accept-Charset header 名
	HeaderAcceptCharset = "Accept-Charset"

	// CharsetUTF8 UTF-8 字符集
	CharsetUTF8 = "UTF-8"
)

// ==================== JetBrains API 端点常量 ====================

const (
	// JetBrainsAPIBaseURL JetBrains API 基础 URL
	JetBrainsAPIBaseURL = "https://api.jetbrains.ai"

	// JetBrainsJWTEndpoint JWT 认证端点
	JetBrainsJWTEndpoint = JetBrainsAPIBaseURL + "/auth/jetbrains-jwt/provide-access/license/v2"

	// JetBrainsQuotaEndpoint 配额查询端点
	JetBrainsQuotaEndpoint = JetBrainsAPIBaseURL + "/user/v5/quota/get"

	// JetBrainsChatEndpoint 聊天流式端点
	JetBrainsChatEndpoint = JetBrainsAPIBaseURL + "/user/v5/llm/chat/stream/v8"

	// JetBrainsStatusQuotaExhausted 配额耗尽状态码 (JetBrains 专有)
	JetBrainsStatusQuotaExhausted = 477

	// JetBrainsChatPrompt 聊天请求的固定 prompt 参数
	JetBrainsChatPrompt = "ij.chat.request.new-chat-on-start"
)

// ==================== JetBrains 流事件类型常量 ====================

const (
	// JetBrainsEventTypeContent 内容事件
	JetBrainsEventTypeContent = "Content"

	// JetBrainsEventTypeToolCall 工具调用事件
	JetBrainsEventTypeToolCall = "ToolCall"

	// JetBrainsEventTypeFunctionCall 函数调用事件（旧格式）
	JetBrainsEventTypeFunctionCall = "FunctionCall"

	// JetBrainsEventTypeFinishMetadata 结束元数据事件
	JetBrainsEventTypeFinishMetadata = "FinishMetadata"
)

// ==================== JetBrains 消息类型常量 ====================

const (
	// JetBrainsMessageTypeUser 用户消息类型
	JetBrainsMessageTypeUser = "user_message"

	// JetBrainsMessageTypeAssistant 助手消息类型
	JetBrainsMessageTypeAssistant = "assistant_message"

	// JetBrainsMessageTypeAssistantText 助手文本消息类型
	JetBrainsMessageTypeAssistantText = "assistant_message_text"

	// JetBrainsMessageTypeAssistantTool 助手工具调用消息类型
	JetBrainsMessageTypeAssistantTool = "assistant_message_tool"

	// JetBrainsMessageTypeSystem 系统消息类型
	JetBrainsMessageTypeSystem = "system_message"

	// JetBrainsMessageTypeTool 工具结果消息类型
	JetBrainsMessageTypeTool = "tool_message"

	// JetBrainsMessageTypeMedia 媒体消息类型
	JetBrainsMessageTypeMedia = "media_message"
)

// ==================== ID 前缀常量 ====================

const (
	// ResponseIDPrefix OpenAI 响应 ID 前缀
	ResponseIDPrefix = "chatcmpl-"

	// MessageIDPrefix Anthropic 消息 ID 前缀
	MessageIDPrefix = "msg_"

	// ToolCallIDPrefix 工具调用 ID 前缀
	ToolCallIDPrefix = "toolu_"
)

// ==================== OpenAI 工具类型常量 ====================

const (
	// ToolTypeFunction 函数工具类型
	ToolTypeFunction = "function"
)

// ==================== OpenAI Finish Reason 常量 ====================

const (
	// FinishReasonStop 正常停止
	FinishReasonStop = "stop"

	// FinishReasonToolCalls 工具调用
	FinishReasonToolCalls = "tool_calls"

	// FinishReasonLength 达到最大 token 限制
	FinishReasonLength = "length"
)

// ==================== JetBrains Finish Reason 常量 ====================
// JetBrains API 返回的结束原因（与 OpenAI 格式略有不同）

const (
	// JetBrainsFinishReasonToolCall JetBrains 工具调用（单数形式）
	JetBrainsFinishReasonToolCall = "tool_call"

	// JetBrainsFinishReasonStop JetBrains 正常停止
	JetBrainsFinishReasonStop = "stop"

	// JetBrainsFinishReasonLength JetBrains 达到最大 token 限制
	JetBrainsFinishReasonLength = "length"
)

// ==================== 角色常量 ====================

const (
	// RoleAssistant 助手角色
	RoleAssistant = "assistant"

	// RoleUser 用户角色
	RoleUser = "user"

	// RoleSystem 系统角色
	RoleSystem = "system"

	// RoleTool 工具角色
	RoleTool = "tool"
)

// ==================== Anthropic 响应类型常量 ====================

const (
	// AnthropicTypeMessage 消息类型
	AnthropicTypeMessage = "message"

	// AnthropicDeltaTypeText 文本 delta 类型
	AnthropicDeltaTypeText = "text_delta"

	// StopReasonEndTurn 正常结束
	StopReasonEndTurn = "end_turn"

	// StopReasonToolUse 工具调用
	StopReasonToolUse = "tool_use"

	// StopReasonMaxTokens 达到最大 token 限制
	StopReasonMaxTokens = "max_tokens"

	// ContentBlockTypeToolUse 工具使用内容块类型
	ContentBlockTypeToolUse = "tool_use"

	// ContentBlockTypeToolResult 工具结果内容块类型
	ContentBlockTypeToolResult = "tool_result"

	// ContentBlockTypeText 文本内容块类型
	ContentBlockTypeText = "text"
)

// ==================== Anthropic 错误类型常量 ====================

const (
	// AnthropicErrorInvalidRequest 无效请求错误
	AnthropicErrorInvalidRequest = "invalid_request_error"

	// AnthropicErrorRateLimit 频率限制错误
	AnthropicErrorRateLimit = "rate_limit_error"

	// AnthropicErrorAPI API 错误
	AnthropicErrorAPI = "api_error"

	// AnthropicErrorModelNotFound 模型未找到错误
	AnthropicErrorModelNotFound = "model_not_found_error"
)

// ==================== API 格式标识常量 ====================

const (
	// APIFormatOpenAI OpenAI 格式
	APIFormatOpenAI = "openai"

	// APIFormatAnthropic Anthropic 格式
	APIFormatAnthropic = "anthropic"
)

// ==================== Anthropic SSE 事件常量 ====================

const (
	// AnthropicEventMessageStart 消息开始事件
	AnthropicEventMessageStart = "event: message_start\n"

	// AnthropicEventMessageStop 消息结束事件
	AnthropicEventMessageStop = "event: message_stop\n"

	// AnthropicEventContentBlockStart 内容块开始事件
	AnthropicEventContentBlockStart = "event: content_block_start\n"

	// AnthropicEventContentBlockDelta 内容块增量事件
	AnthropicEventContentBlockDelta = "event: content_block_delta\n"

	// AnthropicEventContentBlockStop 内容块结束事件
	AnthropicEventContentBlockStop = "event: content_block_stop\n"
)

// ==================== Anthropic 流事件类型常量 ====================

const (
	// StreamEventTypeMessageStart 消息开始
	StreamEventTypeMessageStart = "message_start"

	// StreamEventTypeMessageStop 消息结束
	StreamEventTypeMessageStop = "message_stop"

	// StreamEventTypeContentBlockStart 内容块开始
	StreamEventTypeContentBlockStart = "content_block_start"

	// StreamEventTypeContentBlockDelta 内容块增量
	StreamEventTypeContentBlockDelta = "content_block_delta"

	// StreamEventTypeContentBlockStop 内容块结束
	StreamEventTypeContentBlockStop = "content_block_stop"
)

// ==================== Tool Choice 常量 ====================

const (
	// ToolChoiceAny 强制使用任意工具
	ToolChoiceAny = "any"

	// ToolChoiceAuto 自动选择工具
	ToolChoiceAuto = "auto"

	// ToolChoiceNone 不使用工具
	ToolChoiceNone = "none"
)

// ==================== SSE 流结束标记常量 ====================

const (
	// StreamEndMarker SSE 流结束标记
	StreamEndMarker = "end"

	// StreamNullValue 空值标记
	StreamNullValue = "null"

	// StreamEndLine 完整的流结束行 (data: end)
	StreamEndLine = StreamChunkPrefix + StreamEndMarker
)
