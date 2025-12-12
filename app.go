package main

import (
	"context"
	"fmt"
)

// ==================== App - 应用容器 ====================

// App 应用容器
// 作为依赖注入中心，管理所有组件的生命周期
// 遵循 DIP（依赖倒置）和 SRP（单一职责）原则
type App struct {
	// 配置
	Config *Config

	// 日志
	Logger Logger

	// 核心组件（将在后续阶段添加）
	// AccountManager AccountManager
	// Cache          Cache
	// Server         *Server
	// ...

	// 生命周期管理
	ctx    context.Context
	cancel context.CancelFunc
}

// ==================== App 构造和初始化 ====================

// NewApp 创建新的应用实例
// 这是应用的入口点，负责初始化所有组件
func NewApp() (*App, error) {
	// 1. 加载配置
	config, err := LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// 2. 初始化日志系统
	InitializeLogger()
	logger := appLogger
	if logger == nil {
		logger = NewAppLogger()
	}

	logger.Info("=== JetBrains AI to OpenAI API Proxy ===")
	logger.Info("Application initializing...")

	// 3. 创建应用上下文
	ctx, cancel := context.WithCancel(context.Background())

	app := &App{
		Config: config,
		Logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}

	// 4. 验证配置
	if err := app.validateConfig(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	app.Logger.Info("Configuration loaded successfully")
	app.Logger.Info("  - Server port: %s", app.Config.Server.Port)
	app.Logger.Info("  - Gin mode: %s", app.Config.Server.GinMode)
	app.Logger.Info("  - Client API keys: %d", len(app.Config.Server.ClientAPIKeys))
	app.Logger.Info("  - JetBrains accounts: %d", len(app.Config.Accounts))
	app.Logger.Info("  - Models configured: %d", len(app.Config.Models.Models))

	return app, nil
}

// NewAppWithConfig 使用指定配置创建应用实例
// 用于测试或自定义配置场景
func NewAppWithConfig(config *Config, logger Logger) (*App, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if logger == nil {
		logger = NewAppLogger()
	}

	ctx, cancel := context.WithCancel(context.Background())

	app := &App{
		Config: config,
		Logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}

	if err := app.validateConfig(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return app, nil
}

// ==================== 配置验证 ====================

// validateConfig 验证应用配置
func (a *App) validateConfig() error {
	if a.Config == nil {
		return fmt.Errorf("config is nil")
	}

	// 验证服务器配置
	if a.Config.Server.Port == "" {
		return ErrInvalidConfig("server.port", "port cannot be empty")
	}

	// 验证账户配置
	if len(a.Config.Accounts) == 0 {
		a.Logger.Warn("No JetBrains accounts configured")
		// 不返回错误，允许空账户运行（用于测试）
	}

	// 验证模型配置
	if len(a.Config.Models.Models) == 0 {
		return ErrInvalidConfig("models", "no models configured")
	}

	return nil
}

// ==================== 生命周期管理 ====================

// Start 启动应用
// 在后续阶段将初始化所有组件并启动服务器
func (a *App) Start() error {
	a.Logger.Info("Starting application...")

	// TODO: 在后续阶段实现
	// 1. 初始化缓存
	// 2. 初始化账户管理器
	// 3. 初始化请求处理器
	// 4. 启动性能监控
	// 5. 启动服务器

	a.Logger.Info("Application started successfully")
	return nil
}

// Stop 停止应用
// 优雅关闭所有组件
func (a *App) Stop() error {
	a.Logger.Info("Stopping application...")

	// 取消上下文
	a.cancel()

	// TODO: 在后续阶段实现
	// 1. 停止服务器
	// 2. 保存统计数据
	// 3. 停止性能监控
	// 4. 关闭账户管理器
	// 5. 停止缓存清理
	// 6. 关闭日志

	// 关闭日志
	if err := CloseLogger(); err != nil {
		return fmt.Errorf("failed to close logger: %w", err)
	}

	a.Logger.Info("Application stopped successfully")
	return nil
}

// Context 获取应用上下文
func (a *App) Context() context.Context {
	return a.ctx
}

// ==================== 辅助方法 ====================

// GetConfig 获取应用配置
func (a *App) GetConfig() *Config {
	return a.Config
}

// GetLogger 获取日志实例
func (a *App) GetLogger() Logger {
	return a.Logger
}
