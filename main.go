package main

import (
	"os"

	"github.com/joho/godotenv"
)

const (
	DefaultRequestTimeout = 5 * 60    // 5分钟（秒）
	QuotaCacheTime        = 3600      // 1小时（秒）
	JWTRefreshTime        = 12 * 3600 // 12小时（秒）
)

// 保留少量必要的全局变量（兼容性）
// 这些将在后续版本中完全移除
var (
	accountQuotaCache = make(map[string]*CachedQuotaInfo)
)

func main() {
	// 加载 .env 文件
	if err := godotenv.Load(); err != nil {
		println("No .env file found, using system environment variables")
	}

	// 初始化日志系统（必须最先调用）
	InitializeLogger()
	Info("Logger initialized")

	// 初始化存储并加载统计数据
	if err := initStorage(); err != nil {
		Fatal("Failed to initialize storage: %v", err)
	}
	loadStats()

	// 初始化请求触发的统计保存
	initRequestTriggeredSaving()

	// 从环境变量加载服务器配置
	config, err := loadServerConfigFromEnv()
	if err != nil {
		Fatal("Failed to load server configuration: %v", err)
	}

	// 创建服务器实例
	server, err := NewServer(config)
	if err != nil {
		Fatal("Failed to create server: %v", err)
	}

	// 运行服务器（包含优雅关闭）
	Info("Starting server on port %s", config.Port)
	if err := server.Run(); err != nil {
		Fatal("Server error: %v", err)
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
	port := getEnvWithDefault("PORT", "7860")
	ginMode := getEnvWithDefault("GIN_MODE", "release")

	config := ServerConfig{
		Port:               port,
		GinMode:            ginMode,
		ClientAPIKeys:      clientAPIKeys,
		JetbrainsAccounts:  jetbrainsAccounts,
		ModelsConfigPath:   "models.json",
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
			account := JetbrainsAccount{
				LicenseID:      licenseIDs[i],
				Authorization:  authorizations[i],
				JWT:            "",
				LastUpdated:    0,
				HasQuota:       true,
				LastQuotaCheck: 0,
			}
			accounts = append(accounts, account)
		}
	}

	return accounts
}
