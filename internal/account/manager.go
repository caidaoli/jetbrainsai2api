package account

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"jetbrainsai2api/internal/cache"
	"jetbrainsai2api/internal/core"
	"jetbrainsai2api/internal/util"
)

// PooledAccountManager pool-based account manager implementation
type PooledAccountManager struct {
	accounts   []core.JetbrainsAccount
	pool       chan *core.JetbrainsAccount
	mu         sync.RWMutex
	httpClient *http.Client

	cache   *cache.CacheService
	logger  core.Logger
	metrics core.MetricsCollector
}

// AccountManagerConfig account manager configuration
type AccountManagerConfig struct {
	Accounts   []core.JetbrainsAccount
	HTTPClient *http.Client
	Cache      *cache.CacheService
	Logger     core.Logger
	Metrics    core.MetricsCollector
}

// NewPooledAccountManager creates a new account manager
func NewPooledAccountManager(config AccountManagerConfig) (*PooledAccountManager, error) {
	if len(config.Accounts) == 0 {
		return nil, fmt.Errorf("no accounts provided")
	}

	logger := config.Logger
	if logger == nil {
		logger = &core.NopLogger{}
	}

	metrics := config.Metrics
	if metrics == nil {
		metrics = &core.NopMetrics{}
	}

	am := &PooledAccountManager{
		accounts:   config.Accounts,
		pool:       make(chan *core.JetbrainsAccount, len(config.Accounts)),
		httpClient: config.HTTPClient,
		cache:      config.Cache,
		logger:     logger,
		metrics:    metrics,
	}

	for i := range am.accounts {
		am.pool <- &am.accounts[i]
	}

	am.logger.Info("Account manager initialized with %d accounts", len(config.Accounts))
	return am, nil
}

// AcquireAccount gets an available account
func (am *PooledAccountManager) AcquireAccount(ctx context.Context) (*core.JetbrainsAccount, error) {
	accountWaitStart := time.Now()
	triedAccounts := make(map[*core.JetbrainsAccount]bool)
	totalAccounts := am.GetAccountCount()
	maxAttempts := totalAccounts * 2

	for attempts := 0; attempts < maxAttempts && len(triedAccounts) < totalAccounts; attempts++ {
		account, err := am.tryAcquireOnce(ctx, triedAccounts, totalAccounts, accountWaitStart)
		if err != nil {
			return nil, err
		}
		if account != nil {
			return account, nil
		}
	}

	am.metrics.RecordAccountPoolError()
	return nil, fmt.Errorf("failed to acquire account after trying %d accounts: all accounts unavailable", len(triedAccounts))
}

func (am *PooledAccountManager) tryAcquireOnce(
	ctx context.Context,
	triedAccounts map[*core.JetbrainsAccount]bool,
	totalAccounts int,
	waitStart time.Time,
) (*core.JetbrainsAccount, error) {
	select {
	case <-ctx.Done():
		am.metrics.RecordAccountPoolError()
		return nil, fmt.Errorf("request cancelled while waiting for account: %w", ctx.Err())

	case account := <-am.pool:
		return am.processAccount(account, triedAccounts, totalAccounts, waitStart)

	case <-time.After(core.AccountAcquireTimeout):
		am.metrics.RecordAccountPoolError()
		return nil, fmt.Errorf("timed out waiting for an available JetBrains account")
	}
}

func (am *PooledAccountManager) processAccount(
	account *core.JetbrainsAccount,
	triedAccounts map[*core.JetbrainsAccount]bool,
	totalAccounts int,
	waitStart time.Time,
) (*core.JetbrainsAccount, error) {
	if triedAccounts[account] {
		am.ReleaseAccount(account)
		if len(triedAccounts) >= totalAccounts {
			am.metrics.RecordAccountPoolError()
			return nil, fmt.Errorf("all %d accounts have been tried and failed", totalAccounts)
		}
		return nil, nil
	}

	if len(triedAccounts) == 0 {
		if waitDuration := time.Since(waitStart); waitDuration > 100*time.Millisecond {
			am.metrics.RecordAccountPoolWait(waitDuration)
		}
	}

	triedAccounts[account] = true

	if err := am.ensureAccountReady(account, triedAccounts, totalAccounts); err != nil {
		am.ReleaseAccount(account)
		return nil, nil
	}

	return account, nil
}

func (am *PooledAccountManager) ensureAccountReady(
	account *core.JetbrainsAccount,
	triedAccounts map[*core.JetbrainsAccount]bool,
	totalAccounts int,
) error {
	account.Mu.Lock()
	jwt := account.JWT
	expiryTime := account.ExpiryTime
	licenseID := account.LicenseID
	account.Mu.Unlock()
	if licenseID != "" {
		if jwt == "" || time.Now().After(expiryTime.Add(-core.JWTRefreshTime)) {
			if err := am.RefreshJWT(account); err != nil {
				am.logger.Error("Failed to refresh JWT for %s (tried %d/%d accounts): %v",
					util.GetTokenDisplayName(account), len(triedAccounts), totalAccounts, err)
				am.metrics.RecordAccountPoolError()
				return err
			}
		}
	}

	if err := am.checkQuotaInternal(account); err != nil {
		am.logger.Error("Failed to check quota for %s (tried %d/%d accounts): %v",
			util.GetTokenDisplayName(account), len(triedAccounts), totalAccounts, err)
		am.metrics.RecordAccountPoolError()
		return err
	}

	account.Mu.Lock()
	hasQuota := account.HasQuota
	account.Mu.Unlock()
	if !hasQuota {
		am.logger.Warn("Account %s is over quota (tried %d/%d accounts)",
			util.GetTokenDisplayName(account), len(triedAccounts), totalAccounts)
		return fmt.Errorf("account over quota")
	}

	return nil
}

// ReleaseAccount releases account back to pool
func (am *PooledAccountManager) ReleaseAccount(account *core.JetbrainsAccount) {
	if account == nil {
		return
	}

	select {
	case am.pool <- account:
	default:
		am.logger.Warn("account pool is full, could not return account %s", util.GetTokenDisplayName(account))
	}
}

// GetAccountCount gets total account count
func (am *PooledAccountManager) GetAccountCount() int {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return len(am.accounts)
}

// GetAvailableCount gets available account count
func (am *PooledAccountManager) GetAvailableCount() int {
	return len(am.pool)
}

// RefreshJWT refreshes account JWT token
func (am *PooledAccountManager) RefreshJWT(account *core.JetbrainsAccount) error {
	account.Mu.Lock()
	defer account.Mu.Unlock()

	if account.JWT != "" && time.Now().Before(account.ExpiryTime.Add(-core.JWTRefreshTime)) {
		return nil
	}

	return RefreshJetbrainsJWT(account, am.httpClient, am.logger)
}

// CheckQuota checks account quota
func (am *PooledAccountManager) CheckQuota(account *core.JetbrainsAccount) error {
	return am.checkQuotaInternal(account)
}

func (am *PooledAccountManager) checkQuotaInternal(account *core.JetbrainsAccount) error {
	account.Mu.Lock()
	lastQuotaCheck := account.LastQuotaCheck
	account.Mu.Unlock()

	if lastQuotaCheck > 0 {
		lastCheck := time.Unix(int64(lastQuotaCheck), 0)
		if time.Since(lastCheck) < core.QuotaCacheTime {
			return nil
		}
	}

	if _, err := GetQuotaData(account, am.httpClient, am.cache, am.logger); err != nil {
		return err
	}

	return nil
}

// GetAllAccounts gets all account info (read-only)
func (am *PooledAccountManager) GetAllAccounts() []core.JetbrainsAccount {
	am.mu.RLock()
	defer am.mu.RUnlock()

	accounts := make([]core.JetbrainsAccount, len(am.accounts))
	for i := range am.accounts {
		account := &am.accounts[i]
		account.Mu.Lock()
		accounts[i] = core.JetbrainsAccount{
			LicenseID:      account.LicenseID,
			Authorization:  account.Authorization,
			JWT:            account.JWT,
			LastUpdated:    account.LastUpdated,
			HasQuota:       account.HasQuota,
			LastQuotaCheck: account.LastQuotaCheck,
			ExpiryTime:     account.ExpiryTime,
		}
		account.Mu.Unlock()
	}
	return accounts
}

// Close shuts down account manager
func (am *PooledAccountManager) Close() error {
	am.logger.Info("Account manager shutting down")
	return nil
}
