package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/joho/godotenv"
)

// ==================== 配置结构定义 ====================

// Config 应用统一配置
// 整合所有配置项，支持从环境变量和文件加载
type Config struct {
	// 服务器配置
	Server ServerConfig

	// 账户配置
	Accounts []JetbrainsAccount

	// 模型配置
	Models ModelsConfig

	// 日志配置
	Logging LoggingConfig

	// 性能配置
	Performance PerformanceConfig
}

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

// LoggingConfig 日志配置
type LoggingConfig struct {
	DebugMode   bool
	DebugFile   string
	EnableDebug bool
}

// PerformanceConfig 性能配置
type PerformanceConfig struct {
	CacheCapacity          int
	CacheCleanupInterval   time.Duration
	MetricsMonitorInterval time.Duration
	MetricsWindowDuration  time.Duration
}

// ==================== 配置加载 ====================

// LoadConfig 从环境变量加载完整配置
func LoadConfig() (*Config, error) {
	// 尝试加载 .env 文件
	if err := godotenv.Load(); err != nil {
		Info("No .env file found, using system environment variables")
	}

	config := &Config{
		Server:      loadServerConfig(),
		Accounts:    loadJetbrainsAccounts(),
		Logging:     loadLoggingConfig(),
		Performance: loadPerformanceConfig(),
	}

	// 加载模型配置
	modelsConfig, err := loadModelsConfig(config.Server.ModelsConfigPath)
	if err != nil {
		return nil, ErrConfigLoadFailed("models", err)
	}
	config.Models = modelsConfig

	// 验证配置
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	return config, nil
}

// loadServerConfig 加载服务器配置
func loadServerConfig() ServerConfig {
	// 加载客户端 API keys
	clientAPIKeys := parseEnvList(os.Getenv("CLIENT_API_KEYS"))
	if len(clientAPIKeys) == 0 {
		Warn("CLIENT_API_KEYS environment variable is empty")
	} else {
		Info("Loaded %d client API keys", len(clientAPIKeys))
	}

	// 加载统计认证配置
	statsAuthEnabled := os.Getenv("STATS_AUTH_ENABLED") == "true"
	if statsAuthEnabled {
		Info("Stats authentication enabled")
	}

	return ServerConfig{
		Port:               getEnvWithDefault("PORT", DefaultPort),
		GinMode:            getEnvWithDefault("GIN_MODE", DefaultGinMode),
		ClientAPIKeys:      clientAPIKeys,
		ModelsConfigPath:   DefaultModelsConfigPath,
		HTTPClientSettings: DefaultHTTPClientSettings(),
		StatsAuthEnabled:   statsAuthEnabled,
	}
}

// loadJetbrainsAccounts 从环境变量加载 JetBrains 账户
func loadJetbrainsAccounts() []JetbrainsAccount {
	licenseIDs := parseEnvList(os.Getenv("JETBRAINS_LICENSE_IDS"))
	authorizations := parseEnvList(os.Getenv("JETBRAINS_AUTHORIZATIONS"))

	maxLen := len(licenseIDs)
	if len(authorizations) > maxLen {
		maxLen = len(authorizations)
	}

	// 填充到相同长度
	for len(licenseIDs) < maxLen {
		licenseIDs = append(licenseIDs, "")
	}
	for len(authorizations) < maxLen {
		authorizations = append(authorizations, "")
	}

	var accounts []JetbrainsAccount
	for i := 0; i < maxLen; i++ {
		if licenseIDs[i] != "" && authorizations[i] != "" {
			// 直接 append 以避免复制 mutex 的警告
			accounts = append(accounts, JetbrainsAccount{
				LicenseID:      licenseIDs[i],
				Authorization:  authorizations[i],
				JWT:            "",
				LastUpdated:    0,
				HasQuota:       true,
				LastQuotaCheck: 0,
			})
		}
	}

	if len(accounts) == 0 {
		Warn("No JetBrains accounts configured")
	} else {
		Info("Loaded %d JetBrains accounts", len(accounts))
	}

	return accounts
}

// loadLoggingConfig 加载日志配置
func loadLoggingConfig() LoggingConfig {
	debugMode := strings.ToLower(os.Getenv("GIN_MODE")) == GinModeDebug
	return LoggingConfig{
		DebugMode:   debugMode,
		DebugFile:   os.Getenv("DEBUG_FILE"),
		EnableDebug: debugMode,
	}
}

// loadPerformanceConfig 加载性能配置
func loadPerformanceConfig() PerformanceConfig {
	return PerformanceConfig{
		CacheCapacity:          CacheDefaultCapacity,
		CacheCleanupInterval:   CacheCleanupInterval,
		MetricsMonitorInterval: MetricsMonitorInterval,
		MetricsWindowDuration:  MetricsWindowDuration,
	}
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

// ==================== 配置验证 ====================

// validateConfig 验证配置完整性
func validateConfig(config *Config) error {
	// 验证服务器配置
	if config.Server.Port == "" {
		return ErrInvalidConfig("server.port", "port cannot be empty")
	}

	// 验证账户配置
	if len(config.Accounts) == 0 {
		return ErrNoAccountsConfigured()
	}

	// 验证模型配置
	if len(config.Models.Models) == 0 {
		return ErrInvalidConfig("models", "no models configured")
	}

	return nil
}

// ==================== 模型配置加载 ====================

// loadModelsConfig 加载模型配置文件
func loadModelsConfig(path string) (ModelsConfig, error) {
	var config ModelsConfig

	data, err := os.ReadFile(path)
	if err != nil {
		return config, fmt.Errorf("failed to read models config: %w", err)
	}

	if err := sonic.Unmarshal(data, &config); err != nil {
		// Try old format (string array)
		var modelIDs []string
		if err := sonic.Unmarshal(data, &modelIDs); err != nil {
			return config, fmt.Errorf("failed to parse models config: %w", err)
		}
		// Convert to new format
		config.Models = make(map[string]string)
		for _, modelID := range modelIDs {
			config.Models[modelID] = modelID
		}
	}

	if len(config.Models) == 0 {
		return config, fmt.Errorf("no models found in config")
	}

	Info("Loaded %d models from %s", len(config.Models), path)
	return config, nil
}

// loadModels 加载模型数据（用于API响应）
func loadModels() (ModelsData, error) {
	var result ModelsData

	data, err := os.ReadFile(DefaultModelsConfigPath)
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
			Object:  ModelObjectType,
			Created: now,
			OwnedBy: ModelOwner,
		})
	}

	Info("Loaded %d models from models.json", len(config.Models))
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
