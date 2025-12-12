package main

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/bytedance/sonic"

	"github.com/golang-jwt/jwt/v5"
)

var (
	// jwtRefreshMutex 用于同步 JWT 刷新操作，防止多个 goroutine 同时刷新同一账户的 JWT
	// TODO: 考虑将此 mutex 移至 JetbrainsAccount 结构体中，实现 per-account 锁定，
	// 减少不同账户之间的锁竞争
	jwtRefreshMutex sync.Mutex
)

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
func handleJWTExpiredAndRetry(req *http.Request, account *JetbrainsAccount, httpClient *http.Client) (*http.Response, error) {
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == HTTPStatusUnauthorized && account.LicenseID != "" {
		resp.Body.Close()
		Info("JWT for %s expired, refreshing...", getTokenDisplayName(account))

		jwtRefreshMutex.Lock()
		// Check if another goroutine already refreshed the JWT
		if req.Header.Get(HeaderGrazieAuthJWT) == account.JWT {
			if err := refreshJetbrainsJWT(account, httpClient); err != nil {
				jwtRefreshMutex.Unlock()
				return nil, err
			}
		}
		jwtRefreshMutex.Unlock()

		req.Header.Set(HeaderGrazieAuthJWT, account.JWT)
		return httpClient.Do(req)
	}

	return resp, nil
}

// ensureValidJWT ensures that the account has a valid JWT
func ensureValidJWT(account *JetbrainsAccount, httpClient *http.Client) error {
	if account.JWT == "" && account.LicenseID != "" {
		jwtRefreshMutex.Lock()
		defer jwtRefreshMutex.Unlock()

		// Double-check after acquiring lock
		if account.JWT == "" {
			return refreshJetbrainsJWT(account, httpClient)
		}
	}
	return nil
}

// checkQuota checks the quota for a given JetBrains account
func checkQuota(account *JetbrainsAccount, httpClient *http.Client) error {
	// 如果最近检查过配额（1小时内），跳过检查
	if account.LastQuotaCheck > 0 {
		lastCheck := time.Unix(int64(account.LastQuotaCheck), 0)
		if time.Since(lastCheck) < QuotaCacheTime {
			// 配额检查仍然有效，跳过
			return nil
		}
	}

	quotaData, err := getQuotaData(account, httpClient)
	if err != nil {
		account.HasQuota = false
		return err
	}

	processQuotaData(quotaData, account)
	return nil
}

// refreshJetbrainsJWT refreshes the JWT for a given JetBrains account
func refreshJetbrainsJWT(account *JetbrainsAccount, httpClient *http.Client) error {
	Info("Refreshing JWT for licenseId %s...", account.LicenseID)

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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("JWT refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var data map[string]any
	if err := sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}

	state, _ := data["state"].(string)
	tokenStr, _ := data["token"].(string)

	if state == "PAID" && tokenStr != "" {
		account.JWT = tokenStr
		account.LastUpdated = float64(time.Now().Unix())

		// SECURITY NOTE: 使用 ParseUnverified 是安全的，因为：
		// 1. JWT 刚从 JetBrains 官方 API 获取，来源可信
		// 2. 我们只读取 exp (过期时间) 字段用于本地缓存管理
		// 3. 实际的 JWT 验证由 JetBrains API 在使用时完成
		// 不要将此模式用于验证用户提供的 token 或信任其他 claims
		token, _, err := new(jwt.Parser).ParseUnverified(tokenStr, jwt.MapClaims{})
		if err != nil {
			Warn("could not parse JWT: %v", err)
		} else if claims, ok := token.Claims.(jwt.MapClaims); ok {
			if exp, ok := claims["exp"].(float64); ok {
				account.ExpiryTime = time.Unix(int64(exp), 0)
			}
		}

		Info("Successfully refreshed JWT for licenseId %s, expires at %s", account.LicenseID, account.ExpiryTime.Format(time.RFC3339))
		return nil
	}

	return fmt.Errorf("JWT refresh failed: invalid response state %s", state)
}

// getNextJetbrainsAccount 已废弃 - 使用 AccountManager.AcquireAccount 替代
// 保留此函数签名以维持向后兼容性，但不应再使用

// processQuotaData processes quota data and updates account status
func processQuotaData(quotaData *JetbrainsQuotaResponse, account *JetbrainsAccount) {
	dailyUsed, _ := strconv.ParseFloat(quotaData.Current.Current.Amount, 64)
	dailyTotal, _ := strconv.ParseFloat(quotaData.Current.Maximum.Amount, 64)

	if dailyTotal == 0 {
		dailyTotal = 1 // Avoid division by zero
	}

	account.HasQuota = dailyUsed < dailyTotal
	if !account.HasQuota {
		Warn("Account %s has no quota", getTokenDisplayName(account))
	}

	account.LastQuotaCheck = float64(time.Now().Unix())
}

// getQuotaData 获取配额数据（使用 CacheService）
func getQuotaData(account *JetbrainsAccount, httpClient *http.Client) (*JetbrainsQuotaResponse, error) {
	if err := ensureValidJWT(account, httpClient); err != nil {
		return nil, fmt.Errorf("failed to refresh JWT: %w", err)
	}

	if account.JWT == "" {
		return nil, fmt.Errorf("account has no JWT")
	}

	// 使用统一的缓存键生成函数
	cacheKey := generateQuotaCacheKey(account)

	// 从 CacheService 获取缓存（已内置深拷贝防止 TOCTOU）
	if cachedData, found := globalCacheService.GetQuotaCache(cacheKey); found {
		return cachedData, nil
	}

	// 调用直接获取函数
	quotaData, err := getQuotaDataDirect(account, httpClient)
	if err != nil {
		return nil, err
	}

	// 更新缓存（使用 CacheService）
	globalCacheService.SetQuotaCache(cacheKey, quotaData, QuotaCacheTime)

	return quotaData, nil
}

// getQuotaDataDirect 直接从 JetBrains API 获取配额数据（不使用全局缓存）
func getQuotaDataDirect(account *JetbrainsAccount, httpClient *http.Client) (*JetbrainsQuotaResponse, error) {
	if err := ensureValidJWT(account, httpClient); err != nil {
		return nil, fmt.Errorf("failed to refresh JWT: %w", err)
	}

	if account.JWT == "" {
		return nil, fmt.Errorf("account has no JWT")
	}

	req, err := http.NewRequest(http.MethodPost, JetBrainsQuotaEndpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Length", "0")
	setJetbrainsHeaders(req, account.JWT)

	resp, err := handleJWTExpiredAndRetry(req, account, httpClient)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		// 如果是401，则JWT可能已失效，从缓存中删除
		if resp.StatusCode == HTTPStatusUnauthorized {
			cacheKey := generateQuotaCacheKey(account)
			globalCacheService.DeleteQuotaCache(cacheKey)
		}
		return nil, fmt.Errorf("quota check failed with status %d: %s", resp.StatusCode, string(body))
	}

	var quotaData JetbrainsQuotaResponse
	if err := sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&quotaData); err != nil {
		return nil, err
	}

	if IsDebug() {
		quotaJSON, _ := sonic.MarshalIndent(quotaData, "", "  ")
		Debug("JetBrains Quota API Response: %s", string(quotaJSON))
	}

	processQuotaData(&quotaData, account)

	return &quotaData, nil
}
