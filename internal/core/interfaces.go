package core

import (
	"context"
	"time"
)

// Logger interface
type Logger interface {
	Debug(format string, args ...any)
	Info(format string, args ...any)
	Warn(format string, args ...any)
	Error(format string, args ...any)
	Fatal(format string, args ...any)
}

// Cache interface
type Cache interface {
	Get(key string) (any, bool)
	Set(key string, value any, duration time.Duration)
	Stop()
}

// StorageInterface storage interface
type StorageInterface interface {
	SaveStats(stats *RequestStats) error
	LoadStats() (*RequestStats, error)
	Close() error
}

// AccountManager interface
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

// MetricsCollector interface
type MetricsCollector interface {
	RecordHTTPRequest(duration time.Duration)
	RecordHTTPError()
	RecordCacheHit()
	RecordCacheMiss()
	RecordToolValidation(duration time.Duration)
	RecordAccountPoolWait(duration time.Duration)
	RecordAccountPoolError()
	GetQPS() float64
}

// NopLogger empty logger implementation
type NopLogger struct{}

func (*NopLogger) Debug(format string, args ...any) {}
func (*NopLogger) Info(format string, args ...any)  {}
func (*NopLogger) Warn(format string, args ...any)  {}
func (*NopLogger) Error(format string, args ...any) {}
func (*NopLogger) Fatal(format string, args ...any) {}

// NopMetrics empty metrics collector implementation
type NopMetrics struct{}

func (*NopMetrics) RecordHTTPRequest(duration time.Duration)     {}
func (*NopMetrics) RecordHTTPError()                             {}
func (*NopMetrics) RecordCacheHit()                              {}
func (*NopMetrics) RecordCacheMiss()                             {}
func (*NopMetrics) RecordToolValidation(duration time.Duration)  {}
func (*NopMetrics) RecordAccountPoolWait(duration time.Duration) {}
func (*NopMetrics) RecordAccountPoolError()                      {}
func (*NopMetrics) GetQPS() float64                              { return 0 }
