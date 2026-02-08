package main

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"

	"github.com/golang-jwt/jwt/v5"
)

// setAccountQuotaStatus 线程安全地更新账户配额状态
func setAccountQuotaStatus(account *JetbrainsAccount, hasQuota bool, checkedAt time.Time) {
	if account == nil {
		return
	}

	account.mu.Lock()
	account.HasQuota = hasQuota
	account.LastQuotaCheck = float64(checkedAt.Unix())
	account.mu.Unlock()
}

// markAccountNoQuota 线程安全地将账户标记为无配额
func markAccountNoQuota(account *JetbrainsAccount) {
	setAccountQuotaStatus(account, false, time.Now())
}

// 注意：JWT 刷新不需要全局锁，因为：
// 1. 正常请求流程中，账户从 channel 获取，保证同一时间只有一个 goroutine 持有
// 2. 统计页面操作的是账户副本，不影响原始账户
// 3. AccountManager.RefreshJWT() 有自己的实例锁用于显式刷新场景

// setJetbrainsHeaders sets the required headers for JetBrains API requests
func setJetbrainsHeaders(req *http.Request, jwt string) {
	req.Header.Set("User-Agent", JetBrainsHeaderUserAgent)
	req.Header.Set(HeaderAcceptCharset, CharsetUTF8)
	req.Header.Set(HeaderGrazieAgent, JetBrainsHeaderGrazieAgent)
	if jwt != "" {
		req.Header.Set(HeaderGrazieAuthJWT, jwt)
	}
}

// handleJWTExpiredAndRetry handles JWT expiration and retries the request
// 注意：此函数假设调用者已经独占账户（通过 AcquireAccount），因此不需要锁
func handleJWTExpiredAndRetry(req *http.Request, account *JetbrainsAccount, httpClient *http.Client, logger Logger) (*http.Response, error) {
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == HTTPStatusUnauthorized && account.LicenseID != "" {
		_ = resp.Body.Close()
		logger.Info("JWT for %s expired, refreshing...", getTokenDisplayName(account))

		// 刷新 JWT（账户已被当前 goroutine 独占，无需锁）
		if err := refreshJetbrainsJWT(account, httpClient, logger); err != nil {
			return nil, err
		}

		req.Header.Set(HeaderGrazieAuthJWT, account.JWT)
		return httpClient.Do(req)
	}

	return resp, nil
}

// ensureValidJWT ensures that the account has a valid JWT
// 注意：此函数假设调用者已经独占账户（通过 AcquireAccount），因此不需要锁
func ensureValidJWT(account *JetbrainsAccount, httpClient *http.Client, logger Logger) error {
	if account.JWT == "" && account.LicenseID != "" {
		return refreshJetbrainsJWT(account, httpClient, logger)
	}
	return nil
}

// parseJWTExpiry 从 JWT 字符串中解析过期时间
// 用于静态 JWT 加载时设置过期时间
func parseJWTExpiry(tokenStr string) (time.Time, error) {
	// 验证 JWT 格式（三段式结构）
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("invalid JWT format: expected 3 parts, got %d", len(parts))
	}

	// SECURITY NOTE: 使用 ParseUnverified 是安全的，因为：
	// 1. 我们只读取 exp (过期时间) 字段用于本地提示
	// 2. 实际的 JWT 验证由 JetBrains API 在使用时完成
	token, _, err := new(jwt.Parser).ParseUnverified(tokenStr, jwt.MapClaims{})
	if err != nil {
		return time.Time{}, fmt.Errorf("could not parse JWT: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return time.Time{}, fmt.Errorf("invalid JWT claims format")
	}

	exp, ok := claims["exp"].(float64)
	if !ok {
		return time.Time{}, fmt.Errorf("JWT missing exp claim")
	}

	return time.Unix(int64(exp), 0), nil
}

// refreshJetbrainsJWT refreshes the JWT for a given JetBrains account
func refreshJetbrainsJWT(account *JetbrainsAccount, httpClient *http.Client, logger Logger) error {
	logger.Info("Refreshing JWT for licenseId %s...", account.LicenseID)

	payload := map[string]string{"licenseId": account.LicenseID}
	req, err := createJetbrainsRequest(http.MethodPost, JetBrainsJWTEndpoint, payload, account.Authorization)
	if err != nil {
		return err
	}
	setJetbrainsHeaders(req, "")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBodySize))
		return fmt.Errorf("JWT refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var data map[string]any
	if err := sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}

	state, _ := data["state"].(string)
	tokenStr, _ := data["token"].(string)

	if state == "PAID" && tokenStr != "" {
		// 安全检查：验证 JWT 格式（三段式结构）
		parts := strings.Split(tokenStr, ".")
		if len(parts) != 3 {
			return fmt.Errorf("invalid JWT format: expected 3 parts, got %d", len(parts))
		}

		account.JWT = tokenStr
		account.LastUpdated = float64(time.Now().Unix())

		// SECURITY NOTE: 使用 ParseUnverified 是安全的，因为：
		// 1. JWT 刚从 JetBrains 官方 API 获取，来源可信
		// 2. 我们只读取 exp (过期时间) 字段用于本地缓存管理
		// 3. 实际的 JWT 验证由 JetBrains API 在使用时完成
		// 不要将此模式用于验证用户提供的 token 或信任其他 claims
		token, _, err := new(jwt.Parser).ParseUnverified(tokenStr, jwt.MapClaims{})
		if err != nil {
			logger.Warn("could not parse JWT: %v", err)
		} else if claims, ok := token.Claims.(jwt.MapClaims); ok {
			if exp, ok := claims["exp"].(float64); ok {
				account.ExpiryTime = time.Unix(int64(exp), 0)
			}
		}

		logger.Info("Successfully refreshed JWT for licenseId %s, expires at %s", account.LicenseID, account.ExpiryTime.Format(time.RFC3339))
		return nil
	}

	return fmt.Errorf("JWT refresh failed: invalid response state %s", state)
}

// processQuotaData processes quota data and updates account status
func processQuotaData(quotaData *JetbrainsQuotaResponse, account *JetbrainsAccount, logger Logger) {
	dailyUsed, _ := strconv.ParseFloat(quotaData.Current.Current.Amount, 64)
	dailyTotal, _ := strconv.ParseFloat(quotaData.Current.Maximum.Amount, 64)

	if dailyTotal == 0 {
		dailyTotal = 1 // Avoid division by zero
	}

	hasQuota := dailyUsed < dailyTotal
	setAccountQuotaStatus(account, hasQuota, time.Now())

	if !hasQuota {
		logger.Warn("Account %s has no quota", getTokenDisplayName(account))
	}
}

// getQuotaData 获取配额数据（使用 CacheService）
func getQuotaData(account *JetbrainsAccount, httpClient *http.Client, cache *CacheService, logger Logger) (*JetbrainsQuotaResponse, error) {
	if err := ensureValidJWT(account, httpClient, logger); err != nil {
		return nil, fmt.Errorf("failed to refresh JWT: %w", err)
	}

	if account.JWT == "" {
		return nil, fmt.Errorf("account has no JWT")
	}

	// 使用统一的缓存键生成函数
	cacheKey := generateQuotaCacheKey(account)

	// 从 CacheService 获取缓存（已内置深拷贝防止 TOCTOU）
	if cache != nil {
		if cachedData, found := cache.GetQuotaCache(cacheKey); found {
			return cachedData, nil
		}
	}

	// 调用直接获取函数
	quotaData, err := getQuotaDataDirect(account, httpClient, cache, logger)
	if err != nil {
		return nil, err
	}

	// 更新缓存（使用 CacheService）
	if cache != nil {
		cache.SetQuotaCache(cacheKey, quotaData, QuotaCacheTime)
	}

	return quotaData, nil
}

// getQuotaDataDirect 直接从 JetBrains API 获取配额数据（不使用全局缓存）
// 注意: 调用者 (getQuotaData) 已确保 JWT 有效，此处无需重复检查
func getQuotaDataDirect(account *JetbrainsAccount, httpClient *http.Client, cache *CacheService, logger Logger) (*JetbrainsQuotaResponse, error) {
	req, err := http.NewRequest(http.MethodPost, JetBrainsQuotaEndpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Length", "0")
	setJetbrainsHeaders(req, account.JWT)

	resp, err := handleJWTExpiredAndRetry(req, account, httpClient, logger)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBodySize))
		// 如果是401，则JWT可能已失效，从缓存中删除
		if resp.StatusCode == HTTPStatusUnauthorized && cache != nil {
			cacheKey := generateQuotaCacheKey(account)
			cache.DeleteQuotaCache(cacheKey)
		}
		return nil, fmt.Errorf("quota check failed with status %d: %s", resp.StatusCode, string(body))
	}

	var quotaData JetbrainsQuotaResponse
	if err := sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&quotaData); err != nil {
		return nil, err
	}

	if IsDebug() {
		quotaJSON, _ := sonic.MarshalIndent(quotaData, "", "  ")
		logger.Debug("JetBrains Quota API Response: %s", string(quotaJSON))
	}

	processQuotaData(&quotaData, account, logger)

	return &quotaData, nil
}
