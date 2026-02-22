package metrics

import (
	"sync"
	"testing"
	"time"

	"jetbrainsai2api/internal/core"
)

type countingStorage struct {
	mu        sync.Mutex
	saveCount int
}

func (s *countingStorage) SaveStats(_ *core.RequestStats) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveCount++
	return nil
}

func (s *countingStorage) LoadStats() (*core.RequestStats, error) {
	return &core.RequestStats{}, nil
}

func (s *countingStorage) Close() error { return nil }

func (s *countingStorage) getSaveCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveCount
}

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

	// HistorySize=0 is passed directly; the service uses it as-is.
	// This test just verifies no crash on construction.
	_ = ms.maxHistorySize
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

func TestMetricsService_Close_Idempotent(t *testing.T) {
	st := &countingStorage{}
	ms := NewMetricsService(MetricsConfig{
		SaveInterval: time.Second,
		HistorySize:  10,
		Storage:      st,
		Logger:       &core.NopLogger{},
	})

	ms.RecordRequest(true, 10, "gpt-4", "acc1")

	if err := ms.Close(); err != nil {
		t.Fatalf("第一次关闭不应失败: %v", err)
	}
	firstCloseSaves := st.getSaveCount()
	if firstCloseSaves == 0 {
		t.Fatal("第一次关闭后应至少有一次持久化")
	}

	if err := ms.Close(); err != nil {
		t.Fatalf("第二次关闭不应失败: %v", err)
	}

	if st.getSaveCount() != firstCloseSaves {
		t.Fatalf("第二次 Close 不应新增持久化，第一次=%d，第二次后=%d", firstCloseSaves, st.getSaveCount())
	}
}
