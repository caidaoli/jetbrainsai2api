package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
)

// JetbrainsQuotaResponse defines the structure for the JetBrains quota API response
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

// Data structures
type RequestStats struct {
	TotalRequests      int64           `json:"total_requests"`
	SuccessfulRequests int64           `json:"successful_requests"`
	FailedRequests     int64           `json:"failed_requests"`
	TotalResponseTime  int64           `json:"total_response_time"`
	LastRequestTime    time.Time       `json:"last_request_time"`
	RequestHistory     []RequestRecord `json:"request_history"`
}

type RequestRecord struct {
	Timestamp    time.Time `json:"timestamp"`
	Success      bool      `json:"success"`
	ResponseTime int64     `json:"response_time"`
	Model        string    `json:"model"`
	Account      string    `json:"account"`
}

type PeriodStats struct {
	Requests        int64   `json:"requests"`
	SuccessRate     float64 `json:"successRate"`
	AvgResponseTime int64   `json:"avgResponseTime"`
	QPS             float64 `json:"qps"`
}

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

type JetbrainsAccount struct {
	LicenseID      string     `json:"licenseId,omitempty"`
	Authorization  string     `json:"authorization,omitempty"`
	JWT            string     `json:"jwt,omitempty"`
	LastUpdated    float64    `json:"last_updated"`
	HasQuota       bool       `json:"has_quota"`
	LastQuotaCheck float64    `json:"last_quota_check"`
	ExpiryTime     time.Time  `json:"expiry_time"`
	mu             sync.Mutex `json:"-"` // 账户级互斥锁，用于 JWT 刷新和配额检查
}

type ModelInfo struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type ModelsData struct {
	Data []ModelInfo `json:"data"`
}

type ModelList struct {
	Object string      `json:"object"`
	Data   []ModelInfo `json:"data"`
}

type ModelsConfig struct {
	Models map[string]string `json:"models"`
}

type ChatMessage struct {
	Role       string     `json:"role"`
	Content    any        `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

type Function struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

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

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

// JetBrains专用的工具格式
type JetbrainsToolDefinition struct {
	Name        string                         `json:"name"`
	Description string                         `json:"description,omitempty"`
	Parameters  JetbrainsToolParametersWrapper `json:"parameters"`
}

type JetbrainsToolParametersWrapper struct {
	Schema map[string]any `json:"schema"`
}

type ChatCompletionChoice struct {
	Message      ChatMessage `json:"message"`
	Index        int         `json:"index"`
	FinishReason string      `json:"finish_reason"`
}

type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   map[string]int         `json:"usage"`
}

type StreamChoice struct {
	Delta        map[string]any `json:"delta"`
	Index        int            `json:"index"`
	FinishReason *string        `json:"finish_reason"`
}

type StreamResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []StreamChoice `json:"choices"`
}

// JetbrainsMessage updated to support v8 API format including tool calls
type JetbrainsMessage struct {
	Type         string                 `json:"type"`
	Content      string                 `json:"content,omitempty"`
	MediaType    string                 `json:"mediaType,omitempty"` // New field for v8 image support
	Data         string                 `json:"data,omitempty"`      // New field for v8 image data
	FunctionCall *JetbrainsFunctionCall `json:"functionCall,omitempty"`
	FunctionName string                 `json:"functionName,omitempty"`
	// New fields for v8 tool calls
	ID       string `json:"id,omitempty"`       // Tool call ID
	ToolName string `json:"toolName,omitempty"` // Tool name
	Result   string `json:"result,omitempty"`   // Tool result
}

type JetbrainsFunctionCall struct {
	FunctionName string `json:"functionName"`
	Content      string `json:"content"`
}

type JetbrainsPayload struct {
	Prompt     string               `json:"prompt"`
	Profile    string               `json:"profile"`
	Chat       JetbrainsChat        `json:"chat"`
	Parameters *JetbrainsParameters `json:"parameters,omitempty"`
}

type JetbrainsChat struct {
	Messages []JetbrainsMessage `json:"messages"`
}

type JetbrainsParameters struct {
	Data []JetbrainsData `json:"data"`
}

type JetbrainsData struct {
	Type     string `json:"type"`
	FQDN     string `json:"fqdn,omitempty"`
	Value    string `json:"value,omitempty"`
	Modified int64  `json:"modified,omitempty"`
}

// Anthropic Messages API 数据结构
type AnthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type AnthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// 工具调用字段 (tool_use 类型)
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`
	// 支持图像和其他内容类型
	Source *AnthropicImageSource `json:"source,omitempty"`
}

type AnthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

// FlexibleString 支持字符串或数组形式的system字段
type FlexibleString string

// UnmarshalJSON 自定义JSON解析，支持字符串和数组格式
func (fs *FlexibleString) UnmarshalJSON(data []byte) error {
	// 尝试解析为字符串
	var str string
	if err := sonic.Unmarshal(data, &str); err == nil {
		*fs = FlexibleString(str)
		return nil
	}

	// 尝试解析为数组格式（某些客户端可能发送数组）
	var arr []map[string]any
	if err := sonic.Unmarshal(data, &arr); err == nil {
		// 提取数组中的文本内容
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

type AnthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

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

type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

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

// 流式响应结构
type AnthropicStreamResponse struct {
	Type  string `json:"type"`
	Index *int   `json:"index,omitempty"`
	Delta *struct {
		Type string `json:"type,omitempty"`
		Text string `json:"text,omitempty"`
	} `json:"delta,omitempty"`
	Message *AnthropicMessagesResponse `json:"message,omitempty"`
	Usage   *AnthropicUsage            `json:"usage,omitempty"`
}
