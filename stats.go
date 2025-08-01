package main

import (
	"fmt"
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
)

// 用于控制异步持久化的变量
var (
	lastSaveTime   int64 // 上次保存的时间戳（原子操作）
	pendingSave    int32 // 是否有待保存的数据（原子操作）
	saveChan       chan bool
	saveWorkerOnce sync.Once
)

// saveStats saves the current request statistics using the configured storage
func saveStats() {
	saveStatsWithStorage()
}

// loadStats loads request statistics using the configured storage
func loadStats() {
	loadStatsWithStorage()
}

// showStatsPage 显示统计页面
func showStatsPage(c *gin.Context) {
	// 提供静态HTML文件
	c.File("./static/index.html")
}

// getStatsData 获取统计数据的JSON API端点
func getStatsData(c *gin.Context) {
	// 获取Token信息
	var tokensInfo []gin.H
	for i := range jetbrainsAccounts {
		tokenInfo, err := getTokenInfoFromAccount(&jetbrainsAccounts[i])
		if err != nil {
			tokensInfo = append(tokensInfo, gin.H{
				"name":       getTokenDisplayName(&jetbrainsAccounts[i]),
				"license":    "",
				"used":       0.0,
				"total":      0.0,
				"usageRate":  0.0,
				"expiryDate": "",
				"status":     "错误",
			})
		} else {
			tokensInfo = append(tokensInfo, gin.H{
				"name":       tokenInfo.Name,
				"license":    tokenInfo.License,
				"used":       tokenInfo.Used,
				"total":      tokenInfo.Total,
				"usageRate":  tokenInfo.UsageRate,
				"expiryDate": tokenInfo.ExpiryDate.Format("2006-01-02 15:04:05"),
				"status":     tokenInfo.Status,
			})
		}
	}

	// 获取统计数据
	stats24h := getPeriodStats(24)
	stats7d := getPeriodStats(24 * 7)
	stats30d := getPeriodStats(24 * 30)
	currentQPS := getCurrentQPS()

	// 准备Token过期监控数据
	var expiryInfo []gin.H
	for i := range jetbrainsAccounts {
		account := &jetbrainsAccounts[i]
		expiryTime := account.ExpiryTime

		status := "正常"
		warning := "正常"
		if time.Now().Add(1 * time.Hour).After(expiryTime) {
			status = "即将过期"
			warning = "即将过期"
		}

		expiryInfo = append(expiryInfo, gin.H{
			"name":       getTokenDisplayName(account),
			"expiryTime": expiryTime.Format("2006-01-02 15:04:05"),
			"status":     status,
			"warning":    warning,
		})
	}

	// 返回JSON数据
	c.JSON(200, gin.H{
		"currentTime":  time.Now().Format("2006-01-02 15:04:05"),
		"currentQPS":   fmt.Sprintf("%.3f", currentQPS),
		"totalRecords": len(requestStats.RequestHistory),
		"stats24h":     stats24h,
		"stats7d":      stats7d,
		"stats30d":     stats30d,
		"tokensInfo":   tokensInfo,
		"expiryInfo":   expiryInfo,
	})
}

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
		go saveWorker() // 启动异步保存工作协程
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
func recordRequest(success bool, responseTime int64, model, account string) {
	statsMutex.Lock()
	defer statsMutex.Unlock()

	requestStats.TotalRequests++
	requestStats.LastRequestTime = time.Now()
	requestStats.TotalResponseTime += responseTime

	if success {
		requestStats.SuccessfulRequests++
	} else {
		requestStats.FailedRequests++
	}

	// Add to history (keep last 1000 records)
	record := RequestRecord{
		Timestamp:    time.Now(),
		Success:      success,
		ResponseTime: responseTime,
		Model:        model,
		Account:      account,
	}

	requestStats.RequestHistory = append(requestStats.RequestHistory, record)
	if len(requestStats.RequestHistory) > 1000 {
		requestStats.RequestHistory = requestStats.RequestHistory[1:]
	}

	// 触发异步持久化
	triggerAsyncSave()
}

func getPeriodStats(hours int) PeriodStats {
	statsMutex.Lock()
	defer statsMutex.Unlock()

	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	var periodRequests int64
	var periodSuccessful int64
	var periodResponseTime int64

	for _, record := range requestStats.RequestHistory {
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
	statsMutex.Lock()
	defer statsMutex.Unlock()

	now := time.Now()
	cutoff := now.Add(-1 * time.Minute)
	var recentRequests int64

	for _, record := range requestStats.RequestHistory {
		if record.Timestamp.After(cutoff) {
			recentRequests++
		}
	}

	return float64(recentRequests) / 60.0
}

func getTokenInfoFromAccount(account *JetbrainsAccount) (*TokenInfo, error) {
	quotaData, err := getQuotaData(account)
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
