package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	// 缓存（使用 CacheService 消除全局变量）
	cache *CacheService

	// 指标服务（消除全局变量）
	metricsService *MetricsService

	// 认证和模型
	validClientKeys map[string]bool
	modelsData      ModelsData
	modelsConfig    ModelsConfig

	// 请求处理器
	requestProcessor *RequestProcessor

	// 配置
	config ServerConfig

	// 优雅关闭
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
}

// NewServer 创建新的服务器实例
// OCP: 通过配置开放扩展，对修改封闭
func NewServer(config ServerConfig) (*Server, error) {
	// 验证配置
	if config.Logger == nil {
		return nil, fmt.Errorf("logger is required in ServerConfig")
	}
	if config.Storage == nil {
		return nil, fmt.Errorf("storage is required in ServerConfig")
	}

	config.Logger.Info("Initializing server with %d accounts", len(config.JetbrainsAccounts))

	// 创建 HTTP 客户端（必须先创建，因为 AccountManager 需要它）
	httpClient := createOptimizedHTTPClient(config.HTTPClientSettings)

	// 创建缓存服务（消除全局变量）
	cacheService := NewCacheService()

	// 创建指标服务（从配置获取 Storage 和 Logger）
	metricsService := NewMetricsService(MetricsConfig{
		SaveInterval: MinSaveInterval,
		HistorySize:  HistoryBufferSize,
		Storage:      config.Storage, // 从配置获取
		Logger:       config.Logger,  // 从配置获取
	})

	// 加载历史统计数据
	if err := metricsService.LoadStats(); err != nil {
		config.Logger.Warn("Failed to load historical stats: %v", err)
	}

	// 创建账户管理器（使用新的配置式构造函数）
	accountManager, err := NewPooledAccountManager(AccountManagerConfig{
		Accounts:   config.JetbrainsAccounts,
		HTTPClient: httpClient,
		Cache:      cacheService,
		Logger:     config.Logger, // 从配置获取
		Metrics:    metricsService,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create account manager: %w", err)
	}

	// 加载模型配置（使用新的config.go中的函数）
	modelsData, modelsConfig, err := GetModelsConfig(config.ModelsConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load models config: %w", err)
	}

	// 准备客户端 API keys
	validClientKeys := make(map[string]bool)
	for _, key := range config.ClientAPIKeys {
		validClientKeys[key] = true
	}

	if len(validClientKeys) == 0 {
		config.Logger.Warn("No client API keys configured")
	} else {
		config.Logger.Info("Loaded %d client API keys", len(validClientKeys))
	}

	// 创建 context 用于优雅关闭
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())

	server := &Server{
		port:             config.Port,
		ginMode:          config.GinMode,
		accountManager:   accountManager,
		httpClient:       httpClient,
		cache:            cacheService,
		metricsService:   metricsService,
		validClientKeys:  validClientKeys,
		modelsData:       modelsData,
		modelsConfig:     modelsConfig,
		requestProcessor: NewRequestProcessor(modelsConfig, httpClient, cacheService, metricsService, config.Logger),
		config:           config,
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
		ExpectContinueTimeout: HTTPExpectContinueTimeout,
		DisableKeepAlives:     false,
		ForceAttemptHTTP2:     true,
		ResponseHeaderTimeout: HTTPResponseHeaderTimeout,
		DisableCompression:    false,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   settings.RequestTimeout,
	}
}

// GetModelsConfig 加载模型配置（使用新的config.go中的函数）
func GetModelsConfig(path string) (ModelsData, ModelsConfig, error) {
	modelsData, err := loadModels(path)
	if err != nil {
		return modelsData, ModelsConfig{}, fmt.Errorf("failed to load models: %w", err)
	}

	modelsConfig, err := loadModelsConfig(path)
	if err != nil {
		return modelsData, ModelsConfig{}, fmt.Errorf("failed to load model mappings: %w", err)
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
	s.router.GET("/api/stats", s.getStatsData)
	s.router.GET("/log", streamLog)
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

	s.config.Logger.Info("Starting JetBrains AI OpenAI Compatible API server on port %s", s.port)
	return s.router.Run(":" + s.port)
}

// setupGracefulShutdown 设置优雅关闭
func (s *Server) setupGracefulShutdown() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		s.config.Logger.Info("Shutdown signal received, cleaning up resources...")

		// 取消 context
		s.shutdownCancel()

		// 关闭指标服务（内部会保存统计数据）
		if s.metricsService != nil {
			if err := s.metricsService.Close(); err != nil {
				s.config.Logger.Error("Failed to close metrics service: %v", err)
			}
		}

		// 停止缓存服务
		if s.cache != nil {
			_ = s.cache.Close()
		}

		// 关闭账户管理器
		if err := s.accountManager.Close(); err != nil {
			s.config.Logger.Error("Failed to close account manager: %v", err)
		}

		// 关闭日志（如果是 AppLogger 实例）
		if appLog, ok := s.config.Logger.(*AppLogger); ok {
			_ = appLog.Close()
		}

		s.config.Logger.Info("Graceful shutdown completed")
		os.Exit(0)
	}()
}

// healthCheck 健康检查端点
func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(200, gin.H{
		"status":     "healthy",
		"service":    "jetbrainsai2api",
		"timestamp":  time.Now().Format(TimeFormatDateTime),
		"accounts":   s.accountManager.GetAccountCount(),
		"available":  s.accountManager.GetAvailableCount(),
		"valid_keys": len(s.validClientKeys),
	})
}

// getStatsData 获取统计数据
func (s *Server) getStatsData(c *gin.Context) {
	// 清除配额缓存（使用 CacheService）
	s.cache.ClearQuotaCache()

	// 获取所有账户信息
	accounts := s.accountManager.GetAllAccounts()
	var tokensInfo []gin.H

	for i := range accounts {
		tokenInfo, err := getTokenInfoFromAccount(&accounts[i], s.httpClient, s.cache, s.config.Logger)
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
				"expiryDate": tokenInfo.ExpiryDate.Format(TimeFormatDateTime),
				"status":     tokenInfo.Status,
			})
		}
	}

	// 从 MetricsService 获取统计数据
	stats := s.metricsService.GetRequestStats()
	stats24h := s.getPeriodStatsFromHistory(stats.RequestHistory, 24)
	stats7d := s.getPeriodStatsFromHistory(stats.RequestHistory, 24*7)
	stats30d := s.getPeriodStatsFromHistory(stats.RequestHistory, 24*30)
	currentQPS := s.metricsService.GetQPS()

	// Token 过期监控
	var expiryInfo []gin.H
	for i := range accounts {
		account := &accounts[i]
		expiryTime := account.ExpiryTime

		status := AccountStatusNormal
		warning := AccountStatusNormal
		if time.Now().Add(JWTExpiryCheckTime).After(expiryTime) {
			status = AccountStatusExpiring
			warning = AccountStatusExpiring
		}

		expiryInfo = append(expiryInfo, gin.H{
			"name":       getTokenDisplayName(account),
			"expiryTime": expiryTime.Format(TimeFormatDateTime),
			"status":     status,
			"warning":    warning,
		})
	}

	c.JSON(200, gin.H{
		"currentTime":  time.Now().Format(TimeFormatDateTime),
		"currentQPS":   fmt.Sprintf("%.3f", currentQPS),
		"totalRecords": len(stats.RequestHistory),
		"stats24h":     stats24h,
		"stats7d":      stats7d,
		"stats30d":     stats30d,
		"tokensInfo":   tokensInfo,
		"expiryInfo":   expiryInfo,
	})
}

// getPeriodStatsFromHistory 从历史记录计算周期统计
func (s *Server) getPeriodStatsFromHistory(history []RequestRecord, hours int) PeriodStats {
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	var periodRequests int64
	var periodSuccessful int64
	var periodResponseTime int64

	for _, record := range history {
		if record.Timestamp.After(cutoff) {
			periodRequests++
			periodResponseTime += record.ResponseTime
			if record.Success {
				periodSuccessful++
			}
		}
	}

	stats := PeriodStats{
		Requests: periodRequests,
	}

	if periodRequests > 0 {
		stats.SuccessRate = float64(periodSuccessful) / float64(periodRequests) * 100
		stats.AvgResponseTime = periodResponseTime / periodRequests
	}

	stats.QPS = float64(periodRequests) / (float64(hours) * 3600.0)

	return stats
}

// Close 关闭服务器
func (s *Server) Close() error {
	s.shutdownCancel()
	return s.accountManager.Close()
}
