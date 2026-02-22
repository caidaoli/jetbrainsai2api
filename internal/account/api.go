package account

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/log"
	"jetbrainsai2api/internal/util"

	"github.com/bytedance/sonic"
)

// SetAccountQuotaStatus thread-safe update of account quota status
func SetAccountQuotaStatus(account *core.JetbrainsAccount, hasQuota bool, checkedAt time.Time) {
	if account == nil {
		return
	}

	account.Lock()
	account.HasQuota = hasQuota
	account.LastQuotaCheck = float64(checkedAt.Unix())
	account.Unlock()
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
	if err := util.ValidateJetBrainsRequestTarget(req, "outbound"); err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req) //nolint:gosec // Request target is restricted by util.ValidateJetBrainsRequestTarget.
	if err != nil {
		return nil, err
	}

	account.Lock()
	licenseID := account.LicenseID
	account.Unlock()

	if resp.StatusCode == core.HTTPStatusUnauthorized && licenseID != "" {
		_ = resp.Body.Close()
		logger.Info("JWT for %s expired, refreshing...", util.GetTokenDisplayName(account))

		if err := RefreshJetbrainsJWT(account, httpClient, logger); err != nil {
			return nil, err
		}

		account.Lock()
		jwt := account.JWT
		account.Unlock()
		req.Header.Set(core.HeaderGrazieAuthJWT, jwt)
		if err := util.ValidateJetBrainsRequestTarget(req, "outbound"); err != nil {
			return nil, err
		}
		return httpClient.Do(req) //nolint:gosec // Request target is restricted by util.ValidateJetBrainsRequestTarget.
	}

	return resp, nil
}

// EnsureValidJWT ensures account has a valid JWT, refreshing if empty or expired
func EnsureValidJWT(account *core.JetbrainsAccount, httpClient *http.Client, logger core.Logger) error {
	account.Lock()
	licenseID := account.LicenseID
	needsRefresh := account.JWT == "" || (!account.ExpiryTime.IsZero() && time.Now().After(account.ExpiryTime))
	account.Unlock()

	if licenseID == "" {
		return nil
	}
	if needsRefresh {
		return RefreshJetbrainsJWT(account, httpClient, logger)
	}
	return nil
}

// ParseJWTExpiry parses JWT expiry time
func ParseJWTExpiry(tokenStr string) (time.Time, error) {
	return util.ParseJWTExpiry(tokenStr)
}

// RefreshJetbrainsJWT refreshes JWT for a JetBrains account
func RefreshJetbrainsJWT(account *core.JetbrainsAccount, httpClient *http.Client, logger core.Logger) error {
	account.Lock()
	licenseID := account.LicenseID
	authorization := account.Authorization
	account.Unlock()

	logger.Info("Refreshing JWT for licenseId %s...", licenseID)

	payload := map[string]string{"licenseId": licenseID}
	req, err := util.CreateJetbrainsRequest(http.MethodPost, core.JetBrainsJWTEndpoint, payload, authorization)
	if err != nil {
		return err
	}
	SetJetbrainsHeaders(req, "")

	if err := util.ValidateJetBrainsRequestTarget(req, "outbound"); err != nil {
		return err
	}

	resp, err := httpClient.Do(req) //nolint:gosec // Request target is restricted by util.ValidateJetBrainsRequestTarget.
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
		var expiryTime time.Time
		expiryTime, err := util.ParseJWTExpiry(tokenStr)
		if err != nil {
			logger.Warn("could not parse JWT: %v", err)
		}

		account.Lock()
		account.JWT = tokenStr
		account.LastUpdated = float64(time.Now().Unix())
		account.ExpiryTime = expiryTime
		account.Unlock()

		logger.Info("Successfully refreshed JWT for licenseId %s, expires at %s", licenseID, expiryTime.Format(time.RFC3339))
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

// GetQuotaData gets quota data (using QuotaCache interface)
func GetQuotaData(account *core.JetbrainsAccount, httpClient *http.Client, quotaCache core.QuotaCache, logger core.Logger) (*core.JetbrainsQuotaResponse, error) {
	if err := EnsureValidJWT(account, httpClient, logger); err != nil {
		return nil, fmt.Errorf("failed to refresh JWT: %w", err)
	}

	account.Lock()
	jwt := account.JWT
	licenseID := account.LicenseID
	account.Unlock()

	if jwt == "" {
		return nil, fmt.Errorf("account has no JWT")
	}

	if quotaCache != nil {
		cacheKey := quotaCache.GenerateQuotaCacheKey(jwt, licenseID)
		if cachedData, found := quotaCache.GetQuotaCache(cacheKey); found {
			return cachedData, nil
		}
	}

	quotaData, err := getQuotaDataDirect(account, httpClient, quotaCache, logger)
	if err != nil {
		return nil, err
	}

	if quotaCache != nil {
		cacheKey := quotaCache.GenerateQuotaCacheKey(jwt, licenseID)
		quotaCache.SetQuotaCache(cacheKey, quotaData)
	}

	return quotaData, nil
}

func getQuotaDataDirect(account *core.JetbrainsAccount, httpClient *http.Client, quotaCache core.QuotaCache, logger core.Logger) (*core.JetbrainsQuotaResponse, error) {
	account.Lock()
	jwt := account.JWT
	licenseID := account.LicenseID
	account.Unlock()

	req, err := http.NewRequest(http.MethodPost, core.JetBrainsQuotaEndpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Length", "0")
	SetJetbrainsHeaders(req, jwt)

	resp, err := HandleJWTExpiredAndRetry(req, account, httpClient, logger)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, core.MaxResponseBodySize))
		if resp.StatusCode == core.HTTPStatusUnauthorized && quotaCache != nil {
			cacheKey := quotaCache.GenerateQuotaCacheKey(jwt, licenseID)
			quotaCache.DeleteQuotaCache(cacheKey)
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
