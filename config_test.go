package main

import (
	"testing"
)

// TestDefaultHTTPClientSettings 测试默认HTTP客户端设置
func TestDefaultHTTPClientSettings(t *testing.T) {
	settings := DefaultHTTPClientSettings()

	// 验证各项设置不为零值
	if settings.MaxIdleConns <= 0 {
		t.Errorf("MaxIdleConns 应大于0，实际: %d", settings.MaxIdleConns)
	}

	if settings.MaxIdleConnsPerHost <= 0 {
		t.Errorf("MaxIdleConnsPerHost 应大于0，实际: %d", settings.MaxIdleConnsPerHost)
	}

	if settings.MaxConnsPerHost <= 0 {
		t.Errorf("MaxConnsPerHost 应大于0，实际: %d", settings.MaxConnsPerHost)
	}

	if settings.IdleConnTimeout <= 0 {
		t.Errorf("IdleConnTimeout 应大于0，实际: %v", settings.IdleConnTimeout)
	}

	if settings.TLSHandshakeTimeout <= 0 {
		t.Errorf("TLSHandshakeTimeout 应大于0，实际: %v", settings.TLSHandshakeTimeout)
	}

	if settings.RequestTimeout <= 0 {
		t.Errorf("RequestTimeout 应大于0，实际: %v", settings.RequestTimeout)
	}

	// 验证与常量一致
	if settings.MaxIdleConns != HTTPMaxIdleConns {
		t.Errorf("MaxIdleConns 应等于 HTTPMaxIdleConns(%d)，实际: %d",
			HTTPMaxIdleConns, settings.MaxIdleConns)
	}
}

// TestGetInternalModelName 测试获取内部模型名
func TestGetInternalModelName(t *testing.T) {
	config := ModelsConfig{
		Models: map[string]string{
			"gpt-4":         "internal-gpt4",
			"gpt-3.5-turbo": "internal-gpt35",
			"claude-3":      "internal-claude3",
		},
	}

	tests := []struct {
		name     string
		modelID  string
		expected string
	}{
		{
			name:     "已映射的模型",
			modelID:  "gpt-4",
			expected: "internal-gpt4",
		},
		{
			name:     "另一个已映射的模型",
			modelID:  "claude-3",
			expected: "internal-claude3",
		},
		{
			name:     "未映射的模型返回原ID",
			modelID:  "unknown-model",
			expected: "unknown-model",
		},
		{
			name:     "空模型ID",
			modelID:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getInternalModelName(config, tt.modelID)
			if result != tt.expected {
				t.Errorf("期望 '%s'，实际 '%s'", tt.expected, result)
			}
		})
	}
}

// TestGetInternalModelName_EmptyConfig 测试空配置
func TestGetInternalModelName_EmptyConfig(t *testing.T) {
	config := ModelsConfig{
		Models: map[string]string{},
	}

	result := getInternalModelName(config, "any-model")
	if result != "any-model" {
		t.Errorf("空配置时应返回原模型ID，期望 'any-model'，实际 '%s'", result)
	}
}

// TestGetInternalModelName_NilModels 测试nil模型映射
func TestGetInternalModelName_NilModels(t *testing.T) {
	config := ModelsConfig{
		Models: nil,
	}

	result := getInternalModelName(config, "any-model")
	if result != "any-model" {
		t.Errorf("nil映射时应返回原模型ID，期望 'any-model'，实际 '%s'", result)
	}
}
