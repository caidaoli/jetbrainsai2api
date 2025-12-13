package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// PooledAccountManager 基于池的账户管理器实现
// SRP: 单一职责 - 只负责账户的获取、释放和生命周期管理
type PooledAccountManager struct {
	accounts   []JetbrainsAccount
	pool       chan *JetbrainsAccount
	mu         sync.RWMutex
	httpClient *http.Client

	// 依赖注入
	cache   *CacheService
	logger  Logger
	metrics MetricsCollector
}

// AccountManagerConfig 账户管理器配置
type AccountManagerConfig struct {
	Accounts   []JetbrainsAccount
	HTTPClient *http.Client
	Cache      *CacheService
	Logger     Logger
	Metrics    MetricsCollector
}

// NewPooledAccountManager 创建新的账户管理器
func NewPooledAccountManager(config AccountManagerConfig) (*PooledAccountManager, error) {
	if len(config.Accounts) == 0 {
		return nil, fmt.Errorf("no accounts provided")
	}

	// 使用默认值处理可选依赖
	logger := config.Logger
	if logger == nil {
		logger = &NopLogger{}
	}

	metrics := config.Metrics
	if metrics == nil {
		metrics = &NopMetrics{}
	}

	am := &PooledAccountManager{
		accounts:   config.Accounts, // 直接使用传入的账户切片，避免复制 mutex
		pool:       make(chan *JetbrainsAccount, len(config.Accounts)),
		httpClient: config.HTTPClient,
		cache:      config.Cache,
		logger:     logger,
		metrics:    metrics,
	}

	// 初始化账户池（使用指针避免复制 mutex）
	for i := range am.accounts {
		am.pool <- &am.accounts[i]
	}

	am.logger.Info("Account manager initialized with %d accounts", len(config.Accounts))
	return am, nil
}

// AcquireAccount 获取一个可用账户
// 支持 context 取消和超时
func (am *PooledAccountManager) AcquireAccount(ctx context.Context) (*JetbrainsAccount, error) {
	accountWaitStart := time.Now()
	triedAccounts := make(map[*JetbrainsAccount]bool)
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
		// account == nil 表示需要继续尝试
	}

	am.metrics.RecordAccountPoolError()
	return nil, fmt.Errorf("failed to acquire account after trying %d accounts: all accounts unavailable", len(triedAccounts))
}

// tryAcquireOnce 尝试获取一个账户（单次尝试）
// 返回值：(account, nil) 成功，(nil, error) 失败且应终止，(nil, nil) 需要重试
func (am *PooledAccountManager) tryAcquireOnce(
	ctx context.Context,
	triedAccounts map[*JetbrainsAccount]bool,
	totalAccounts int,
	waitStart time.Time,
) (*JetbrainsAccount, error) {
	select {
	case <-ctx.Done():
		am.metrics.RecordAccountPoolError()
		return nil, fmt.Errorf("request cancelled while waiting for account: %w", ctx.Err())

	case account := <-am.pool:
		return am.processAccount(account, triedAccounts, totalAccounts, waitStart)

	case <-time.After(AccountAcquireTimeout):
		am.metrics.RecordAccountPoolError()
		return nil, fmt.Errorf("timed out waiting for an available JetBrains account")
	}
}

// processAccount 处理获取到的账户，检查 JWT 和配额
func (am *PooledAccountManager) processAccount(
	account *JetbrainsAccount,
	triedAccounts map[*JetbrainsAccount]bool,
	totalAccounts int,
	waitStart time.Time,
) (*JetbrainsAccount, error) {
	// 检查是否已尝试过此账户
	if triedAccounts[account] {
		am.ReleaseAccount(account)
		if len(triedAccounts) >= totalAccounts {
			am.metrics.RecordAccountPoolError()
			return nil, fmt.Errorf("all %d accounts have been tried and failed", totalAccounts)
		}
		return nil, nil // 需要重试
	}

	// 记录等待时间（仅首次获取时）
	if len(triedAccounts) == 0 {
		if waitDuration := time.Since(waitStart); waitDuration > 100*time.Millisecond {
			am.metrics.RecordAccountPoolWait(waitDuration)
		}
	}

	triedAccounts[account] = true

	// 验证账户可用性
	if err := am.ensureAccountReady(account, triedAccounts, totalAccounts); err != nil {
		am.ReleaseAccount(account)
		return nil, nil // 需要重试
	}

	return account, nil
}

// ensureAccountReady 确保账户的 JWT 和配额有效
func (am *PooledAccountManager) ensureAccountReady(
	account *JetbrainsAccount,
	triedAccounts map[*JetbrainsAccount]bool,
	totalAccounts int,
) error {
	// 检查并刷新 JWT（如果需要）
	if account.LicenseID != "" {
		if account.JWT == "" || time.Now().After(account.ExpiryTime.Add(-JWTRefreshTime)) {
			if err := am.refreshJWTInternal(account); err != nil {
				am.logger.Error("Failed to refresh JWT for %s (tried %d/%d accounts): %v",
					getTokenDisplayName(account), len(triedAccounts), totalAccounts, err)
				am.metrics.RecordAccountPoolError()
				return err
			}
		}
	}

	// 检查配额
	if err := am.checkQuotaInternal(account); err != nil {
		am.logger.Error("Failed to check quota for %s (tried %d/%d accounts): %v",
			getTokenDisplayName(account), len(triedAccounts), totalAccounts, err)
		am.metrics.RecordAccountPoolError()
		return err
	}

	if !account.HasQuota {
		am.logger.Warn("Account %s is over quota (tried %d/%d accounts)",
			getTokenDisplayName(account), len(triedAccounts), totalAccounts)
		return fmt.Errorf("account over quota")
	}

	return nil
}

// ReleaseAccount 释放账户回池
// 非阻塞设计，如果池满则警告
func (am *PooledAccountManager) ReleaseAccount(account *JetbrainsAccount) {
	if account == nil {
		return
	}

	select {
	case am.pool <- account:
		// 成功归还
	default:
		// 池满了（不应该发生）
		am.logger.Warn("account pool is full, could not return account %s", getTokenDisplayName(account))
	}
}

// GetAccountCount 获取账户总数
func (am *PooledAccountManager) GetAccountCount() int {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return len(am.accounts)
}

// GetAvailableCount 获取当前可用账户数
func (am *PooledAccountManager) GetAvailableCount() int {
	return len(am.pool)
}

// RefreshJWT 刷新账户的 JWT token
// 使用账户级互斥锁防止并发刷新同一账户
func (am *PooledAccountManager) RefreshJWT(account *JetbrainsAccount) error {
	account.mu.Lock()
	defer account.mu.Unlock()

	// 双重检查：可能已经被其他 goroutine 刷新了
	if account.JWT != "" && time.Now().Before(account.ExpiryTime.Add(-JWTRefreshTime)) {
		return nil // 已经是新的了
	}

	return refreshJetbrainsJWT(account, am.httpClient)
}

// refreshJWTInternal 内部 JWT 刷新实现（使用账户级锁）
func (am *PooledAccountManager) refreshJWTInternal(account *JetbrainsAccount) error {
	account.mu.Lock()
	defer account.mu.Unlock()

	// 双重检查：可能已经被其他 goroutine 刷新了
	if account.JWT != "" && time.Now().Before(account.ExpiryTime.Add(-JWTRefreshTime)) {
		return nil // 已经是新的了
	}

	return refreshJetbrainsJWT(account, am.httpClient)
}

// CheckQuota 检查账户配额
func (am *PooledAccountManager) CheckQuota(account *JetbrainsAccount) error {
	return am.checkQuotaInternal(account)
}

// checkQuotaInternal 内部配额检查实现（使用账户级锁保护状态修改）
func (am *PooledAccountManager) checkQuotaInternal(account *JetbrainsAccount) error {
	account.mu.Lock()
	defer account.mu.Unlock()

	// 如果最近检查过配额（1小时内），跳过检查
	if account.LastQuotaCheck > 0 {
		lastCheck := time.Unix(int64(account.LastQuotaCheck), 0)
		if time.Since(lastCheck) < QuotaCacheTime {
			// 配额检查仍然有效，跳过
			return nil
		}
	}

	// 使用注入的 cache 获取配额数据
	quotaData, err := getQuotaData(account, am.httpClient, am.cache)
	if err != nil {
		account.HasQuota = false
		return err
	}

	processQuotaData(quotaData, account)
	return nil
}

// GetAllAccounts 获取所有账户信息（只读）
// 用于统计页面显示
func (am *PooledAccountManager) GetAllAccounts() []JetbrainsAccount {
	am.mu.RLock()
	defer am.mu.RUnlock()

	// 手动复制账户信息，跳过 mutex 字段（使用索引避免复制）
	accounts := make([]JetbrainsAccount, len(am.accounts))
	for i := range am.accounts {
		accounts[i] = JetbrainsAccount{
			LicenseID:      am.accounts[i].LicenseID,
			Authorization:  am.accounts[i].Authorization,
			JWT:            am.accounts[i].JWT,
			LastUpdated:    am.accounts[i].LastUpdated,
			HasQuota:       am.accounts[i].HasQuota,
			LastQuotaCheck: am.accounts[i].LastQuotaCheck,
			ExpiryTime:     am.accounts[i].ExpiryTime,
			// mu 字段不复制，使用零值
		}
	}
	return accounts
}

// Close 关闭账户管理器
// 目前是空操作，但预留接口供将来扩展
func (am *PooledAccountManager) Close() error {
	am.logger.Info("Account manager shutting down")
	return nil
}
