package main

import (
	"log"
	"os"
	"time"

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
	defer func() { _ = storageInstance.Close() }()

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
	var accounts []JetbrainsAccount

	// 方式1：许可证模式（推荐，支持自动刷新JWT）
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

	for i := 0; i < maxLen; i++ {
		if licenseIDs[i] != "" && authorizations[i] != "" {
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

	// 方式2：静态JWT模式（不推荐，JWT会过期且无法自动刷新）
	jwts := parseEnvList(os.Getenv("JETBRAINS_JWTS"))
	for i, jwtToken := range jwts {
		if jwtToken != "" {
			// 直接 append 结构体字面量，避免复制 mutex (go vet 警告)
			accounts = append(accounts, JetbrainsAccount{
				LicenseID:      "", // 无许可证，无法刷新
				Authorization:  "", // 无授权，无法刷新
				JWT:            jwtToken,
				LastUpdated:    float64(time.Now().Unix()),
				HasQuota:       true,
				LastQuotaCheck: 0,
			})

			// 解析 JWT 过期时间，修改切片中最后一个元素
			if expiry, err := parseJWTExpiry(jwtToken); err != nil {
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
