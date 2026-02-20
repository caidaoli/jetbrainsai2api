package core

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
	JetBrainsAPIBaseURL            = "https://api.jetbrains.ai"
	JetBrainsJWTEndpoint           = JetBrainsAPIBaseURL + "/auth/jetbrains-jwt/provide-access/license/v2"
	JetBrainsQuotaEndpoint         = JetBrainsAPIBaseURL + "/user/v5/quota/get"
	JetBrainsChatEndpoint          = JetBrainsAPIBaseURL + "/user/v5/llm/chat/stream/v8"
	JetBrainsResponsesEndpoint     = JetBrainsAPIBaseURL + "/user/v5/llm/responses/stream/v8"
	JetBrainsStatusQuotaExhausted  = 477
	JetBrainsChatPrompt            = "ij.chat.request.new-chat-on-start"
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

// JetBrains finish reason constants
const (
	JetBrainsFinishReasonToolCall = "tool_call"
	JetBrainsFinishReasonStop     = "stop"
	JetBrainsFinishReasonLength   = "length"
)
