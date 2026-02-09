package config

import (
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"jetbrainsai2api/internal/account"
	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/util"

	"github.com/bytedance/sonic"
)

// ServerConfig server configuration
type ServerConfig struct {
	Port               string
	GinMode            string
	ClientAPIKeys      []string
	JetbrainsAccounts  []core.JetbrainsAccount
	ModelsConfigPath   string
	HTTPClientSettings HTTPClientSettings
	Storage            core.StorageInterface
	Logger             core.Logger
}

// HTTPClientSettings HTTP client configuration
type HTTPClientSettings struct {
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	MaxConnsPerHost     int
	IdleConnTimeout     time.Duration
	TLSHandshakeTimeout time.Duration
	RequestTimeout      time.Duration
}

// DefaultHTTPClientSettings default HTTP client settings
func DefaultHTTPClientSettings() HTTPClientSettings {
	return HTTPClientSettings{
		MaxIdleConns:        core.HTTPMaxIdleConns,
		MaxIdleConnsPerHost: core.HTTPMaxIdleConnsPerHost,
		MaxConnsPerHost:     core.HTTPMaxConnsPerHost,
		IdleConnTimeout:     core.HTTPIdleConnTimeout,
		TLSHandshakeTimeout: core.HTTPTLSHandshakeTimeout,
		RequestTimeout:      core.HTTPRequestTimeout,
	}
}

// LoadModels loads model data for API response
func LoadModels(path string, logger core.Logger) (core.ModelsData, error) {
	var result core.ModelsData

	config, err := LoadModelsConfig(path)
	if err != nil {
		return result, err
	}

	now := time.Now().Unix()
	modelKeys := make([]string, 0, len(config.Models))
	for modelKey := range config.Models {
		modelKeys = append(modelKeys, modelKey)
	}
	sort.Strings(modelKeys)

	for _, modelKey := range modelKeys {
		result.Data = append(result.Data, core.ModelInfo{
			ID:      modelKey,
			Object:  core.ModelObjectType,
			Created: now,
			OwnedBy: core.ModelOwner,
		})
	}

	logger.Info("Loaded %d models from %s", len(config.Models), path)
	return result, nil
}

// LoadModelsConfig loads model configuration mapping
func LoadModelsConfig(path string) (core.ModelsConfig, error) {
	var config core.ModelsConfig

	data, err := os.ReadFile(path) //nolint:gosec // G304: path from config, not user input
	if err != nil {
		return config, fmt.Errorf("failed to read %s: %w", path, err)
	}

	if err := sonic.Unmarshal(data, &config); err != nil {
		var modelIDs []string
		if err := sonic.Unmarshal(data, &modelIDs); err != nil {
			return config, fmt.Errorf("failed to parse %s: %w", path, err)
		}
		config.Models = make(map[string]string)
		for _, modelID := range modelIDs {
			config.Models[modelID] = modelID
		}
	}

	if config.Models == nil {
		config.Models = make(map[string]string)
	}

	return config, nil
}

// GetModelItem finds a model by ID
func GetModelItem(modelsData core.ModelsData, modelID string) *core.ModelInfo {
	for _, model := range modelsData.Data {
		if model.ID == modelID {
			return &model
		}
	}
	return nil
}

// GetModelsConfig loads both ModelsData and ModelsConfig
func GetModelsConfig(path string, logger core.Logger) (core.ModelsData, core.ModelsConfig, error) {
	modelsData, err := LoadModels(path, logger)
	if err != nil {
		return modelsData, core.ModelsConfig{}, fmt.Errorf("failed to load models: %w", err)
	}

	modelsConfig, err := LoadModelsConfig(path)
	if err != nil {
		return modelsData, core.ModelsConfig{}, fmt.Errorf("failed to load model mappings: %w", err)
	}

	return modelsData, modelsConfig, nil
}

// LoadServerConfigFromEnv loads server config from environment variables
func LoadServerConfigFromEnv(logger core.Logger) (ServerConfig, error) {
	clientAPIKeys := util.ParseEnvList(os.Getenv("CLIENT_API_KEYS"))
	if len(clientAPIKeys) == 0 {
		logger.Warn("CLIENT_API_KEYS environment variable is empty")
	} else {
		logger.Info("Loaded %d client API keys", len(clientAPIKeys))
	}

	jetbrainsAccounts := LoadJetbrainsAccountsFromEnv()
	if len(jetbrainsAccounts) == 0 {
		logger.Warn("No JetBrains accounts configured")
	} else {
		logger.Info("Loaded %d JetBrains accounts", len(jetbrainsAccounts))
	}

	port := util.GetEnvWithDefault("PORT", core.DefaultPort)
	ginMode := util.GetEnvWithDefault("GIN_MODE", core.DefaultGinMode)

	config := ServerConfig{
		Port:               port,
		GinMode:            ginMode,
		ClientAPIKeys:      clientAPIKeys,
		JetbrainsAccounts:  jetbrainsAccounts,
		ModelsConfigPath:   core.DefaultModelsConfigPath,
		HTTPClientSettings: DefaultHTTPClientSettings(),
	}

	return config, nil
}

// LoadJetbrainsAccountsFromEnv loads JetBrains accounts from environment variables
func LoadJetbrainsAccountsFromEnv() []core.JetbrainsAccount {
	var accounts []core.JetbrainsAccount

	licenseIDs := util.ParseEnvList(os.Getenv("JETBRAINS_LICENSE_IDS"))
	authorizations := util.ParseEnvList(os.Getenv("JETBRAINS_AUTHORIZATIONS"))

	maxLen := len(licenseIDs)
	if len(authorizations) > maxLen {
		maxLen = len(authorizations)
	}

	for len(licenseIDs) < maxLen {
		licenseIDs = append(licenseIDs, "")
	}
	for len(authorizations) < maxLen {
		authorizations = append(authorizations, "")
	}

	for i := 0; i < maxLen; i++ {
		if licenseIDs[i] != "" && authorizations[i] != "" {
			accounts = append(accounts, core.JetbrainsAccount{
				LicenseID:      licenseIDs[i],
				Authorization:  authorizations[i],
				JWT:            "",
				LastUpdated:    0,
				HasQuota:       true,
				LastQuotaCheck: 0,
			})
		}
	}

	jwts := util.ParseEnvList(os.Getenv("JETBRAINS_JWTS"))
	for i, jwtToken := range jwts {
		if jwtToken != "" {
			accounts = append(accounts, core.JetbrainsAccount{
				LicenseID:      "",
				Authorization:  "",
				JWT:            jwtToken,
				LastUpdated:    float64(time.Now().Unix()),
				HasQuota:       true,
				LastQuotaCheck: 0,
			})

			if expiry, err := account.ParseJWTExpiry(jwtToken); err != nil {
				log.Printf("[WARN] 静态JWT #%d 解析失败: %v", i+1, err)
			} else {
				accounts[len(accounts)-1].ExpiryTime = expiry
				if time.Now().After(expiry) {
					log.Printf("[WARN] 静态JWT #%d 已过期 (过期时间: %s)", i+1, expiry.Format(time.RFC3339))
				} else {
					log.Printf("[INFO] 静态JWT #%d 将于 %s 过期", i+1, expiry.Format(time.RFC3339))
				}
			}
		}
	}

	if len(jwts) > 0 {
		log.Printf("[WARN] 使用静态JWT模式，JWT过期后将无法自动刷新，建议使用许可证模式")
	}

	return accounts
}
