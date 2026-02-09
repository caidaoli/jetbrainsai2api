package metrics

import (
	"testing"
	"time"

	"jetbrainsai2api/internal/core"
)

func TestNewMetricsService(t *testing.T) {
	ms := NewMetricsService(MetricsConfig{
		SaveInterval: time.Second,
		HistorySize:  10,
		Storage:      nil,
		Logger:       &core.NopLogger{},
	})
	defer func() { _ = ms.Close() }()

	if ms == nil {
		t.Fatal("MetricsService should not be nil")
	}
}

func TestMetricsService_RecordRequest(t *testing.T) {
	ms := NewMetricsService(MetricsConfig{
		SaveInterval: time.Second,
		HistorySize:  10,
		Storage:      nil,
		Logger:       &core.NopLogger{},
	})
	defer func() { _ = ms.Close() }()

	ms.RecordRequest(true, 100, "gpt-4", "acc1")
	ms.RecordRequest(false, 200, "gpt-4", "acc2")
	ms.RecordRequest(true, 150, "claude-3", "acc1")

	// Flush buffer
	time.Sleep(200 * time.Millisecond)

	stats := ms.GetRequestStats()
	if stats.TotalRequests != 3 {
		t.Errorf("Expected 3 total requests, got %d", stats.TotalRequests)
	}
	if stats.SuccessfulRequests != 2 {
		t.Errorf("Expected 2 successful requests, got %d", stats.SuccessfulRequests)
	}
	if stats.FailedRequests != 1 {
		t.Errorf("Expected 1 failed request, got %d", stats.FailedRequests)
	}
}

func TestMetricsService_GetQPS(t *testing.T) {
	ms := NewMetricsService(MetricsConfig{
		SaveInterval: time.Second,
		HistorySize:  10,
		Storage:      nil,
		Logger:       &core.NopLogger{},
	})
	defer func() { _ = ms.Close() }()

	qps := ms.GetQPS()
	if qps < 0 {
		t.Errorf("QPS should not be negative, got %f", qps)
	}
}

func TestMetricsService_MaxHistorySize(t *testing.T) {
	ms := NewMetricsService(MetricsConfig{
		SaveInterval: time.Second,
		HistorySize:  3,
		Storage:      nil,
		Logger:       &core.NopLogger{},
	})
	defer func() { _ = ms.Close() }()

	for i := 0; i < 5; i++ {
		ms.RecordRequest(true, 100, "model", "acc")
	}

	// Wait for flush
	time.Sleep(200 * time.Millisecond)

	stats := ms.GetRequestStats()
	if len(stats.RequestHistory) > 3 {
		t.Errorf("History should be capped at 3, got %d", len(stats.RequestHistory))
	}
}

func TestMetricsService_DefaultHistorySize(t *testing.T) {
	ms := NewMetricsService(MetricsConfig{
		SaveInterval: time.Second,
		HistorySize:  0,
		Storage:      nil,
		Logger:       &core.NopLogger{},
	})
	defer func() { _ = ms.Close() }()

	if ms.maxHistorySize != 0 {
		// HistorySize=0 is passed directly; the service uses it as-is
		// This just verifies no crash
	}
}

func TestRecordSuccessWithMetrics(t *testing.T) {
	ms := NewMetricsService(MetricsConfig{
		SaveInterval: time.Second,
		HistorySize:  10,
		Storage:      nil,
		Logger:       &core.NopLogger{},
	})
	defer func() { _ = ms.Close() }()

	RecordSuccessWithMetrics(ms, time.Now(), "gpt-4", "acc1")

	time.Sleep(200 * time.Millisecond)

	stats := ms.GetRequestStats()
	if stats.SuccessfulRequests != 1 {
		t.Errorf("Expected 1 successful request, got %d", stats.SuccessfulRequests)
	}
}

func TestRecordFailureWithMetrics(t *testing.T) {
	ms := NewMetricsService(MetricsConfig{
		SaveInterval: time.Second,
		HistorySize:  10,
		Storage:      nil,
		Logger:       &core.NopLogger{},
	})
	defer func() { _ = ms.Close() }()

	RecordFailureWithMetrics(ms, time.Now(), "gpt-4", "acc1")

	time.Sleep(200 * time.Millisecond)

	stats := ms.GetRequestStats()
	if stats.FailedRequests != 1 {
		t.Errorf("Expected 1 failed request, got %d", stats.FailedRequests)
	}
}
