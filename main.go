package main

import (
	"os"

	"github.com/joho/godotenv"
)

func main() {
	// 加载 .env 文件
	if err := godotenv.Load(); err != nil {
		println("No .env file found, using system environment variables")
	}

	// 创建日志实例（依赖注入，不使用全局变量）
	logger := createLogger()
	logger.Info("Logger initialized")

	// 创建存储实例（依赖注入，不使用全局变量）
	storageInstance, err := initStorage()
	if err != nil {
		logger.Fatal("Failed to initialize storage: %v", err)
	}
	defer storageInstance.Close()

	// 从环境变量加载服务器配置
	config, err := loadServerConfigFromEnv()
	if err != nil {
		logger.Fatal("Failed to load server configuration: %v", err)
	}

	// 注入依赖到配置
	config.Storage = storageInstance
	config.Logger = logger

	// 创建服务器实例
	server, err := NewServer(config)
	if err != nil {
		logger.Fatal("Failed to create server: %v", err)
	}

	// 运行服务器（包含优雅关闭）
	logger.Info("Starting server on port %s", config.Port)
	if err := server.Run(); err != nil {
		logger.Fatal("Server error: %v", err)
	}
}

// loadServerConfigFromEnv 从环境变量加载服务器配置
func loadServerConfigFromEnv() (ServerConfig, error) {
	// 加载客户端 API keys
	clientAPIKeys := parseEnvList(os.Getenv("CLIENT_API_KEYS"))
	if len(clientAPIKeys) == 0 {
		Warn("CLIENT_API_KEYS environment variable is empty")
	} else {
		Info("Loaded %d client API keys", len(clientAPIKeys))
	}

	// 加载 JetBrains 账户
	jetbrainsAccounts := loadJetbrainsAccountsFromEnv()
	if len(jetbrainsAccounts) == 0 {
		Warn("No JetBrains accounts configured")
	} else {
		Info("Loaded %d JetBrains accounts", len(jetbrainsAccounts))
	}

	// 获取端口和 Gin 模式
	port := getEnvWithDefault("PORT", DefaultPort)
	ginMode := getEnvWithDefault("GIN_MODE", DefaultGinMode)

	config := ServerConfig{
		Port:               port,
		GinMode:            ginMode,
		ClientAPIKeys:      clientAPIKeys,
		JetbrainsAccounts:  jetbrainsAccounts,
		ModelsConfigPath:   DefaultModelsConfigPath,
		HTTPClientSettings: DefaultHTTPClientSettings(),
	}

	return config, nil
}

// loadJetbrainsAccountsFromEnv 从环境变量加载 JetBrains 账户
func loadJetbrainsAccountsFromEnv() []JetbrainsAccount {
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

	return accounts
}
