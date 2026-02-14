package core

// OpenAI object type constants
const (
	ModelObjectType               = "model"
	ModelOwner                    = "jetbrains-ai"
	ChatCompletionObjectType      = "chat.completion"
	ChatCompletionChunkObjectType = "chat.completion.chunk"
	ModelListObjectType           = "list"
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

// Tool choice constants
const (
	ToolChoiceAny = "any"
)
