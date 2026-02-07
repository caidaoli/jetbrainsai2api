package main

import (
	"os"
	"strings"
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

// TestLoadModels 测试模型加载函数
func TestLoadModels(t *testing.T) {
	tests := []struct {
		name        string
		fileContent string
		wantErr     bool
		errContains string
		validate    func(t *testing.T, result ModelsData)
	}{
		{
			name:        "新格式JSON(map)",
			fileContent: `{"models":{"gpt-4":"internal-gpt4","claude-3":"internal-claude3"}}`,
			wantErr:     false,
			validate: func(t *testing.T, result ModelsData) {
				if len(result.Data) != 2 {
					t.Errorf("期望加载2个模型，实际 %d", len(result.Data))
				}
				modelIDs := make(map[string]bool)
				for _, model := range result.Data {
					modelIDs[model.ID] = true
					if model.Object != ModelObjectType {
						t.Errorf("模型 %s 的 Object 应为 '%s'，实际 '%s'", model.ID, ModelObjectType, model.Object)
					}
					if model.OwnedBy != ModelOwner {
						t.Errorf("模型 %s 的 OwnedBy 应为 '%s'，实际 '%s'", model.ID, ModelOwner, model.OwnedBy)
					}
					if model.Created <= 0 {
						t.Errorf("模型 %s 的 Created 时间戳应大于0", model.ID)
					}
				}
				if !modelIDs["gpt-4"] || !modelIDs["claude-3"] {
					t.Errorf("应包含 gpt-4 和 claude-3 模型")
				}
			},
		},
		{
			name:        "旧格式JSON(数组)",
			fileContent: `["gpt-4","gpt-3.5-turbo","claude-3"]`,
			wantErr:     false,
			validate: func(t *testing.T, result ModelsData) {
				if len(result.Data) != 3 {
					t.Errorf("期望加载3个模型，实际 %d", len(result.Data))
				}
				modelIDs := make(map[string]bool)
				for _, model := range result.Data {
					modelIDs[model.ID] = true
					if model.Object != ModelObjectType {
						t.Errorf("模型 %s 的 Object 应为 '%s'，实际 '%s'", model.ID, ModelObjectType, model.Object)
					}
					if model.OwnedBy != ModelOwner {
						t.Errorf("模型 %s 的 OwnedBy 应为 '%s'，实际 '%s'", model.ID, ModelOwner, model.OwnedBy)
					}
				}
				if !modelIDs["gpt-4"] || !modelIDs["gpt-3.5-turbo"] || !modelIDs["claude-3"] {
					t.Errorf("应包含 gpt-4, gpt-3.5-turbo 和 claude-3 模型")
				}
			},
		},
		{
			name:        "空模型列表(新格式)",
			fileContent: `{"models":{}}`,
			wantErr:     false,
			validate: func(t *testing.T, result ModelsData) {
				if len(result.Data) != 0 {
					t.Errorf("期望加载0个模型，实际 %d", len(result.Data))
				}
			},
		},
		{
			name:        "空模型列表(旧格式)",
			fileContent: `[]`,
			wantErr:     false,
			validate: func(t *testing.T, result ModelsData) {
				if len(result.Data) != 0 {
					t.Errorf("期望加载0个模型，实际 %d", len(result.Data))
				}
			},
		},
		{
			name:        "无效JSON",
			fileContent: `{invalid json}`,
			wantErr:     true,
			errContains: "failed to parse",
		},
		{
			name:        "既不是map也不是数组",
			fileContent: `"just a string"`,
			wantErr:     true,
			errContains: "failed to parse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建临时文件
			tmpFile, err := os.CreateTemp("", "models_*.json")
			if err != nil {
				t.Fatalf("创建临时文件失败: %v", err)
			}
			defer func() { _ = os.Remove(tmpFile.Name()) }()

			// 写入测试内容
			if _, err := tmpFile.WriteString(tt.fileContent); err != nil {
				t.Fatalf("写入临时文件失败: %v", err)
			}
			_ = tmpFile.Close()

			// 执行测试
			result, err := loadModels(tmpFile.Name())

			// 验证错误
			if tt.wantErr {
				if err == nil {
					t.Errorf("期望返回错误，但成功了")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("期望错误包含 '%s'，实际错误: %v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Errorf("不期望返回错误，实际: %v", err)
				return
			}

			// 验证结果
			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

// TestLoadModels_FileNotFound 测试文件不存在的情况
func TestLoadModels_FileNotFound(t *testing.T) {
	result, err := loadModels("/nonexistent/path/models.json")

	if err == nil {
		t.Errorf("期望返回错误，但成功了")
	}

	if !contains(err.Error(), "failed to read") {
		t.Errorf("期望错误包含 'failed to read'，实际: %v", err)
	}

	if len(result.Data) != 0 {
		t.Errorf("文件不存在时应返回空结果，实际返回了 %d 个模型", len(result.Data))
	}
}

// TestDefaultModelsConfigContainsGPT4ORegression 防止默认配置丢失 gpt-4o 映射
func TestDefaultModelsConfigContainsGPT4ORegression(t *testing.T) {
	modelsData, modelsConfig, err := GetModelsConfig(DefaultModelsConfigPath)
	if err != nil {
		t.Fatalf("加载默认模型配置失败: %v", err)
	}

	if mapped := getInternalModelName(modelsConfig, "gpt-4o"); mapped != "openai-gpt-4o" {
		t.Fatalf("gpt-4o 映射错误，期望 'openai-gpt-4o'，实际 '%s'", mapped)
	}

	if model := getModelItem(modelsData, "gpt-4o"); model == nil {
		t.Fatalf("/v1/models 语义回归：默认模型列表缺少 gpt-4o")
	}
}

// TestGetModelsConfig_OldFormatCompatibility 确保旧数组格式与新格式均可用于完整配置加载
func TestGetModelsConfig_OldFormatCompatibility(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "models_old_format_*.json")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.WriteString(`["gpt-4o","gpt-4.1"]`); err != nil {
		t.Fatalf("写入临时文件失败: %v", err)
	}
	_ = tmpFile.Close()

	modelsData, modelsConfig, err := GetModelsConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("GetModelsConfig 应兼容旧格式，实际报错: %v", err)
	}

	if len(modelsData.Data) != 2 {
		t.Fatalf("期望加载 2 个模型，实际 %d", len(modelsData.Data))
	}

	if mapped := getInternalModelName(modelsConfig, "gpt-4o"); mapped != "gpt-4o" {
		t.Fatalf("旧格式映射应回退为同名，实际 '%s'", mapped)
	}
}

// TestGetModelItem 测试从模型数据中查找模型
func TestGetModelItem(t *testing.T) {
	modelsData := ModelsData{
		Data: []ModelInfo{
			{
				ID:      "gpt-4",
				Object:  ModelObjectType,
				Created: 1234567890,
				OwnedBy: ModelOwner,
			},
			{
				ID:      "claude-3",
				Object:  ModelObjectType,
				Created: 1234567891,
				OwnedBy: ModelOwner,
			},
			{
				ID:      "gpt-3.5-turbo",
				Object:  ModelObjectType,
				Created: 1234567892,
				OwnedBy: ModelOwner,
			},
		},
	}

	tests := []struct {
		name     string
		data     ModelsData
		modelID  string
		wantNil  bool
		validate func(t *testing.T, result *ModelInfo)
	}{
		{
			name:    "找到第一个模型",
			data:    modelsData,
			modelID: "gpt-4",
			wantNil: false,
			validate: func(t *testing.T, result *ModelInfo) {
				if result.ID != "gpt-4" {
					t.Errorf("期望模型ID为 'gpt-4'，实际 '%s'", result.ID)
				}
				if result.Created != 1234567890 {
					t.Errorf("期望Created为 1234567890，实际 %d", result.Created)
				}
			},
		},
		{
			name:    "找到中间的模型",
			data:    modelsData,
			modelID: "claude-3",
			wantNil: false,
			validate: func(t *testing.T, result *ModelInfo) {
				if result.ID != "claude-3" {
					t.Errorf("期望模型ID为 'claude-3'，实际 '%s'", result.ID)
				}
				if result.Created != 1234567891 {
					t.Errorf("期望Created为 1234567891，实际 %d", result.Created)
				}
			},
		},
		{
			name:    "找到最后一个模型",
			data:    modelsData,
			modelID: "gpt-3.5-turbo",
			wantNil: false,
			validate: func(t *testing.T, result *ModelInfo) {
				if result.ID != "gpt-3.5-turbo" {
					t.Errorf("期望模型ID为 'gpt-3.5-turbo'，实际 '%s'", result.ID)
				}
			},
		},
		{
			name:    "找不到模型",
			data:    modelsData,
			modelID: "nonexistent-model",
			wantNil: true,
		},
		{
			name:    "空模型ID",
			data:    modelsData,
			modelID: "",
			wantNil: true,
		},
		{
			name: "空模型列表",
			data: ModelsData{
				Data: []ModelInfo{},
			},
			modelID: "gpt-4",
			wantNil: true,
		},
		{
			name: "nil模型列表",
			data: ModelsData{
				Data: nil,
			},
			modelID: "gpt-4",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getModelItem(tt.data, tt.modelID)

			if tt.wantNil {
				if result != nil {
					t.Errorf("期望返回 nil，实际返回了模型: %+v", result)
				}
				return
			}

			if result == nil {
				t.Errorf("期望找到模型，实际返回 nil")
				return
			}

			// 验证结果
			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

// 辅助函数：字符串包含检查
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// TestLoadModels_ShouldReturnStableSortedOrder 确保模型列表顺序稳定，避免 map 迭代随机性
func TestLoadModels_ShouldReturnStableSortedOrder(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "models_sorted_order_*.json")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	content := `{
		"models": {
			"z-model": "internal-z",
			"a-model": "internal-a",
			"m-model": "internal-m"
		}
	}`
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("写入临时文件失败: %v", err)
	}
	_ = tmpFile.Close()

	result, err := loadModels(tmpFile.Name())
	if err != nil {
		t.Fatalf("loadModels 失败: %v", err)
	}
	if len(result.Data) != 3 {
		t.Fatalf("期望 3 个模型，实际 %d", len(result.Data))
	}

	expected := []string{"a-model", "m-model", "z-model"}
	for i, modelID := range expected {
		if result.Data[i].ID != modelID {
			t.Fatalf("模型顺序错误: index=%d 期望=%s 实际=%s", i, modelID, result.Data[i].ID)
		}
	}
}
