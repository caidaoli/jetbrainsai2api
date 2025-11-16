package main

import (
	"time"
)

// Cache 定义缓存接口
type Cache interface {
	Get(key string) (any, bool)
	Set(key string, value any, duration time.Duration)
	Stop()
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

	// 系统指标
	UpdateSystemMetrics()
	ResetWindow()

	// 查询指标
	GetQPS() float64
	GetMetricsString() string
}

// AccountManager 接口已在 account_manager.go 中定义
