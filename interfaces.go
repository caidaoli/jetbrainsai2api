package main

import (
	"context"
	"time"
)

// ==================== 接口定义 ====================

// Logger 日志接口
type Logger interface {
	Debug(format string, args ...any)
	Info(format string, args ...any)
	Warn(format string, args ...any)
	Error(format string, args ...any)
	Fatal(format string, args ...any)
}

// Cache 定义缓存接口
type Cache interface {
	Get(key string) (any, bool)
	Set(key string, value any, duration time.Duration)
	Stop()
}

// StorageInterface 存储接口
type StorageInterface interface {
	SaveStats(stats *RequestStats) error
	LoadStats() (*RequestStats, error)
	Close() error
}

// AccountManager 账户管理器接口
type AccountManager interface {
	AcquireAccount(ctx context.Context) (*JetbrainsAccount, error)
	ReleaseAccount(account *JetbrainsAccount)
	RefreshJWT(account *JetbrainsAccount) error
	CheckQuota(account *JetbrainsAccount) error
	GetAccountCount() int
	GetAvailableCount() int
	GetAllAccounts() []JetbrainsAccount
	Close() error
}

// MetricsCollector 定义性能指标收集接口
type MetricsCollector interface {
	// HTTP 请求指标
	RecordHTTPRequest(duration time.Duration)
	RecordHTTPError()

	// 缓存指标
	RecordCacheHit()
	RecordCacheMiss()

	// 工具验证指标
	RecordToolValidation(duration time.Duration)

	// 账户池指标
	RecordAccountPoolWait(duration time.Duration)
	RecordAccountPoolError()

	// 查询指标
	GetQPS() float64
}

// ==================== Nop 实现（用于测试和默认值） ====================

// NopLogger 空日志实现
type NopLogger struct{}

func (*NopLogger) Debug(format string, args ...any) {}
func (*NopLogger) Info(format string, args ...any)  {}
func (*NopLogger) Warn(format string, args ...any)  {}
func (*NopLogger) Error(format string, args ...any) {}
func (*NopLogger) Fatal(format string, args ...any) {}

// NopMetrics 空指标收集器实现
type NopMetrics struct{}

func (*NopMetrics) RecordHTTPRequest(duration time.Duration)     {}
func (*NopMetrics) RecordHTTPError()                             {}
func (*NopMetrics) RecordCacheHit()                              {}
func (*NopMetrics) RecordCacheMiss()                             {}
func (*NopMetrics) RecordToolValidation(duration time.Duration)  {}
func (*NopMetrics) RecordAccountPoolWait(duration time.Duration) {}
func (*NopMetrics) RecordAccountPoolError()                      {}
func (*NopMetrics) GetQPS() float64                              { return 0 }

// ==================== 编译时接口实现验证 ====================
// 确保具体类型正确实现了接口

var (
	_ Logger           = (*AppLogger)(nil)
	_ Logger           = (*NopLogger)(nil)
	_ Cache            = (*LRUCache)(nil)
	_ Cache            = (*CacheService)(nil)
	_ StorageInterface = (*FileStorage)(nil)
	_ StorageInterface = (*RedisStorage)(nil)
	_ AccountManager   = (*PooledAccountManager)(nil)
	_ MetricsCollector = (*MetricsService)(nil)
	_ MetricsCollector = (*NopMetrics)(nil)
)
