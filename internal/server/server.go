package server

import (
	"context"
	"errors"
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
	modelsData      core.ModelList
	modelsConfig    core.ModelsConfig

	requestProcessor *process.RequestProcessor

	config config.ServerConfig

	rateLimiter *rateLimiter

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

	rateLimit := 120
	if envRate := os.Getenv("RATE_LIMIT"); envRate != "" {
		if parsed, parseErr := fmt.Sscanf(envRate, "%d", &rateLimit); parseErr != nil || parsed != 1 || rateLimit <= 0 {
			cfg.Logger.Warn("Invalid RATE_LIMIT value '%s', using default 120", envRate)
			rateLimit = 120
		}
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
		rateLimiter:      newRateLimiter(rateLimit),
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

	srv := &http.Server{
		Addr:              ":" + s.port,
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      5 * time.Minute, // SSE streams need longer timeout
	}

	go func() {
		<-s.shutdownCtx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			s.config.Logger.Error("Server shutdown error: %v", err)
		}
	}()

	s.config.Logger.Info("Server starting on port %s", s.port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

func (s *Server) setupGracefulShutdown() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		s.config.Logger.Info("Shutdown signal received, shutting down gracefully...")
		s.shutdownCancel()
	}()
}

func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(200, gin.H{"status": "healthy"})
}

func (s *Server) getStatsData(c *gin.Context) {
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
	periodStats := metrics.GetPeriodStats(stats.RequestHistory, 24, 24*7, 24*30)
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
		"stats24h":     periodStats[24],
		"stats7d":      periodStats[24*7],
		"stats30d":     periodStats[24*30],
		"tokensInfo":   tokensInfo,
		"expiryInfo":   expiryInfo,
	})
}

// Close closes the server
func (s *Server) Close() error {
	if s.shutdownCancel != nil {
		s.shutdownCancel()
	}

	var closeErr error

	if s.accountManager != nil {
		if err := s.accountManager.Close(); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close account manager: %w", err))
		}
	}

	if s.metricsService != nil {
		if err := s.metricsService.Close(); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close metrics service: %w", err))
		}
	}

	if s.cache != nil {
		if err := s.cache.Close(); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close cache service: %w", err))
		}
	}

	return closeErr
}
