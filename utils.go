package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ============================================================================
// 环境和配置工具
// ============================================================================

// IsDebug 返回应用是否运行在调试模式
// 保留此函数以维持向下兼容性
func IsDebug() bool {
	return gin.Mode() == gin.DebugMode
}

// getEnvWithDefault 获取环境变量，如果不存在则返回默认值
func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// parseEnvList 解析逗号分隔的环境变量为去空格的切片
func parseEnvList(envVar string) []string {
	if envVar == "" {
		return nil
	}
	parts := strings.Split(envVar, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// ============================================================================
// HTTP 工具
// ============================================================================

// createJetbrainsRequest 创建 JetBrains API HTTP 请求，设置标准头部
func createJetbrainsRequest(method, url string, payload any, authorization string) (*http.Request, error) {
	var body io.Reader

	if payload != nil {
		payloadBytes, err := marshalJSON(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewBuffer(payloadBytes)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set(HeaderContentType, ContentTypeJSON)
	if authorization != "" {
		req.Header.Set("authorization", AuthBearerPrefix+authorization)
	}

	return req, nil
}

// ============================================================================
// 消息处理工具
// ============================================================================

// extractTextContent 从消息的 content 字段提取文本
// 支持 string 和 []ContentBlock 两种格式
func extractTextContent(content any) string {
	if content == nil {
		return ""
	}

	switch v := content.(type) {
	case string:
		return v
	case []any:
		var textParts []string
		for _, item := range v {
			if itemMap, ok := item.(map[string]any); ok {
				if itemType, ok := itemMap["type"].(string); ok && itemType == ContentBlockTypeText {
					if text, ok := itemMap["text"].(string); ok {
						textParts = append(textParts, text)
					}
				}
			}
		}
		return strings.Join(textParts, " ")
	}
	return ""
}

// ============================================================================
// 统计记录工具
// ============================================================================

// recordSuccessWithMetrics 记录成功的请求（使用注入的 MetricsService）
func recordSuccessWithMetrics(metrics *MetricsService, startTime time.Time, model, account string) {
	metrics.RecordRequest(true, time.Since(startTime).Milliseconds(), model, account)
}

// recordFailureWithMetrics 记录失败的请求（使用注入的 MetricsService）
func recordFailureWithMetrics(metrics *MetricsService, startTime time.Time, model, account string) {
	metrics.RecordRequest(false, time.Since(startTime).Milliseconds(), model, account)
	metrics.RecordHTTPError()
}

// ============================================================================
// 账户显示名称工具
// ============================================================================

// truncateString 截断字符串并在中间添加替换文本
func truncateString(s string, prefixLen, suffixLen int, replacement string) string {
	if len(s) > prefixLen+suffixLen {
		return s[:prefixLen] + replacement + s[len(s)-suffixLen:]
	}
	return s
}

// generateID 生成带前缀的唯一ID（基于纳秒时间戳）
// 适用于需要时间序可追溯的场景，如响应ID、消息ID
func generateID(prefix string) string {
	return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
}

// generateRandomID 生成带前缀的随机ID（基于加密安全随机数）
// 适用于需要不可预测性的场景，如工具调用ID
func generateRandomID(prefix string) string {
	bytes := make([]byte, 10)
	_, _ = rand.Read(bytes)
	return fmt.Sprintf("%s%s", prefix, hex.EncodeToString(bytes))
}

// getTokenDisplayName 获取账户的显示名称（用于日志）
func getTokenDisplayName(account *JetbrainsAccount) string {
	if account == nil {
		return "Token Unknown"
	}

	account.mu.Lock()
	jwt := account.JWT
	licenseID := account.LicenseID
	account.mu.Unlock()

	if jwt != "" {
		return truncateString(jwt, 0, 6, "Token ...")
	}
	if licenseID != "" {
		return truncateString(licenseID, 0, 6, "Token ...")
	}
	return "Token Unknown"
}

// getLicenseDisplayName 获取许可证的显示名称（用于统计页面）
func getLicenseDisplayName(account *JetbrainsAccount) string {
	if account == nil {
		return "Unknown"
	}

	account.mu.Lock()
	authorization := account.Authorization
	account.mu.Unlock()

	if authorization != "" {
		return truncateString(authorization, 3, 3, "*")
	}
	return "Unknown"
}

// getTokenInfoFromAccount 获取账户的Token信息（用于统计页面）
func getTokenInfoFromAccount(account *JetbrainsAccount, httpClient *http.Client, cache *CacheService, logger Logger) (*TokenInfo, error) {
	quotaData, err := getQuotaData(account, httpClient, cache, logger)
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

	account.mu.Lock()
	hasQuota := account.HasQuota
	expiryTime := account.ExpiryTime
	account.mu.Unlock()

	status := AccountStatusNormal
	if !hasQuota {
		status = AccountStatusNoQuota
	} else if time.Now().Add(AccountExpiryWarningTime).After(expiryTime) {
		status = AccountStatusExpiring
	}

	return &TokenInfo{
		Name:       getTokenDisplayName(account),
		License:    getLicenseDisplayName(account),
		Used:       dailyUsed,
		Total:      dailyTotal,
		UsageRate:  usageRate,
		ExpiryDate: expiryTime,
		Status:     status,
		HasQuota:   hasQuota,
	}, nil
}


// estimateTokenCount 估算 token 数量
// 简单估算：平均每个 token 约 4 个字符
func estimateTokenCount(text string) int {
	return len(text) / 4
}
