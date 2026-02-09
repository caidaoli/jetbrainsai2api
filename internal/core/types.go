package core

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
)

// JetbrainsQuotaResponse defines the structure for the JetBrains quota API response.
type JetbrainsQuotaResponse struct {
	Current struct {
		Current struct {
			Amount string `json:"amount"`
		} `json:"current"`
		Maximum struct {
			Amount string `json:"amount"`
		} `json:"maximum"`
	} `json:"current"`
	Until string `json:"until"`
}

// Clone returns a deep copy of JetbrainsQuotaResponse.
func (q *JetbrainsQuotaResponse) Clone() *JetbrainsQuotaResponse {
	if q == nil {
		return nil
	}
	return &JetbrainsQuotaResponse{
		Current: struct {
			Current struct {
				Amount string `json:"amount"`
			} `json:"current"`
			Maximum struct {
				Amount string `json:"amount"`
			} `json:"maximum"`
		}{
			Current: struct {
				Amount string `json:"amount"`
			}{
				Amount: q.Current.Current.Amount,
			},
			Maximum: struct {
				Amount string `json:"amount"`
			}{
				Amount: q.Current.Maximum.Amount,
			},
		},
		Until: q.Until,
	}
}

// RequestStats holds aggregated request statistics for monitoring.
type RequestStats struct {
	TotalRequests      int64           `json:"total_requests"`
	SuccessfulRequests int64           `json:"successful_requests"`
	FailedRequests     int64           `json:"failed_requests"`
	TotalResponseTime  int64           `json:"total_response_time"`
	LastRequestTime    time.Time       `json:"last_request_time"`
	RequestHistory     []RequestRecord `json:"request_history"`
}

// RequestRecord represents a single request's metadata for history tracking.
type RequestRecord struct {
	Timestamp    time.Time `json:"timestamp"`
	Success      bool      `json:"success"`
	ResponseTime int64     `json:"response_time"`
	Model        string    `json:"model"`
	Account      string    `json:"account"`
}

// PeriodStats holds computed statistics for a time period.
type PeriodStats struct {
	Requests        int64   `json:"requests"`
	SuccessRate     float64 `json:"successRate"`
	AvgResponseTime int64   `json:"avgResponseTime"`
	QPS             float64 `json:"qps"`
}

// TokenInfo holds account token and quota information for the monitoring panel.
type TokenInfo struct {
	Name       string    `json:"name"`
	License    string    `json:"license"`
	Used       float64   `json:"used"`
	Total      float64   `json:"total"`
	UsageRate  float64   `json:"usage_rate"`
	ExpiryDate time.Time `json:"expiry_date"`
	Status     string    `json:"status"`
	HasQuota   bool      `json:"has_quota"`
}

// JetbrainsAccount represents a JetBrains API account with JWT credentials.
type JetbrainsAccount struct {
	LicenseID      string     `json:"licenseId,omitempty"`
	Authorization  string     `json:"authorization,omitempty"`
	JWT            string     `json:"jwt,omitempty"`
	LastUpdated    float64    `json:"last_updated"`
	HasQuota       bool       `json:"has_quota"`
	LastQuotaCheck float64    `json:"last_quota_check"`
	ExpiryTime     time.Time  `json:"expiry_time"`
	Mu             sync.Mutex `json:"-"` // account-level mutex for JWT refresh and quota check
}

// ModelInfo represents a single model entry in the models list.
type ModelInfo struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ModelsData holds a list of available models.
type ModelsData struct {
	Data []ModelInfo `json:"data"`
}

// ModelList is the OpenAI-compatible model list response.
type ModelList struct {
	Object string      `json:"object"`
	Data   []ModelInfo `json:"data"`
}

// ModelsConfig holds the model ID mapping configuration from models.json.
type ModelsConfig struct {
	Models map[string]string `json:"models"`
}

// ChatMessage represents a single message in an OpenAI chat completion request.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    any        `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall represents a tool invocation within a chat message.
type ToolCall struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

// Function holds the function name and arguments for a tool call.
type Function struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatCompletionRequest is the OpenAI-compatible chat completion request payload.
type ChatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	TopP        *float64      `json:"top_p,omitempty"`
	Tools       []Tool        `json:"tools,omitempty"`
	ToolChoice  any           `json:"tool_choice,omitempty"`
	Stop        any           `json:"stop,omitempty"`
	ServiceTier string        `json:"service_tier,omitempty"`
}

// Tool represents a tool definition in an OpenAI chat completion request.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction holds the function schema for a tool definition.
type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

// JetbrainsToolDefinition is the JetBrains-specific tool definition format.
type JetbrainsToolDefinition struct {
	Name        string                         `json:"name"`
	Description string                         `json:"description,omitempty"`
	Parameters  JetbrainsToolParametersWrapper `json:"parameters"`
}

// JetbrainsToolParametersWrapper wraps tool parameter schemas in JetBrains format.
type JetbrainsToolParametersWrapper struct {
	Schema map[string]any `json:"schema"`
}

// ChatCompletionChoice represents a single choice in an OpenAI chat completion response.
type ChatCompletionChoice struct {
	Message      ChatMessage `json:"message"`
	Index        int         `json:"index"`
	FinishReason string      `json:"finish_reason"`
}

// ChatCompletionResponse is the OpenAI-compatible non-streaming chat completion response.
type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   map[string]int         `json:"usage"`
}

// StreamChoice represents a single choice in an OpenAI streaming response chunk.
type StreamChoice struct {
	Delta        map[string]any `json:"delta"`
	Index        int            `json:"index"`
	FinishReason *string        `json:"finish_reason"`
}

// StreamResponse is the OpenAI-compatible streaming response chunk.
type StreamResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []StreamChoice `json:"choices"`
}

// JetbrainsMessage represents a message in the JetBrains v8 API format including tool calls.
type JetbrainsMessage struct {
	Type      string `json:"type"`
	Content   string `json:"content,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
	Data      string `json:"data,omitempty"`
	ID        string `json:"id,omitempty"`
	ToolName  string `json:"toolName,omitempty"`
	Result    string `json:"result,omitempty"`
}

// JetbrainsPayload is the top-level request payload sent to JetBrains API.
type JetbrainsPayload struct {
	Prompt     string               `json:"prompt"`
	Profile    string               `json:"profile"`
	Chat       JetbrainsChat        `json:"chat"`
	Parameters *JetbrainsParameters `json:"parameters,omitempty"`
}

// JetbrainsChat holds the message history in a JetBrains API request.
type JetbrainsChat struct {
	Messages []JetbrainsMessage `json:"messages"`
}

// JetbrainsParameters holds tool definitions for JetBrains API requests.
type JetbrainsParameters struct {
	Data []JetbrainsData `json:"data"`
}

// JetbrainsData represents a single parameter entry in JetBrains API requests.
type JetbrainsData struct {
	Type     string `json:"type"`
	FQDN     string `json:"fqdn,omitempty"`
	Value    string `json:"value,omitempty"`
	Modified int64  `json:"modified,omitempty"`
}

// AnthropicMessage represents a message in the Anthropic Messages API format.
type AnthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// AnthropicContentBlock represents a content block in an Anthropic message.
type AnthropicContentBlock struct {
	Type  string         `json:"type"`
	Text  string         `json:"text,omitempty"`
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`
}

// FlexibleString supports string or array form of system field.
type FlexibleString string

// UnmarshalJSON custom JSON parsing, supports string and array formats.
func (fs *FlexibleString) UnmarshalJSON(data []byte) error {
	var str string
	if err := sonic.Unmarshal(data, &str); err == nil {
		*fs = FlexibleString(str)
		return nil
	}

	var arr []map[string]any
	if err := sonic.Unmarshal(data, &arr); err == nil {
		var parts []string
		for _, item := range arr {
			if textVal, ok := item["text"]; ok {
				if text, ok := textVal.(string); ok {
					parts = append(parts, text)
				}
			} else if typeVal, ok := item["type"]; ok && typeVal == ContentBlockTypeText {
				if textVal, ok := item["content"]; ok {
					if text, ok := textVal.(string); ok {
						parts = append(parts, text)
					}
				}
			}
		}
		*fs = FlexibleString(strings.Join(parts, ""))
		return nil
	}

	return fmt.Errorf("invalid system field format")
}

// AnthropicTool represents a tool definition in the Anthropic Messages API.
type AnthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

// AnthropicMessagesRequest is the Anthropic Messages API request payload.
type AnthropicMessagesRequest struct {
	Model         string             `json:"model"`
	MaxTokens     int                `json:"max_tokens"`
	Messages      []AnthropicMessage `json:"messages"`
	System        FlexibleString     `json:"system,omitempty"`
	Temperature   *float64           `json:"temperature,omitempty"`
	TopP          *float64           `json:"top_p,omitempty"`
	TopK          *int               `json:"top_k,omitempty"`
	Stream        *bool              `json:"stream,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
	Tools         []AnthropicTool    `json:"tools,omitempty"`
	ToolChoice    any                `json:"tool_choice,omitempty"`
}

// AnthropicUsage holds token usage information for Anthropic API responses.
type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// AnthropicMessagesResponse is the Anthropic Messages API non-streaming response.
type AnthropicMessagesResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"`
	Role         string                  `json:"role"`
	Content      []AnthropicContentBlock `json:"content"`
	Model        string                  `json:"model"`
	StopReason   string                  `json:"stop_reason,omitempty"`
	StopSequence *string                 `json:"stop_sequence,omitempty"`
	Usage        AnthropicUsage          `json:"usage"`
}

// AnthropicStreamResponse is the Anthropic Messages API streaming response event.
type AnthropicStreamResponse struct {
	Type         string                 `json:"type"`
	Index        *int                   `json:"index,omitempty"`
	ContentBlock *AnthropicContentBlock `json:"content_block,omitempty"`
	Delta        *struct {
		Type string `json:"type,omitempty"`
		Text string `json:"text,omitempty"`
	} `json:"delta,omitempty"`
	Message *AnthropicMessagesResponse `json:"message,omitempty"`
	Usage   *AnthropicUsage            `json:"usage,omitempty"`
}

// ToolInfo holds tool information.
type ToolInfo struct {
	ID     string
	Name   string
	Result string
}
