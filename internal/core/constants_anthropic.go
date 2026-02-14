package core

// Anthropic response type constants
const (
	AnthropicTypeMessage   = "message"
	AnthropicDeltaTypeText = "text_delta"
)

// Anthropic stop reason constants
const (
	StopReasonEndTurn   = "end_turn"
	StopReasonToolUse   = "tool_use"
	StopReasonMaxTokens = "max_tokens"
)

// Anthropic content block type constants
const (
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

// Anthropic stream event type constants
const (
	StreamEventTypeMessageStart      = "message_start"
	StreamEventTypeMessageStop       = "message_stop"
	StreamEventTypeContentBlockStart = "content_block_start"
	StreamEventTypeContentBlockDelta = "content_block_delta"
	StreamEventTypeContentBlockStop  = "content_block_stop"
)
