package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"jetbrainsai2api/internal/account"
	"jetbrainsai2api/internal/cache"
	"jetbrainsai2api/internal/config"
	"jetbrainsai2api/internal/core"
	logpkg "jetbrainsai2api/internal/log"
	"jetbrainsai2api/internal/metrics"
	"jetbrainsai2api/internal/process"
	"jetbrainsai2api/internal/util"

	"github.com/gin-gonic/gin"
)

// Server application server
type Server struct {
	port    string
	ginMode string

	accountManager core.AccountManager
	httpClient     *http.Client
	router         *gin.Engine

	cache          *cache.CacheService
	metricsService *metrics.MetricsService

	validClientKeys map[string]bool
	modelsData      core.ModelsData
	modelsConfig    core.ModelsConfig

	requestProcessor *process.RequestProcessor

	config config.ServerConfig

	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
}

// NewServer creates a new server instance
func NewServer(cfg config.ServerConfig) (*Server, error) {
	if cfg.Logger == nil {
		return nil, fmt.Errorf("logger is required in ServerConfig")
	}
	if cfg.Storage == nil {
		return nil, fmt.Errorf("storage is required in ServerConfig")
	}

	cfg.Logger.Info("Initializing server with %d accounts", len(cfg.JetbrainsAccounts))

	httpClient := createOptimizedHTTPClient(cfg.HTTPClientSettings)

	cacheService := cache.NewCacheService()

	metricsService := metrics.NewMetricsService(metrics.MetricsConfig{
		SaveInterval: core.MinSaveInterval,
		HistorySize:  core.HistoryBufferSize,
		Storage:      cfg.Storage,
		Logger:       cfg.Logger,
	})

	if err := metricsService.LoadStats(); err != nil {
		cfg.Logger.Warn("Failed to load historical stats: %v", err)
	}

	accountManager, err := account.NewPooledAccountManager(account.AccountManagerConfig{
		Accounts:   cfg.JetbrainsAccounts,
		HTTPClient: httpClient,
		Cache:      cacheService,
		Logger:     cfg.Logger,
		Metrics:    metricsService,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create account manager: %w", err)
	}

	modelsData, modelsConfig, err := config.GetModelsConfig(cfg.ModelsConfigPath, cfg.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to load models config: %w", err)
	}

	validClientKeys := make(map[string]bool)
	for _, key := range cfg.ClientAPIKeys {
		validClientKeys[key] = true
	}

	if len(validClientKeys) == 0 {
		cfg.Logger.Warn("No client API keys configured")
	} else {
		cfg.Logger.Info("Loaded %d client API keys", len(validClientKeys))
	}

	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())

	server := &Server{
		port:             cfg.Port,
		ginMode:          cfg.GinMode,
		accountManager:   accountManager,
		httpClient:       httpClient,
		cache:            cacheService,
		metricsService:   metricsService,
		validClientKeys:  validClientKeys,
		modelsData:       modelsData,
		modelsConfig:     modelsConfig,
		requestProcessor: process.NewRequestProcessor(modelsConfig, httpClient, cacheService, metricsService, cfg.Logger),
		config:           cfg,
		shutdownCtx:      shutdownCtx,
		shutdownCancel:   shutdownCancel,
	}

	server.setupRoutes()

	return server, nil
}

func createOptimizedHTTPClient(settings config.HTTPClientSettings) *http.Client {
	transport := &http.Transport{
		MaxIdleConns:          settings.MaxIdleConns,
		MaxIdleConnsPerHost:   settings.MaxIdleConnsPerHost,
		MaxConnsPerHost:       settings.MaxConnsPerHost,
		IdleConnTimeout:       settings.IdleConnTimeout,
		TLSHandshakeTimeout:   settings.TLSHandshakeTimeout,
		ExpectContinueTimeout: core.HTTPExpectContinueTimeout,
		DisableKeepAlives:     false,
		ForceAttemptHTTP2:     true,
		ResponseHeaderTimeout: core.HTTPResponseHeaderTimeout,
		DisableCompression:    false,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   settings.RequestTimeout,
	}
}

// Run runs the server
func (s *Server) Run() error {
	s.setupGracefulShutdown()

	s.config.Logger.Info("Starting JetBrains AI OpenAI Compatible API server on port %s", s.port)
	return s.router.Run(":" + s.port)
}

func (s *Server) setupGracefulShutdown() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		s.config.Logger.Info("Shutdown signal received, cleaning up resources...")

		s.shutdownCancel()

		if s.metricsService != nil {
			if err := s.metricsService.Close(); err != nil {
				s.config.Logger.Error("Failed to close metrics service: %v", err)
			}
		}

		if s.cache != nil {
			_ = s.cache.Close()
		}

		if err := s.accountManager.Close(); err != nil {
			s.config.Logger.Error("Failed to close account manager: %v", err)
		}

		if appLog, ok := s.config.Logger.(*logpkg.AppLogger); ok {
			_ = appLog.Close()
		}

		s.config.Logger.Info("Graceful shutdown completed")
		os.Exit(0)
	}()
}

func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(200, gin.H{
		"status":     "healthy",
		"service":    "jetbrainsai2api",
		"timestamp":  time.Now().Format(core.TimeFormatDateTime),
		"accounts":   s.accountManager.GetAccountCount(),
		"available":  s.accountManager.GetAvailableCount(),
		"valid_keys": len(s.validClientKeys),
	})
}

func (s *Server) getStatsData(c *gin.Context) {
	s.cache.ClearQuotaCache()

	accounts := s.accountManager.GetAllAccounts()
	var tokensInfo []gin.H

	for i := range accounts {
		quotaData, err := account.GetQuotaData(&accounts[i], s.httpClient, s.cache, s.config.Logger)
		tokenInfo := util.GetTokenInfoFromAccount(&accounts[i], quotaData, err)
		if err != nil {
			tokensInfo = append(tokensInfo, gin.H{
				"name":       util.GetTokenDisplayName(&accounts[i]),
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
				"expiryDate": tokenInfo.ExpiryDate.Format(core.TimeFormatDateTime),
				"status":     tokenInfo.Status,
			})
		}
	}

	stats := s.metricsService.GetRequestStats()
	stats24h := s.getPeriodStatsFromHistory(stats.RequestHistory, 24)
	stats7d := s.getPeriodStatsFromHistory(stats.RequestHistory, 24*7)
	stats30d := s.getPeriodStatsFromHistory(stats.RequestHistory, 24*30)
	currentQPS := s.metricsService.GetQPS()

	var expiryInfo []gin.H
	for i := range accounts {
		acct := &accounts[i]
		expiryTime := acct.ExpiryTime

		status := core.AccountStatusNormal
		warning := core.AccountStatusNormal
		if time.Now().Add(core.JWTExpiryCheckTime).After(expiryTime) {
			status = core.AccountStatusExpiring
			warning = core.AccountStatusExpiring
		}

		expiryInfo = append(expiryInfo, gin.H{
			"name":       util.GetTokenDisplayName(acct),
			"expiryTime": expiryTime.Format(core.TimeFormatDateTime),
			"status":     status,
			"warning":    warning,
		})
	}

	c.JSON(200, gin.H{
		"currentTime":  time.Now().Format(core.TimeFormatDateTime),
		"currentQPS":   fmt.Sprintf("%.3f", currentQPS),
		"totalRecords": len(stats.RequestHistory),
		"stats24h":     stats24h,
		"stats7d":      stats7d,
		"stats30d":     stats30d,
		"tokensInfo":   tokensInfo,
		"expiryInfo":   expiryInfo,
	})
}

func (s *Server) getPeriodStatsFromHistory(history []core.RequestRecord, hours int) core.PeriodStats {
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

	stats := core.PeriodStats{
		Requests: periodRequests,
	}

	if periodRequests > 0 {
		stats.SuccessRate = float64(periodSuccessful) / float64(periodRequests) * 100
		stats.AvgResponseTime = periodResponseTime / periodRequests
	}

	stats.QPS = float64(periodRequests) / (float64(hours) * 3600.0)

	return stats
}

// Close closes the server
func (s *Server) Close() error {
	s.shutdownCancel()
	return s.accountManager.Close()
}
