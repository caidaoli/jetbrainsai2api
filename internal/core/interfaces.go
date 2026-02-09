package core

import (
	"context"
	"time"
)

// Logger defines the logging interface used throughout the application.
type Logger interface {
	Debug(format string, args ...any)
	Info(format string, args ...any)
	Warn(format string, args ...any)
	Error(format string, args ...any)
	Fatal(format string, args ...any)
}

// Cache defines the caching interface with TTL support.
type Cache interface {
	Get(key string) (any, bool)
	Set(key string, value any, duration time.Duration)
	Stop()
}

// StorageInterface defines the persistence interface for request statistics.
type StorageInterface interface {
	SaveStats(stats *RequestStats) error
	LoadStats() (*RequestStats, error)
	Close() error
}

// AccountManager defines the interface for JetBrains account pool management.
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

// MetricsCollector defines the interface for collecting runtime metrics.
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

// NopLogger is a no-op Logger implementation for testing.
type NopLogger struct{}

// Debug is a no-op.
func (*NopLogger) Debug(format string, args ...any) {}

// Info is a no-op.
func (*NopLogger) Info(format string, args ...any) {}

// Warn is a no-op.
func (*NopLogger) Warn(format string, args ...any) {}

// Error is a no-op.
func (*NopLogger) Error(format string, args ...any) {}

// Fatal is a no-op.
func (*NopLogger) Fatal(format string, args ...any) {}

// NopMetrics is a no-op MetricsCollector implementation for testing.
type NopMetrics struct{}

// RecordHTTPRequest is a no-op.
func (*NopMetrics) RecordHTTPRequest(duration time.Duration) {}

// RecordHTTPError is a no-op.
func (*NopMetrics) RecordHTTPError() {}

// RecordCacheHit is a no-op.
func (*NopMetrics) RecordCacheHit() {}

// RecordCacheMiss is a no-op.
func (*NopMetrics) RecordCacheMiss() {}

// RecordToolValidation is a no-op.
func (*NopMetrics) RecordToolValidation(duration time.Duration) {}

// RecordAccountPoolWait is a no-op.
func (*NopMetrics) RecordAccountPoolWait(duration time.Duration) {}

// RecordAccountPoolError is a no-op.
func (*NopMetrics) RecordAccountPoolError() {}

// GetQPS is a no-op that always returns 0.
func (*NopMetrics) GetQPS() float64 { return 0 }
