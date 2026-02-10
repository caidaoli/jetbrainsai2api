package metrics

import (
	"embed"
	"fmt"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"jetbrainsai2api/internal/core"

	"github.com/gin-gonic/gin"
)

// StatsPageHTML holds the embedded monitoring dashboard HTML.
//
//go:embed static/index.html
var StatsPageHTML embed.FS

// AtomicRequestStats thread-safe request statistics
type AtomicRequestStats struct {
	TotalRequests      atomic.Int64
	SuccessfulRequests atomic.Int64
	FailedRequests     atomic.Int64
	TotalResponseTime  atomic.Int64
}

// MetricsConfig configuration for MetricsService
type MetricsConfig struct {
	SaveInterval time.Duration
	HistorySize  int
	Storage      core.StorageInterface
	Logger       core.Logger
}

// MetricsService collects and manages metrics
type MetricsService struct {
	atomicStats      AtomicRequestStats
	requestHistory   []core.RequestRecord
	historyMu        sync.RWMutex
	lastRequestTime  time.Time
	maxHistorySize   int
	storage          core.StorageInterface
	logger           core.Logger
	lastSaveTime     time.Time
	minSaveInterval  time.Duration
	done             chan struct{}
	historyBuffer    []core.RequestRecord
	bufferMu         sync.Mutex
	bufferFlushTimer *time.Ticker
	recentRequests   []time.Time
	recentMu         sync.Mutex
}

// NewMetricsService creates a new MetricsService
func NewMetricsService(config MetricsConfig) *MetricsService {
	ms := &MetricsService{
		maxHistorySize:  config.HistorySize,
		storage:         config.Storage,
		logger:          config.Logger,
		minSaveInterval: config.SaveInterval,
		done:            make(chan struct{}),
		historyBuffer:   make([]core.RequestRecord, 0, core.HistoryBatchSize),
	}

	ms.bufferFlushTimer = time.NewTicker(core.HistoryFlushInterval)
	go ms.flushLoop()

	return ms
}

func (ms *MetricsService) flushLoop() {
	for {
		select {
		case <-ms.bufferFlushTimer.C:
			ms.flushBuffer()
		case <-ms.done:
			return
		}
	}
}

func (ms *MetricsService) flushBuffer() {
	ms.bufferMu.Lock()
	if len(ms.historyBuffer) == 0 {
		ms.bufferMu.Unlock()
		return
	}
	batch := ms.historyBuffer
	ms.historyBuffer = make([]core.RequestRecord, 0, core.HistoryBatchSize)
	ms.bufferMu.Unlock()

	ms.historyMu.Lock()
	ms.requestHistory = append(ms.requestHistory, batch...)
	if len(ms.requestHistory) > ms.maxHistorySize {
		ms.requestHistory = ms.requestHistory[len(ms.requestHistory)-ms.maxHistorySize:]
	}
	ms.historyMu.Unlock()
}

// RecordRequest records a request result
func (ms *MetricsService) RecordRequest(success bool, responseTime int64, model string, account string) {
	now := time.Now()
	ms.historyMu.Lock()
	ms.lastRequestTime = now
	ms.historyMu.Unlock()
	ms.atomicStats.TotalRequests.Add(1)
	ms.atomicStats.TotalResponseTime.Add(responseTime)

	if success {
		ms.atomicStats.SuccessfulRequests.Add(1)
	} else {
		ms.atomicStats.FailedRequests.Add(1)
	}

	ms.recentMu.Lock()
	ms.recentRequests = append(ms.recentRequests, now)
	cutoff := now.Add(-1 * time.Minute)
	startIdx := 0
	for startIdx < len(ms.recentRequests) && ms.recentRequests[startIdx].Before(cutoff) {
		startIdx++
	}
	if startIdx > 0 {
		newRecent := make([]time.Time, len(ms.recentRequests)-startIdx)
		copy(newRecent, ms.recentRequests[startIdx:])
		ms.recentRequests = newRecent
	}
	ms.recentMu.Unlock()

	record := core.RequestRecord{
		Timestamp:    now,
		Success:      success,
		ResponseTime: responseTime,
		Model:        model,
		Account:      account,
	}

	ms.bufferMu.Lock()
	ms.historyBuffer = append(ms.historyBuffer, record)
	shouldFlush := len(ms.historyBuffer) >= core.HistoryBatchSize
	ms.bufferMu.Unlock()

	if shouldFlush {
		ms.flushBuffer()
	}

	ms.SaveStatsDebounced()
}

// RecordHTTPRequest records HTTP request duration
func (ms *MetricsService) RecordHTTPRequest(duration time.Duration) {
	ms.atomicStats.TotalResponseTime.Add(duration.Milliseconds())
}

// RecordHTTPError records HTTP error
func (ms *MetricsService) RecordHTTPError() {
	ms.atomicStats.FailedRequests.Add(1)
}

// RecordCacheHit records cache hit
func (ms *MetricsService) RecordCacheHit() {}

// RecordCacheMiss records cache miss
func (ms *MetricsService) RecordCacheMiss() {}

// RecordToolValidation records tool validation duration
func (ms *MetricsService) RecordToolValidation(duration time.Duration) {}

// RecordAccountPoolWait records account pool wait
func (ms *MetricsService) RecordAccountPoolWait(duration time.Duration) {}

// RecordAccountPoolError records account pool error
func (ms *MetricsService) RecordAccountPoolError() {}

// GetQPS returns current QPS
func (ms *MetricsService) GetQPS() float64 {
	ms.recentMu.Lock()
	defer ms.recentMu.Unlock()

	now := time.Now()
	cutoff := now.Add(-1 * time.Minute)
	startIdx := 0
	for startIdx < len(ms.recentRequests) && ms.recentRequests[startIdx].Before(cutoff) {
		startIdx++
	}
	if startIdx > 0 {
		newRecent := make([]time.Time, len(ms.recentRequests)-startIdx)
		copy(newRecent, ms.recentRequests[startIdx:])
		ms.recentRequests = newRecent
	}

	if len(ms.recentRequests) == 0 {
		return 0
	}

	return math.Round(float64(len(ms.recentRequests))/60.0*1000) / 1000
}

// GetRequestStats returns current stats snapshot
func (ms *MetricsService) GetRequestStats() core.RequestStats {
	ms.flushBuffer()
	ms.historyMu.RLock()
	defer ms.historyMu.RUnlock()

	historyCopy := make([]core.RequestRecord, len(ms.requestHistory))
	copy(historyCopy, ms.requestHistory)

	return core.RequestStats{
		TotalRequests:      ms.atomicStats.TotalRequests.Load(),
		SuccessfulRequests: ms.atomicStats.SuccessfulRequests.Load(),
		FailedRequests:     ms.atomicStats.FailedRequests.Load(),
		TotalResponseTime:  ms.atomicStats.TotalResponseTime.Load(),
		LastRequestTime:    ms.lastRequestTime,
		RequestHistory:     historyCopy,
	}
}

// GetPeriodStats computes period statistics for multiple hour windows in a single pass.
func GetPeriodStats(history []core.RequestRecord, hourPeriods ...int) map[int]core.PeriodStats {
	if len(hourPeriods) == 0 {
		return nil
	}

	now := time.Now()
	cutoffs := make([]time.Time, len(hourPeriods))
	requests := make([]int64, len(hourPeriods))
	successful := make([]int64, len(hourPeriods))
	responseTime := make([]int64, len(hourPeriods))

	for i, hours := range hourPeriods {
		cutoffs[i] = now.Add(-time.Duration(hours) * time.Hour)
	}

	for _, record := range history {
		for i, cutoff := range cutoffs {
			if record.Timestamp.After(cutoff) {
				requests[i]++
				responseTime[i] += record.ResponseTime
				if record.Success {
					successful[i]++
				}
			}
		}
	}

	result := make(map[int]core.PeriodStats, len(hourPeriods))
	for i, hours := range hourPeriods {
		stats := core.PeriodStats{
			Requests: requests[i],
			QPS:      float64(requests[i]) / (float64(hours) * 3600.0),
		}
		if requests[i] > 0 {
			stats.SuccessRate = float64(successful[i]) / float64(requests[i]) * 100
			stats.AvgResponseTime = responseTime[i] / requests[i]
		}
		result[hours] = stats
	}
	return result
}

// LoadStats loads stats from storage
func (ms *MetricsService) LoadStats() error {
	if ms.storage == nil {
		return nil
	}
	stats, err := ms.storage.LoadStats()
	if err != nil {
		return err
	}

	ms.atomicStats.TotalRequests.Store(stats.TotalRequests)
	ms.atomicStats.SuccessfulRequests.Store(stats.SuccessfulRequests)
	ms.atomicStats.FailedRequests.Store(stats.FailedRequests)
	ms.atomicStats.TotalResponseTime.Store(stats.TotalResponseTime)
	ms.lastRequestTime = stats.LastRequestTime

	ms.historyMu.Lock()
	ms.requestHistory = stats.RequestHistory
	ms.historyMu.Unlock()

	return nil
}

// SaveStatsDebounced saves stats with debounce
func (ms *MetricsService) SaveStatsDebounced() {
	now := time.Now()
	ms.historyMu.Lock()
	if now.Sub(ms.lastSaveTime) < ms.minSaveInterval {
		ms.historyMu.Unlock()
		return
	}
	ms.lastSaveTime = now
	ms.historyMu.Unlock()

	if ms.storage == nil {
		return
	}

	stats := ms.GetRequestStats()
	if err := ms.storage.SaveStats(&stats); err != nil {
		ms.logger.Warn("Failed to save stats: %v", err)
	}
}

// Close saves final stats and stops
func (ms *MetricsService) Close() error {
	close(ms.done)
	ms.bufferFlushTimer.Stop()
	ms.flushBuffer()

	if ms.storage != nil {
		stats := ms.GetRequestStats()
		return ms.storage.SaveStats(&stats)
	}
	return nil
}

// RecordSuccessWithMetrics records successful request
func RecordSuccessWithMetrics(metrics *MetricsService, startTime time.Time, model, account string) {
	metrics.RecordRequest(true, time.Since(startTime).Milliseconds(), model, account)
}

// RecordFailureWithMetrics records failed request
func RecordFailureWithMetrics(metrics *MetricsService, startTime time.Time, model, account string) {
	metrics.RecordRequest(false, time.Since(startTime).Milliseconds(), model, account)
}

// ShowStatsPage serves the stats HTML page
func ShowStatsPage(c *gin.Context) {
	data, err := StatsPageHTML.ReadFile("static/index.html")
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to load stats page")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

// StreamLog serves the log stream endpoint
func StreamLog(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	_, _ = fmt.Fprintf(c.Writer, "data: Log stream is alive\n\n")
	c.Writer.Flush()
}
