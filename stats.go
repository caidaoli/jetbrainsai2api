package main

import (
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	statsFilePath = "stats.json"
	// 控制持久化频率，避免过于频繁的写操作
	minSaveInterval = 5 * time.Second
	// 请求历史记录的缓冲区大小
	historyBufferSize = 1000
)

// 用于控制异步持久化的变量
var (
	lastSaveTime   int64 // 上次保存的时间戳（原子操作）
	pendingSave    int32 // 是否有待保存的数据（原子操作）
	saveChan       chan bool
	saveWorkerOnce sync.Once
)

// AtomicRequestStats 使用 atomic 操作的高性能统计结构
// 避免每次请求都获取互斥锁，显著提升并发性能
type AtomicRequestStats struct {
	totalRequests      int64 // 使用 atomic 操作
	successfulRequests int64 // 使用 atomic 操作
	failedRequests     int64 // 使用 atomic 操作
	totalResponseTime  int64 // 使用 atomic 操作

	// 请求历史使用无锁的 channel 缓冲
	historyChannel chan RequestRecord
	historyBuffer  []RequestRecord
	historyMutex   sync.RWMutex // 仅在读取历史时使用

	lastRequestTime atomic.Value // 存储 time.Time
}

// NewAtomicRequestStats 创建新的原子统计结构
func NewAtomicRequestStats() *AtomicRequestStats {
	stats := &AtomicRequestStats{
		historyChannel: make(chan RequestRecord, historyBufferSize),
		historyBuffer:  make([]RequestRecord, 0, historyBufferSize),
	}

	// 启动后台 worker 处理历史记录
	go stats.historyWorker()

	return stats
}

// historyWorker 后台处理请求历史记录，避免阻塞主路径
func (s *AtomicRequestStats) historyWorker() {
	ticker := time.NewTicker(100 * time.Millisecond) // 每100ms批量处理
	defer ticker.Stop()

	batch := make([]RequestRecord, 0, 100)

	for {
		select {
		case record := <-s.historyChannel:
			batch = append(batch, record)

			// 批量处理，减少锁竞争
			if len(batch) >= 100 {
				s.flushHistoryBatch(batch)
				batch = make([]RequestRecord, 0, 100)
			}

		case <-ticker.C:
			// 定期刷新批次
			if len(batch) > 0 {
				s.flushHistoryBatch(batch)
				batch = make([]RequestRecord, 0, 100)
			}
		}
	}
}

// flushHistoryBatch 批量刷新历史记录
func (s *AtomicRequestStats) flushHistoryBatch(batch []RequestRecord) {
	s.historyMutex.Lock()
	defer s.historyMutex.Unlock()

	for _, record := range batch {
		s.historyBuffer = append(s.historyBuffer, record)
		if len(s.historyBuffer) > historyBufferSize {
			// 保留最新的 historyBufferSize 条记录
			s.historyBuffer = s.historyBuffer[len(s.historyBuffer)-historyBufferSize:]
		}
	}
}

// RecordRequest 记录请求（无锁高性能版本）
func (s *AtomicRequestStats) RecordRequest(success bool, responseTime int64, model, account string) {
	// 原子操作，无需锁
	atomic.AddInt64(&s.totalRequests, 1)
	atomic.AddInt64(&s.totalResponseTime, responseTime)

	if success {
		atomic.AddInt64(&s.successfulRequests, 1)
	} else {
		atomic.AddInt64(&s.failedRequests, 1)
	}

	// 更新最后请求时间
	s.lastRequestTime.Store(time.Now())

	// 非阻塞发送到历史 channel
	record := RequestRecord{
		Timestamp:    time.Now(),
		Success:      success,
		ResponseTime: responseTime,
		Model:        model,
		Account:      account,
	}

	select {
	case s.historyChannel <- record:
		// 成功发送
	default:
		// Channel 满了，丢弃最旧的记录（避免阻塞）
		// 这比阻塞主请求路径更好
	}
}

// ToRequestStats 转换为可序列化的 RequestStats 结构
func (s *AtomicRequestStats) ToRequestStats() RequestStats {
	s.historyMutex.RLock()
	history := make([]RequestRecord, len(s.historyBuffer))
	copy(history, s.historyBuffer)
	s.historyMutex.RUnlock()

	var lastTime time.Time
	if t := s.lastRequestTime.Load(); t != nil {
		lastTime = t.(time.Time)
	}

	return RequestStats{
		TotalRequests:      atomic.LoadInt64(&s.totalRequests),
		SuccessfulRequests: atomic.LoadInt64(&s.successfulRequests),
		FailedRequests:     atomic.LoadInt64(&s.failedRequests),
		TotalResponseTime:  atomic.LoadInt64(&s.totalResponseTime),
		LastRequestTime:    lastTime,
		RequestHistory:     history,
	}
}

// GetHistory 获取请求历史的副本
func (s *AtomicRequestStats) GetHistory() []RequestRecord {
	s.historyMutex.RLock()
	defer s.historyMutex.RUnlock()

	history := make([]RequestRecord, len(s.historyBuffer))
	copy(history, s.historyBuffer)
	return history
}

// 全局原子统计实例
var atomicStats = NewAtomicRequestStats()

// saveStats saves the current request statistics using the configured storage
func saveStats() {
	// 从原子统计转换为可序列化的结构
	_ = atomicStats.ToRequestStats()

	// 直接保存，不需要全局变量
	saveStatsWithStorage()
}

// loadStats loads request statistics using the configured storage
func loadStats() {
	stats := loadStatsWithStorage()
	if stats == nil {
		return
	}

	// 从加载的数据初始化原子统计
	for _, record := range stats.RequestHistory {
		select {
		case atomicStats.historyChannel <- record:
		default:
			// Channel 满了，跳过旧记录
		}
	}
	atomic.StoreInt64(&atomicStats.totalRequests, stats.TotalRequests)
	atomic.StoreInt64(&atomicStats.successfulRequests, stats.SuccessfulRequests)
	atomic.StoreInt64(&atomicStats.failedRequests, stats.FailedRequests)
	atomic.StoreInt64(&atomicStats.totalResponseTime, stats.TotalResponseTime)
	if !stats.LastRequestTime.IsZero() {
		atomicStats.lastRequestTime.Store(stats.LastRequestTime)
	}
}

// showStatsPage 显示统计页面
func showStatsPage(c *gin.Context) {
	// 提供静态HTML文件
	c.File("./static/index.html")
}

// getStatsData 已移至 server.go 作为 Server 方法
// 保留此注释以维持向后兼容性

// streamLog 流式日志输出
func streamLog(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	// Keep the connection open
	<-c.Request.Context().Done()
}

func truncateString(s string, prefixLen, suffixLen int, replacement string) string {
	if len(s) > prefixLen+suffixLen {
		return s[:prefixLen] + replacement + s[len(s)-suffixLen:]
	}
	return s
}

func getTokenDisplayName(account *JetbrainsAccount) string {
	if account.JWT != "" {
		return truncateString(account.JWT, 0, 6, "Token ...")
	}
	if account.LicenseID != "" {
		return truncateString(account.LicenseID, 0, 6, "Token ...")
	}
	return "Token Unknown"
}

func getLicenseDisplayName(account *JetbrainsAccount) string {
	if account.Authorization != "" {
		return truncateString(account.Authorization, 3, 3, "*")
	}
	return "Unknown"
}

// initRequestTriggeredSaving 初始化基于请求触发的持久化机制
func initRequestTriggeredSaving() {
	saveWorkerOnce.Do(func() {
		saveChan = make(chan bool, 100) // 缓冲通道，避免阻塞
		go saveWorker()                 // 启动异步保存工作协程
	})
}

// saveWorker 异步保存工作协程
func saveWorker() {
	for range saveChan {
		// 检查是否需要保存（防抖机制）
		now := time.Now().Unix()
		lastSave := atomic.LoadInt64(&lastSaveTime)

		if now-lastSave >= int64(minSaveInterval.Seconds()) {
			// 执行实际的保存操作
			saveStats()
			atomic.StoreInt64(&lastSaveTime, now)
			atomic.StoreInt32(&pendingSave, 0)
		} else {
			// 延迟保存
			time.Sleep(minSaveInterval - time.Duration(now-lastSave)*time.Second)
			saveStats()
			atomic.StoreInt64(&lastSaveTime, time.Now().Unix())
			atomic.StoreInt32(&pendingSave, 0)
		}
	}
}

// triggerAsyncSave 触发异步保存（非阻塞）
func triggerAsyncSave() {
	// 使用原子操作检查是否已有待保存的请求
	if atomic.CompareAndSwapInt32(&pendingSave, 0, 1) {
		select {
		case saveChan <- true:
			// 成功发送保存信号
		default:
			// 通道已满，重置状态
			atomic.StoreInt32(&pendingSave, 0)
		}
	}
}

// Statistics functions
// recordRequest 使用无锁原子操作的高性能版本
// 在高并发下性能显著优于互斥锁版本
func recordRequest(success bool, responseTime int64, model, account string) {
	// 使用原子统计，完全无锁
	atomicStats.RecordRequest(success, responseTime, model, account)

	// 触发异步持久化
	triggerAsyncSave()
}

func getPeriodStats(hours int) PeriodStats {
	// 从原子统计获取历史记录副本（无锁读取）
	history := atomicStats.GetHistory()

	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	var periodRequests int64
	var periodSuccessful int64
	var periodResponseTime int64

	for _, record := range history {
		if record.Timestamp.After(cutoff) {
			periodRequests++
			periodResponseTime += record.ResponseTime
			if record.Success {
				periodSuccessful++
			}
		}
	}

	stats := PeriodStats{
		Requests: periodRequests,
	}

	if periodRequests > 0 {
		stats.SuccessRate = float64(periodSuccessful) / float64(periodRequests) * 100
		stats.AvgResponseTime = periodResponseTime / periodRequests
	}

	// Calculate QPS based on the entire period
	stats.QPS = float64(periodRequests) / (float64(hours) * 3600.0)

	return stats
}

func getCurrentQPS() float64 {
	// 从原子统计获取历史记录副本
	history := atomicStats.GetHistory()

	now := time.Now()
	cutoff := now.Add(-1 * time.Minute)
	var recentRequests int64

	for _, record := range history {
		if record.Timestamp.After(cutoff) {
			recentRequests++
		}
	}

	return float64(recentRequests) / 60.0
}

func getTokenInfoFromAccount(account *JetbrainsAccount, httpClient *http.Client) (*TokenInfo, error) {
	quotaData, err := getQuotaData(account, httpClient)
	if err != nil {
		return &TokenInfo{
			Name:   getTokenDisplayName(account),
			Status: "错误",
		}, err
	}

	dailyUsed, _ := strconv.ParseFloat(quotaData.Current.Current.Amount, 64)
	dailyTotal, _ := strconv.ParseFloat(quotaData.Current.Maximum.Amount, 64)

	var usageRate float64
	if dailyTotal > 0 {
		usageRate = (dailyUsed / dailyTotal) * 100
	}

	status := "正常"
	if !account.HasQuota {
		status = "配额不足"
	} else if time.Now().Add(24 * time.Hour).After(account.ExpiryTime) {
		status = "即将过期"
	}

	return &TokenInfo{
		Name:       getTokenDisplayName(account),
		License:    getLicenseDisplayName(account),
		Used:       dailyUsed,
		Total:      dailyTotal,
		UsageRate:  usageRate,
		ExpiryDate: account.ExpiryTime,
		Status:     status,
		HasQuota:   account.HasQuota,
	}, nil
}
