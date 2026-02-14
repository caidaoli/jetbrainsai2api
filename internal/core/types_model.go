package core

// ModelInfo represents a single model entry in the models list.
type ModelInfo struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
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
