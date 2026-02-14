package core

// Default config constants
const (
	DefaultPort             = "7860"
	DefaultGinMode          = "release"
	DefaultModelsConfigPath = "models.json"
	CORSMaxAge              = "86400"
)

// Content type and header constants
const (
	ContentTypeEventStream = "text/event-stream"
	ContentTypeJSON        = "application/json"
	CacheControlNoCache    = "no-cache"
	ConnectionKeepAlive    = "keep-alive"
	HeaderContentType      = "Content-Type"
	HeaderAuthorization    = "Authorization"
	HeaderAccept           = "Accept"
	HeaderCacheControl     = "Cache-Control"
	HeaderConnection       = "Connection"
	HeaderXAPIKey          = "x-api-key"
	AuthBearerPrefix       = "Bearer "
)

// SSE stream constants
const (
	StreamChunkDoneMessage = "[DONE]"
	StreamChunkPrefix      = "data: "
)

// Role constants
const (
	RoleAssistant = "assistant"
	RoleUser      = "user"
	RoleSystem    = "system"
	RoleTool      = "tool"
)

// API format identifier constants
const (
	APIFormatOpenAI    = "openai"
	APIFormatAnthropic = "anthropic"
)

// SSE stream end marker constants
const (
	StreamEndMarker = "end"
	StreamNullValue = "null"
	StreamEndLine   = StreamChunkPrefix + StreamEndMarker
)
