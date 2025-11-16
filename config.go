package main

import (
	"fmt"
	"os"
	"time"

	"github.com/bytedance/sonic"
)

// loadModels loads model definitions from models.json
func loadModels() (ModelsData, error) {
	var result ModelsData

	data, err := os.ReadFile("models.json")
	if err != nil {
		return result, fmt.Errorf("failed to read models.json: %w", err)
	}

	var config ModelsConfig
	if err := sonic.Unmarshal(data, &config); err != nil {
		// Try old format (string array)
		var modelIDs []string
		if err := sonic.Unmarshal(data, &modelIDs); err != nil {
			return result, fmt.Errorf("failed to parse models.json: %w", err)
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
			Object:  "model",
			Created: now,
			OwnedBy: "jetbrains-ai",
		})
	}

	Info("Loaded %d models from models.json", len(config.Models))
	return result, nil
}

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
