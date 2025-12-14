package main

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
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

	// 优雅关闭支持
	ctx    context.Context
	cancel context.CancelFunc
}

// NewAtomicRequestStats 创建新的原子统计结构
func NewAtomicRequestStats() *AtomicRequestStats {
	ctx, cancel := context.WithCancel(context.Background())
	stats := &AtomicRequestStats{
		historyChannel: make(chan RequestRecord, HistoryBufferSize),
		historyBuffer:  make([]RequestRecord, 0, HistoryBufferSize),
		ctx:            ctx,
		cancel:         cancel,
	}

	// 启动后台 worker 处理历史记录
	go stats.historyWorker()

	return stats
}

// Stop 停止 historyWorker goroutine
func (s *AtomicRequestStats) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

// historyWorker 后台处理请求历史记录，避免阻塞主路径
func (s *AtomicRequestStats) historyWorker() {
	ticker := time.NewTicker(HistoryFlushInterval)
	defer ticker.Stop()

	batch := make([]RequestRecord, 0, HistoryBatchSize)

	for {
		select {
		case <-s.ctx.Done():
			// 收到关闭信号，刷新剩余批次后退出
			if len(batch) > 0 {
				s.flushHistoryBatch(batch)
			}
			return

		case record := <-s.historyChannel:
			batch = append(batch, record)

			// 批量处理，减少锁竞争
			if len(batch) >= HistoryBatchSize {
				s.flushHistoryBatch(batch)
				batch = make([]RequestRecord, 0, HistoryBatchSize)
			}

		case <-ticker.C:
			// 定期刷新批次
			if len(batch) > 0 {
				s.flushHistoryBatch(batch)
				batch = make([]RequestRecord, 0, HistoryBatchSize)
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
		if len(s.historyBuffer) > HistoryBufferSize {
			// 保留最新的 HistoryBufferSize 条记录
			s.historyBuffer = s.historyBuffer[len(s.historyBuffer)-HistoryBufferSize:]
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

// LoadFromStats 从 RequestStats 加载数据
func (s *AtomicRequestStats) LoadFromStats(stats *RequestStats) {
	if stats == nil {
		return
	}

	// 从加载的数据初始化原子统计
	for _, record := range stats.RequestHistory {
		select {
		case s.historyChannel <- record:
		default:
			// Channel 满了，跳过旧记录
		}
	}
	atomic.StoreInt64(&s.totalRequests, stats.TotalRequests)
	atomic.StoreInt64(&s.successfulRequests, stats.SuccessfulRequests)
	atomic.StoreInt64(&s.failedRequests, stats.FailedRequests)
	atomic.StoreInt64(&s.totalResponseTime, stats.TotalResponseTime)
	if !stats.LastRequestTime.IsZero() {
		s.lastRequestTime.Store(stats.LastRequestTime)
	}
}

// PerformanceMetrics 性能指标收集器
type PerformanceMetrics struct {
	mu sync.RWMutex

	// HTTP相关指标
	httpRequests    int64
	httpErrors      int64
	avgResponseTime float64

	// 缓存相关指标
	cacheHits    int64
	cacheMisses  int64
	cacheHitRate float64

	// 工具验证相关指标
	toolValidations    int64
	toolValidationTime float64

	// 账户管理相关指标
	accountPoolWait   int64
	accountPoolErrors int64

	// QPS 计算
	windowStartTime time.Time
	windowRequests  int64
}

// NewPerformanceMetrics 创建新的性能指标
func NewPerformanceMetrics() *PerformanceMetrics {
	return &PerformanceMetrics{
		windowStartTime: time.Now(),
	}
}

// MetricsService 统一的指标服务
// SRP: 单一职责 - 只负责指标收集、统计和持久化
type MetricsService struct {
	// 请求统计
	requestStats *AtomicRequestStats

	// 性能指标
	perfMetrics *PerformanceMetrics

	// 配置
	saveInterval time.Duration
	historySize  int

	// 依赖
	storage StorageInterface
	logger  Logger

	// 持久化控制
	lastSaveTime   int64 // 上次保存的时间戳（原子操作）
	pendingSave    int32 // 是否有待保存的数据（原子操作）
	saveChan       chan bool
	saveWorkerOnce sync.Once

	// 优雅关闭
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// MetricsConfig 指标服务配置
type MetricsConfig struct {
	SaveInterval time.Duration
	HistorySize  int
	Storage      StorageInterface
	Logger       Logger
}

// NewMetricsService 创建新的指标服务
func NewMetricsService(config MetricsConfig) *MetricsService {
	ctx, cancel := context.WithCancel(context.Background())

	ms := &MetricsService{
		requestStats: NewAtomicRequestStats(),
		perfMetrics:  NewPerformanceMetrics(),
		saveInterval: config.SaveInterval,
		historySize:  config.HistorySize,
		storage:      config.Storage,
		logger:       config.Logger,
		saveChan:     make(chan bool, 100),
		ctx:          ctx,
		cancel:       cancel,
	}

	// 启动异步保存工作协程
	ms.saveWorkerOnce.Do(func() {
		ms.wg.Add(1)
		go ms.saveWorker()
	})

	return ms
}

// RecordRequest 记录请求
func (ms *MetricsService) RecordRequest(success bool, responseTime int64, model, account string) {
	// 使用原子统计，完全无锁
	ms.requestStats.RecordRequest(success, responseTime, model, account)

	// 触发异步持久化
	ms.triggerAsyncSave()
}

// RecordHTTPRequest 记录HTTP请求
func (ms *MetricsService) RecordHTTPRequest(duration time.Duration) {
	ms.perfMetrics.mu.Lock()
	defer ms.perfMetrics.mu.Unlock()

	ms.perfMetrics.httpRequests++
	ms.perfMetrics.windowRequests++

	// 计算平均响应时间（指数移动平均）
	if ms.perfMetrics.avgResponseTime == 0 {
		ms.perfMetrics.avgResponseTime = float64(duration.Milliseconds())
	} else {
		ms.perfMetrics.avgResponseTime = (ms.perfMetrics.avgResponseTime*0.9 + float64(duration.Milliseconds())*0.1)
	}
}

// RecordHTTPError 记录HTTP错误
func (ms *MetricsService) RecordHTTPError() {
	ms.perfMetrics.mu.Lock()
	defer ms.perfMetrics.mu.Unlock()

	ms.perfMetrics.httpErrors++
}

// RecordCacheHit 记录缓存命中
func (ms *MetricsService) RecordCacheHit() {
	ms.perfMetrics.mu.Lock()
	defer ms.perfMetrics.mu.Unlock()

	ms.perfMetrics.cacheHits++

	// 计算缓存命中率
	total := ms.perfMetrics.cacheHits + ms.perfMetrics.cacheMisses
	if total > 0 {
		ms.perfMetrics.cacheHitRate = float64(ms.perfMetrics.cacheHits) / float64(total)
	}
}

// RecordCacheMiss 记录缓存未命中
func (ms *MetricsService) RecordCacheMiss() {
	ms.perfMetrics.mu.Lock()
	defer ms.perfMetrics.mu.Unlock()

	ms.perfMetrics.cacheMisses++

	// 计算缓存命中率
	total := ms.perfMetrics.cacheHits + ms.perfMetrics.cacheMisses
	if total > 0 {
		ms.perfMetrics.cacheHitRate = float64(ms.perfMetrics.cacheHits) / float64(total)
	}
}

// RecordToolValidation 记录工具验证
func (ms *MetricsService) RecordToolValidation(duration time.Duration) {
	ms.perfMetrics.mu.Lock()
	defer ms.perfMetrics.mu.Unlock()

	ms.perfMetrics.toolValidations++

	// 计算平均验证时间（指数移动平均）
	if ms.perfMetrics.toolValidationTime == 0 {
		ms.perfMetrics.toolValidationTime = float64(duration.Milliseconds())
	} else {
		ms.perfMetrics.toolValidationTime = (ms.perfMetrics.toolValidationTime*0.9 + float64(duration.Milliseconds())*0.1)
	}
}

// RecordAccountPoolWait 记录账户池等待
func (ms *MetricsService) RecordAccountPoolWait(duration time.Duration) {
	ms.perfMetrics.mu.Lock()
	defer ms.perfMetrics.mu.Unlock()

	ms.perfMetrics.accountPoolWait++
}

// RecordAccountPoolError 记录账户池错误
func (ms *MetricsService) RecordAccountPoolError() {
	ms.perfMetrics.mu.Lock()
	defer ms.perfMetrics.mu.Unlock()

	ms.perfMetrics.accountPoolErrors++
}

// GetQPS 获取当前QPS
func (ms *MetricsService) GetQPS() float64 {
	ms.perfMetrics.mu.RLock()
	defer ms.perfMetrics.mu.RUnlock()

	windowDuration := time.Since(ms.perfMetrics.windowStartTime).Seconds()
	if windowDuration == 0 {
		return 0
	}

	return float64(ms.perfMetrics.windowRequests) / windowDuration
}

// GetRequestStats 获取请求统计
func (ms *MetricsService) GetRequestStats() RequestStats {
	return ms.requestStats.ToRequestStats()
}

// LoadStats 加载统计数据
func (ms *MetricsService) LoadStats() error {
	if ms.storage == nil {
		return nil
	}

	stats, err := ms.storage.LoadStats()
	if err != nil {
		if ms.logger != nil {
			ms.logger.Error("Error loading stats: %v", err)
		}
		return err
	}

	ms.requestStats.LoadFromStats(stats)

	if ms.logger != nil {
		ms.logger.Info("Successfully loaded %d request records", len(stats.RequestHistory))
	}

	return nil
}

// saveWorker 异步保存工作协程
func (ms *MetricsService) saveWorker() {
	defer ms.wg.Done()

	for {
		select {
		case <-ms.saveChan:
			// 检查是否需要保存（防抖机制）
			now := time.Now().Unix()
			lastSave := atomic.LoadInt64(&ms.lastSaveTime)

			if now-lastSave >= int64(ms.saveInterval.Seconds()) {
				// 执行实际的保存操作
				ms.saveStats()
				atomic.StoreInt64(&ms.lastSaveTime, now)
				atomic.StoreInt32(&ms.pendingSave, 0)
			} else {
				// 延迟保存
				time.Sleep(ms.saveInterval - time.Duration(now-lastSave)*time.Second)
				ms.saveStats()
				atomic.StoreInt64(&ms.lastSaveTime, time.Now().Unix())
				atomic.StoreInt32(&ms.pendingSave, 0)
			}

		case <-ms.ctx.Done():
			// 优雅关闭：保存最终状态
			ms.saveStats()
			return
		}
	}
}

// triggerAsyncSave 触发异步保存（非阻塞）
func (ms *MetricsService) triggerAsyncSave() {
	// 使用原子操作检查是否已有待保存的请求
	if atomic.CompareAndSwapInt32(&ms.pendingSave, 0, 1) {
		select {
		case ms.saveChan <- true:
			// 成功发送保存信号
		default:
			// 通道已满，重置状态
			atomic.StoreInt32(&ms.pendingSave, 0)
		}
	}
}

// saveStats 保存统计数据
func (ms *MetricsService) saveStats() {
	if ms.storage == nil {
		return
	}

	stats := ms.requestStats.ToRequestStats()
	if err := ms.storage.SaveStats(&stats); err != nil {
		if ms.logger != nil {
			ms.logger.Error("Error saving stats: %v", err)
		}
	}
}

// Close 优雅关闭指标服务
func (ms *MetricsService) Close() error {
	// 停止保存协程
	if ms.cancel != nil {
		ms.cancel()
	}

	// 停止 requestStats 的 historyWorker goroutine
	if ms.requestStats != nil {
		ms.requestStats.Stop()
	}

	// 等待所有 goroutine 完成
	ms.wg.Wait()

	return nil
}

// ==================== HTTP Handler 函数 ====================

// showStatsPage 显示统计页面
func showStatsPage(c *gin.Context) {
	c.File("./static/index.html")
}

// streamLog 流式日志输出
func streamLog(c *gin.Context) {
	setStreamingHeaders(c, "")

	// Keep the connection open
	<-c.Request.Context().Done()
}
