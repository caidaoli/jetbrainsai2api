package main

import (
	"fmt"
	"os"
	"time"

	"github.com/bytedance/sonic"
)

// ==================== 配置结构定义 ====================

// ServerConfig 服务器配置
type ServerConfig struct {
	Port               string
	GinMode            string
	ClientAPIKeys      []string
	JetbrainsAccounts  []JetbrainsAccount // 账户列表
	ModelsConfigPath   string
	HTTPClientSettings HTTPClientSettings
	StatsAuthEnabled   bool             // 是否启用统计端点认证
	Storage            StorageInterface // 存储实例（依赖注入）
	Logger             Logger           // 日志实例（依赖注入）
}

// HTTPClientSettings HTTP客户端配置
type HTTPClientSettings struct {
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	MaxConnsPerHost     int
	IdleConnTimeout     time.Duration
	TLSHandshakeTimeout time.Duration
	RequestTimeout      time.Duration
}

// DefaultHTTPClientSettings 默认HTTP客户端配置
func DefaultHTTPClientSettings() HTTPClientSettings {
	return HTTPClientSettings{
		MaxIdleConns:        HTTPMaxIdleConns,
		MaxIdleConnsPerHost: HTTPMaxIdleConnsPerHost,
		MaxConnsPerHost:     HTTPMaxConnsPerHost,
		IdleConnTimeout:     HTTPIdleConnTimeout,
		TLSHandshakeTimeout: HTTPTLSHandshakeTimeout,
		RequestTimeout:      HTTPRequestTimeout,
	}
}

// ==================== 模型配置加载 ====================

// loadModels 加载模型数据（用于API响应）
func loadModels(path string) (ModelsData, error) {
	var result ModelsData

	data, err := os.ReadFile(path)
	if err != nil {
		return result, fmt.Errorf("failed to read %s: %w", path, err)
	}

	var config ModelsConfig
	if err := sonic.Unmarshal(data, &config); err != nil {
		// Try old format (string array)
		var modelIDs []string
		if err := sonic.Unmarshal(data, &modelIDs); err != nil {
			return result, fmt.Errorf("failed to parse %s: %w", path, err)
		}
		// Convert to new format
		config.Models = make(map[string]string)
		for _, modelID := range modelIDs {
			config.Models[modelID] = modelID
		}
	}

	now := time.Now().Unix()
	for modelKey := range config.Models {
		result.Data = append(result.Data, ModelInfo{
			ID:      modelKey,
			Object:  ModelObjectType,
			Created: now,
			OwnedBy: ModelOwner,
		})
	}

	Info("Loaded %d models from %s", len(config.Models), path)
	return result, nil
}

// ==================== 辅助函数 ====================

// getInternalModelName 获取内部模型名称（通过配置映射）
func getInternalModelName(config ModelsConfig, modelID string) string {
	if internalModel, exists := config.Models[modelID]; exists {
		return internalModel
	}
	return modelID
}

// getModelItem 从模型数据中查找指定 ID 的模型
func getModelItem(modelsData ModelsData, modelID string) *ModelInfo {
	for _, model := range modelsData.Data {
		if model.ID == modelID {
			return &model
		}
	}
	return nil
}
