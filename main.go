package main

import (
	"github.com/bytedance/sonic"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

const (
	DefaultRequestTimeout = 30 * time.Second
	QuotaCacheTime        = time.Hour
	JWTRefreshTime        = 12 * time.Hour
)

// Global variables
var (
	validClientKeys        = make(map[string]bool)
	jetbrainsAccounts      []JetbrainsAccount
	accountPool            chan *JetbrainsAccount // 新增账户池通道
	modelsData             ModelsData
	modelsConfig           ModelsConfig
	httpClient             *http.Client
	requestStats           RequestStats
	statsMutex             sync.Mutex
	// 性能优化相关缓存
	toolsCache             = make(map[string][]Tool)
	toolsCacheMutex        sync.RWMutex
	accountQuotaCache      = make(map[string]*CachedQuotaInfo)
	quotaCacheMutex        sync.RWMutex
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	// Initialize storage and load statistics
	if err := initStorage(); err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	loadStats()

	// Initialize optimized HTTP client with connection pooling
	transport := &http.Transport{
		MaxIdleConns:        200,  // 增加连接池大小
		MaxIdleConnsPerHost: 50,   // 增加每个主机的连接数
		MaxConnsPerHost:     100,  // 限制每个主机的最大连接数
		IdleConnTimeout:     120 * time.Second, // 延长空闲连接超时
		TLSHandshakeTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:   false, // 启用 Keep-Alive
		ForceAttemptHTTP2:   true,  // 强制使用 HTTP/2
	}
	httpClient = &http.Client{
		Transport: transport,
		Timeout:   DefaultRequestTimeout,
	}

	// Load configuration
	modelsData = loadModels()
	data, err := os.ReadFile("models.json")
	if err == nil {
		sonic.Unmarshal(data, &modelsConfig)
	}
	loadClientAPIKeys()
	loadJetbrainsAccounts()
	// 初始化账户池
	initAccountPool()

	// Initialize request-triggered statistics saving
	initRequestTriggeredSaving()

	// Set up graceful shutdown
	setupGracefulShutdown()

	// Start pprof server
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	r := setupRoutes()

	log.Println("Starting JetBrains AI OpenAI Compatible API server...")
	port := getEnvWithDefault("PORT", "7860")

	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func setupGracefulShutdown() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Println("Shutdown signal received, saving statistics before exiting...")
		saveStats()
		os.Exit(0)
	}()
}

func initAccountPool() {
	if len(jetbrainsAccounts) == 0 {
		log.Println("Warning: No JetBrains accounts loaded, account pool is empty.")
		return
	}
	accountPool = make(chan *JetbrainsAccount, len(jetbrainsAccounts))
	for i := range jetbrainsAccounts {
		accountPool <- &jetbrainsAccounts[i]
	}
	log.Printf("Account pool initialized with %d accounts", len(jetbrainsAccounts))
}