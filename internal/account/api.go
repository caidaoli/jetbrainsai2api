package account

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"jetbrainsai2api/internal/cache"
	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/log"
	"jetbrainsai2api/internal/util"

	"github.com/bytedance/sonic"
	"github.com/golang-jwt/jwt/v5"
)

// SetAccountQuotaStatus thread-safe update of account quota status
func SetAccountQuotaStatus(account *core.JetbrainsAccount, hasQuota bool, checkedAt time.Time) {
	if account == nil {
		return
	}

	account.Mu.Lock()
	account.HasQuota = hasQuota
	account.LastQuotaCheck = float64(checkedAt.Unix())
	account.Mu.Unlock()
}

// MarkAccountNoQuota marks account as having no quota
func MarkAccountNoQuota(account *core.JetbrainsAccount) {
	SetAccountQuotaStatus(account, false, time.Now())
}

// SetJetbrainsHeaders sets the required headers for JetBrains API requests
func SetJetbrainsHeaders(req *http.Request, jwtToken string) {
	req.Header.Set("User-Agent", core.JetBrainsHeaderUserAgent)
	req.Header.Set(core.HeaderAcceptCharset, core.CharsetUTF8)
	req.Header.Set(core.HeaderGrazieAgent, core.JetBrainsHeaderGrazieAgent)
	if jwtToken != "" {
		req.Header.Set(core.HeaderGrazieAuthJWT, jwtToken)
	}
}

// HandleJWTExpiredAndRetry handles JWT expiration and retries the request
func HandleJWTExpiredAndRetry(req *http.Request, account *core.JetbrainsAccount, httpClient *http.Client, logger core.Logger) (*http.Response, error) {
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == core.HTTPStatusUnauthorized && account.LicenseID != "" {
		_ = resp.Body.Close()
		logger.Info("JWT for %s expired, refreshing...", util.GetTokenDisplayName(account))

		if err := RefreshJetbrainsJWT(account, httpClient, logger); err != nil {
			return nil, err
		}

		req.Header.Set(core.HeaderGrazieAuthJWT, account.JWT)
		return httpClient.Do(req)
	}

	return resp, nil
}

// EnsureValidJWT ensures account has a valid JWT
func EnsureValidJWT(account *core.JetbrainsAccount, httpClient *http.Client, logger core.Logger) error {
	if account.JWT == "" && account.LicenseID != "" {
		return RefreshJetbrainsJWT(account, httpClient, logger)
	}
	return nil
}

// ParseJWTExpiry parses JWT expiry time
func ParseJWTExpiry(tokenStr string) (time.Time, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("invalid JWT format: expected 3 parts, got %d", len(parts))
	}

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

// RefreshJetbrainsJWT refreshes JWT for a JetBrains account
func RefreshJetbrainsJWT(account *core.JetbrainsAccount, httpClient *http.Client, logger core.Logger) error {
	logger.Info("Refreshing JWT for licenseId %s...", account.LicenseID)

	payload := map[string]string{"licenseId": account.LicenseID}
	req, err := util.CreateJetbrainsRequest(http.MethodPost, core.JetBrainsJWTEndpoint, payload, account.Authorization)
	if err != nil {
		return err
	}
	SetJetbrainsHeaders(req, "")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, core.MaxResponseBodySize))
		return fmt.Errorf("JWT refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var data map[string]any
	if err := sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}

	state, _ := data["state"].(string)
	tokenStr, _ := data["token"].(string)

	if state == "PAID" && tokenStr != "" {
		parts := strings.Split(tokenStr, ".")
		if len(parts) != 3 {
			return fmt.Errorf("invalid JWT format: expected 3 parts, got %d", len(parts))
		}

		account.JWT = tokenStr
		account.LastUpdated = float64(time.Now().Unix())

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

// ProcessQuotaData processes quota data and updates account status
func ProcessQuotaData(quotaData *core.JetbrainsQuotaResponse, account *core.JetbrainsAccount, logger core.Logger) {
	dailyUsed, _ := strconv.ParseFloat(quotaData.Current.Current.Amount, 64)
	dailyTotal, _ := strconv.ParseFloat(quotaData.Current.Maximum.Amount, 64)

	if dailyTotal == 0 {
		dailyTotal = 1
	}

	hasQuota := dailyUsed < dailyTotal
	SetAccountQuotaStatus(account, hasQuota, time.Now())

	if !hasQuota {
		logger.Warn("Account %s has no quota", util.GetTokenDisplayName(account))
	}
}

// GetQuotaData gets quota data (using CacheService)
func GetQuotaData(account *core.JetbrainsAccount, httpClient *http.Client, cacheService *cache.CacheService, logger core.Logger) (*core.JetbrainsQuotaResponse, error) {
	if err := EnsureValidJWT(account, httpClient, logger); err != nil {
		return nil, fmt.Errorf("failed to refresh JWT: %w", err)
	}

	if account.JWT == "" {
		return nil, fmt.Errorf("account has no JWT")
	}

	cacheKey := cache.GenerateQuotaCacheKey(account)

	if cacheService != nil {
		if cachedData, found := cacheService.GetQuotaCache(cacheKey); found {
			return cachedData, nil
		}
	}

	quotaData, err := getQuotaDataDirect(account, httpClient, cacheService, logger)
	if err != nil {
		return nil, err
	}

	if cacheService != nil {
		cacheService.SetQuotaCache(cacheKey, quotaData, core.QuotaCacheTime)
	}

	return quotaData, nil
}

func getQuotaDataDirect(account *core.JetbrainsAccount, httpClient *http.Client, cacheService *cache.CacheService, logger core.Logger) (*core.JetbrainsQuotaResponse, error) {
	req, err := http.NewRequest(http.MethodPost, core.JetBrainsQuotaEndpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Length", "0")
	SetJetbrainsHeaders(req, account.JWT)

	resp, err := HandleJWTExpiredAndRetry(req, account, httpClient, logger)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, core.MaxResponseBodySize))
		if resp.StatusCode == core.HTTPStatusUnauthorized && cacheService != nil {
			cacheKey := cache.GenerateQuotaCacheKey(account)
			cacheService.DeleteQuotaCache(cacheKey)
		}
		return nil, fmt.Errorf("quota check failed with status %d: %s", resp.StatusCode, string(body))
	}

	var quotaData core.JetbrainsQuotaResponse
	if err := sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&quotaData); err != nil {
		return nil, err
	}

	if log.IsDebug() {
		quotaJSON, _ := sonic.MarshalIndent(quotaData, "", "  ")
		logger.Debug("JetBrains Quota API Response: %s", string(quotaJSON))
	}

	ProcessQuotaData(&quotaData, account, logger)

	return &quotaData, nil
}
