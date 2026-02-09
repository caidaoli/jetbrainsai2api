package config

import (
	"os"
	"testing"

	"jetbrainsai2api/internal/core"
)

func TestLoadModelsConfig_ValidJSON(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "models_test_*.json")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := `{"models":{"gpt-4":"openai-gpt-4","claude-3":"anthropic-claude-3"}}`
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("写入临时文件失败: %v", err)
	}
	tmpFile.Close()

	config, err := LoadModelsConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadModelsConfig failed: %v", err)
	}

	if len(config.Models) != 2 {
		t.Errorf("Expected 2 models, got %d", len(config.Models))
	}

	if config.Models["gpt-4"] != "openai-gpt-4" {
		t.Errorf("Expected 'openai-gpt-4' for 'gpt-4', got '%s'", config.Models["gpt-4"])
	}
}

func TestLoadModelsConfig_ArrayFormat(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "models_test_*.json")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := `["model-a","model-b"]`
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("写入临时文件失败: %v", err)
	}
	tmpFile.Close()

	config, err := LoadModelsConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadModelsConfig failed: %v", err)
	}

	if len(config.Models) != 2 {
		t.Errorf("Expected 2 models, got %d", len(config.Models))
	}

	if config.Models["model-a"] != "model-a" {
		t.Errorf("Array format: Expected identity mapping for 'model-a'")
	}
}

func TestLoadModelsConfig_NonExistentFile(t *testing.T) {
	_, err := LoadModelsConfig("/tmp/nonexistent_models_file_12345.json")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

func TestLoadModels(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "models_test_*.json")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := `{"models":{"gpt-4o":"openai-gpt-4o","claude-3":"anthropic-claude-3"}}`
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("写入临时文件失败: %v", err)
	}
	tmpFile.Close()

	modelsData, err := LoadModels(tmpFile.Name(), &core.NopLogger{})
	if err != nil {
		t.Fatalf("LoadModels failed: %v", err)
	}

	if len(modelsData.Data) != 2 {
		t.Errorf("Expected 2 model items, got %d", len(modelsData.Data))
	}
}

func TestGetModelItem(t *testing.T) {
	modelsData := core.ModelsData{
		Data: []core.ModelInfo{
			{ID: "gpt-4", Object: "model", OwnedBy: "jetbrains-ai"},
			{ID: "claude-3", Object: "model", OwnedBy: "jetbrains-ai"},
		},
	}

	model := GetModelItem(modelsData, "gpt-4")
	if model == nil {
		t.Fatal("Expected to find gpt-4")
	}
	if model.ID != "gpt-4" {
		t.Errorf("Expected ID 'gpt-4', got '%s'", model.ID)
	}

	model = GetModelItem(modelsData, "nonexistent")
	if model != nil {
		t.Error("Expected nil for nonexistent model")
	}
}

func TestDefaultHTTPClientSettings(t *testing.T) {
	settings := DefaultHTTPClientSettings()
	if settings.MaxIdleConns <= 0 {
		t.Error("MaxIdleConns should be positive")
	}
	if settings.RequestTimeout <= 0 {
		t.Error("RequestTimeout should be positive")
	}
}
