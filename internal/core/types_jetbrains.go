package core

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

// ToolInfo holds tool information.
type ToolInfo struct {
	ID     string
	Name   string
	Result string
}
