package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gin-gonic/gin"
)

// Server 应用服务器
// 封装所有全局状态，遵循 OCP（开闭原则）和 DIP（依赖倒置原则）
type Server struct {
	// 配置
	port    string
	ginMode string

	// 核心组件（依赖注入）
	accountManager AccountManager
	httpClient     *http.Client
	router         *gin.Engine

	// 缓存（新增：消除全局变量）
	cache Cache

	// 认证和模型
	validClientKeys map[string]bool
	modelsData      ModelsData
	modelsConfig    ModelsConfig

	// 请求处理器
	requestProcessor *RequestProcessor

	// 优雅关闭
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port               string
	GinMode            string
	ClientAPIKeys      []string
	JetbrainsAccounts  []JetbrainsAccount
	ModelsConfigPath   string
	HTTPClientSettings HTTPClientSettings
}

// HTTPClientSettings HTTP 客户端配置
type HTTPClientSettings struct {
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	MaxConnsPerHost     int
	IdleConnTimeout     time.Duration
	TLSHandshakeTimeout time.Duration
	RequestTimeout      time.Duration
}

// DefaultHTTPClientSettings 默认 HTTP 客户端配置
func DefaultHTTPClientSettings() HTTPClientSettings {
	return HTTPClientSettings{
		MaxIdleConns:        500,
		MaxIdleConnsPerHost: 100,
		MaxConnsPerHost:     200,
		IdleConnTimeout:     600 * time.Second,
		TLSHandshakeTimeout: 30 * time.Second,
		RequestTimeout:      5 * time.Minute,
	}
}

// NewServer 创建新的服务器实例
// OCP: 通过配置开放扩展，对修改封闭
func NewServer(config ServerConfig) (*Server, error) {
	// 初始化日志系统
	InitializeLogger()
	Info("Initializing server with %d accounts", len(config.JetbrainsAccounts))

	// 创建 HTTP 客户端（必须先创建，因为 AccountManager 需要它）
	httpClient := createOptimizedHTTPClient(config.HTTPClientSettings)

	// 创建缓存实例（消除全局变量）
	cache := NewCache()

	// 创建账户管理器
	accountManager, err := NewPooledAccountManager(config.JetbrainsAccounts, httpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create account manager: %w", err)
	}

	// 加载模型配置
	modelsData, modelsConfig, err := loadModelsConfig(config.ModelsConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load models config: %w", err)
	}

	// 准备客户端 API keys
	validClientKeys := make(map[string]bool)
	for _, key := range config.ClientAPIKeys {
		validClientKeys[key] = true
	}

	if len(validClientKeys) == 0 {
		Warn("No client API keys configured")
	} else {
		Info("Loaded %d client API keys", len(validClientKeys))
	}

	// 创建 context 用于优雅关闭
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())

	server := &Server{
		port:             config.Port,
		ginMode:          config.GinMode,
		accountManager:   accountManager,
		httpClient:       httpClient,
		cache:            cache,
		validClientKeys:  validClientKeys,
		modelsData:       modelsData,
		modelsConfig:     modelsConfig,
		requestProcessor: NewRequestProcessor(modelsConfig, httpClient, cache),
		shutdownCtx:      shutdownCtx,
		shutdownCancel:   shutdownCancel,
	}

	// 设置路由
	server.setupRoutes()

	return server, nil
}

// createOptimizedHTTPClient 创建优化的 HTTP 客户端
func createOptimizedHTTPClient(settings HTTPClientSettings) *http.Client {
	transport := &http.Transport{
		MaxIdleConns:          settings.MaxIdleConns,
		MaxIdleConnsPerHost:   settings.MaxIdleConnsPerHost,
		MaxConnsPerHost:       settings.MaxConnsPerHost,
		IdleConnTimeout:       settings.IdleConnTimeout,
		TLSHandshakeTimeout:   settings.TLSHandshakeTimeout,
		ExpectContinueTimeout: 5 * time.Second,
		DisableKeepAlives:     false,
		ForceAttemptHTTP2:     true,
		ResponseHeaderTimeout: 30 * time.Second,
		DisableCompression:    false,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   settings.RequestTimeout,
	}
}

// loadModelsConfig 加载模型配置
func loadModelsConfig(path string) (ModelsData, ModelsConfig, error) {
	modelsData, err := loadModels()
	if err != nil {
		return modelsData, ModelsConfig{}, fmt.Errorf("failed to load models: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return modelsData, ModelsConfig{}, err
	}

	var modelsConfig ModelsConfig
	if err := sonic.Unmarshal(data, &modelsConfig); err != nil {
		return modelsData, ModelsConfig{}, err
	}

	return modelsData, modelsConfig, nil
}

// setupRoutes 设置路由
// SRP: 单一职责 - 只负责路由配置
func (s *Server) setupRoutes() {
	gin.SetMode(s.ginMode)
	s.router = gin.New()

	// 添加中间件
	s.router.Use(gin.Logger())
	s.router.Use(gin.Recovery())
	s.router.Use(s.corsMiddleware())

	// 公共路由（无需认证）
	s.router.GET("/", showStatsPage)
	s.router.GET("/log", streamLog)
	s.router.GET("/api/stats", s.getStatsData)
	s.router.GET("/health", s.healthCheck)

	// API 路由（需要认证）
	api := s.router.Group("/v1")
	api.Use(s.authenticateClient)
	{
		api.GET("/models", s.listModels)
		api.POST("/chat/completions", s.chatCompletions)
		api.POST("/messages", s.anthropicMessages)
	}
}

// Run 运行服务器
func (s *Server) Run() error {
	// 设置优雅关闭
	s.setupGracefulShutdown()

	Info("Starting JetBrains AI OpenAI Compatible API server on port %s", s.port)
	return s.router.Run(":" + s.port)
}

// setupGracefulShutdown 设置优雅关闭
func (s *Server) setupGracefulShutdown() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		Info("Shutdown signal received, cleaning up resources...")

		// 取消 context
		s.shutdownCancel()

		// 保存统计数据
		saveStats()

		// 停止缓存后台 goroutine
		messageConversionCache.Stop()
		toolsValidationCache.Stop()
		validatedToolsCache.Stop()
		paramTransformCache.Stop()

		// 停止性能监控
		StopMetricsMonitor()

		// 关闭账户管理器
		if err := s.accountManager.Close(); err != nil {
			Error("Failed to close account manager: %v", err)
		}

		// 关闭日志
		CloseLogger()

		Info("Graceful shutdown completed")
		os.Exit(0)
	}()
}

// healthCheck 健康检查端点
func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(200, gin.H{
		"status":     "healthy",
		"service":    "jetbrainsai2api",
		"timestamp":  time.Now().Format("2006-01-02 15:04:05"),
		"accounts":   s.accountManager.GetAccountCount(),
		"available":  s.accountManager.GetAvailableCount(),
		"valid_keys": len(s.validClientKeys),
	})
}

// getStatsData 获取统计数据
func (s *Server) getStatsData(c *gin.Context) {
	// 清除配额缓存
	quotaCacheMutex.Lock()
	for key := range accountQuotaCache {
		delete(accountQuotaCache, key)
	}
	quotaCacheMutex.Unlock()

	// 获取所有账户信息
	accounts := s.accountManager.GetAllAccounts()
	var tokensInfo []gin.H

	for i := range accounts {
		tokenInfo, err := getTokenInfoFromAccount(&accounts[i], s.httpClient)
		if err != nil {
			tokensInfo = append(tokensInfo, gin.H{
				"name":       getTokenDisplayName(&accounts[i]),
				"license":    "",
				"used":       0.0,
				"total":      0.0,
				"usageRate":  0.0,
				"expiryDate": "",
				"status":     "错误",
			})
		} else {
			tokensInfo = append(tokensInfo, gin.H{
				"name":       tokenInfo.Name,
				"license":    tokenInfo.License,
				"used":       tokenInfo.Used,
				"total":      tokenInfo.Total,
				"usageRate":  tokenInfo.UsageRate,
				"expiryDate": tokenInfo.ExpiryDate.Format("2006-01-02 15:04:05"),
				"status":     tokenInfo.Status,
			})
		}
	}

	// 获取统计数据
	stats24h := getPeriodStats(24)
	stats7d := getPeriodStats(24 * 7)
	stats30d := getPeriodStats(24 * 30)
	currentQPS := getCurrentQPS()

	// Token 过期监控
	var expiryInfo []gin.H
	for i := range accounts {
		account := &accounts[i]
		expiryTime := account.ExpiryTime

		status := "正常"
		warning := "正常"
		if time.Now().Add(1 * time.Hour).After(expiryTime) {
			status = "即将过期"
			warning = "即将过期"
		}

		expiryInfo = append(expiryInfo, gin.H{
			"name":       getTokenDisplayName(account),
			"expiryTime": expiryTime.Format("2006-01-02 15:04:05"),
			"status":     status,
			"warning":    warning,
		})
	}

	c.JSON(200, gin.H{
		"currentTime":  time.Now().Format("2006-01-02 15:04:05"),
		"currentQPS":   fmt.Sprintf("%.3f", currentQPS),
		"totalRecords": len(atomicStats.GetHistory()),
		"stats24h":     stats24h,
		"stats7d":      stats7d,
		"stats30d":     stats30d,
		"tokensInfo":   tokensInfo,
		"expiryInfo":   expiryInfo,
	})
}

// corsMiddleware CORS中间件
func (s *Server) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, x-api-key")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// Close 关闭服务器
func (s *Server) Close() error {
	s.shutdownCancel()
	return s.accountManager.Close()
}
