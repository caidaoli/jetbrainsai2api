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
	jwtRefresh sync.Mutex
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
		logger = &nopLogger{}
	}

	metrics := config.Metrics
	if metrics == nil {
		metrics = &nopMetrics{}
	}

	am := &PooledAccountManager{
		accounts:   make([]JetbrainsAccount, len(config.Accounts)),
		pool:       make(chan *JetbrainsAccount, len(config.Accounts)),
		httpClient: config.HTTPClient,
		cache:      config.Cache,
		logger:     logger,
		metrics:    metrics,
	}

	// 复制账户配置
	copy(am.accounts, config.Accounts)

	// 初始化账户池
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

	// 最多重试3次，避免无限循环
	const maxRetries = AccountAcquireMaxRetries
	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			am.metrics.RecordAccountPoolError()
			return nil, fmt.Errorf("request cancelled while waiting for account: %w", ctx.Err())

		case account := <-am.pool:
			// 记录等待时间
			if attempt == 0 {
				waitDuration := time.Since(accountWaitStart)
				if waitDuration > 100*time.Millisecond {
					am.metrics.RecordAccountPoolWait(waitDuration)
				}
			}

			// 检查并刷新 JWT（如果需要）
			if account.LicenseID != "" {
				if account.JWT == "" || time.Now().After(account.ExpiryTime.Add(-JWTRefreshTime)) {
					if err := am.refreshJWTInternal(account); err != nil {
						am.logger.Error("Failed to refresh JWT for %s (attempt %d/%d): %v", getTokenDisplayName(account), attempt+1, maxRetries, err)
						am.metrics.RecordAccountPoolError()
						// JWT刷新失败，放回池中，重试下一个账户
						am.ReleaseAccount(account)
						continue
					}
				}
			}

			// 检查配额
			if err := am.checkQuotaInternal(account); err != nil {
				am.logger.Error("Failed to check quota for %s (attempt %d/%d): %v", getTokenDisplayName(account), attempt+1, maxRetries, err)
				am.metrics.RecordAccountPoolError()
				// 配额检查失败，放回池中，重试下一个账户
				am.ReleaseAccount(account)
				continue
			}

			if !account.HasQuota {
				am.logger.Warn("Account %s is over quota (attempt %d/%d)", getTokenDisplayName(account), attempt+1, maxRetries)
				// 配额不足，放回池中，重试下一个账户
				am.ReleaseAccount(account)
				continue
			}

			// 成功获取账户
			return account, nil

		case <-time.After(AccountAcquireTimeout):
			am.metrics.RecordAccountPoolError()
			return nil, fmt.Errorf("timed out waiting for an available JetBrains account")
		}
	}

	// 所有重试都失败
	am.metrics.RecordAccountPoolError()
	return nil, fmt.Errorf("failed to acquire account after %d attempts: all accounts unavailable", maxRetries)
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
// 使用互斥锁防止并发刷新同一账户
func (am *PooledAccountManager) RefreshJWT(account *JetbrainsAccount) error {
	am.jwtRefresh.Lock()
	defer am.jwtRefresh.Unlock()

	// 双重检查：可能已经被其他 goroutine 刷新了
	if account.JWT != "" && time.Now().Before(account.ExpiryTime.Add(-JWTRefreshTime)) {
		return nil // 已经是新的了
	}

	return am.refreshJWTInternal(account)
}

// refreshJWTInternal 内部 JWT 刷新实现（不加锁）
func (am *PooledAccountManager) refreshJWTInternal(account *JetbrainsAccount) error {
	return refreshJetbrainsJWT(account, am.httpClient)
}

// CheckQuota 检查账户配额
func (am *PooledAccountManager) CheckQuota(account *JetbrainsAccount) error {
	return am.checkQuotaInternal(account)
}

// checkQuotaInternal 内部配额检查实现
func (am *PooledAccountManager) checkQuotaInternal(account *JetbrainsAccount) error {
	// 如果最近检查过配额（1小时内），跳过检查
	if account.LastQuotaCheck > 0 {
		lastCheck := time.Unix(int64(account.LastQuotaCheck), 0)
		if time.Since(lastCheck) < QuotaCacheTime {
			// 配额检查仍然有效，跳过
			return nil
		}
	}

	// 使用注入的 cache，如果没有则使用直接调用
	var quotaData *JetbrainsQuotaResponse
	var err error

	if am.cache != nil {
		// 使用 CacheService 获取配额数据（已修复 TOCTOU）
		cacheKey := generateQuotaCacheKey(account)
		cached, found := am.cache.GetQuotaCache(cacheKey)
		if found {
			quotaData = cached
		} else {
			quotaData, err = getQuotaDataDirect(account, am.httpClient)
			if err != nil {
				account.HasQuota = false
				return err
			}
			// 缓存结果
			am.cache.SetQuotaCache(cacheKey, quotaData, QuotaCacheTime)
		}
	} else {
		// 回退到直接调用（向后兼容）
		quotaData, err = getQuotaData(account, am.httpClient)
		if err != nil {
			account.HasQuota = false
			return err
		}
	}

	processQuotaData(quotaData, account)
	return nil
}

// GetAllAccounts 获取所有账户信息（只读）
// 用于统计页面显示
func (am *PooledAccountManager) GetAllAccounts() []JetbrainsAccount {
	am.mu.RLock()
	defer am.mu.RUnlock()

	accounts := make([]JetbrainsAccount, len(am.accounts))
	copy(accounts, am.accounts)
	return accounts
}

// Close 关闭账户管理器
// 目前是空操作，但预留接口供将来扩展
func (am *PooledAccountManager) Close() error {
	am.logger.Info("Account manager shutting down")
	return nil
}

// nopLogger 默认日志实现（空操作）
type nopLogger struct{}

func (l *nopLogger) Debug(format string, args ...any) {}
func (l *nopLogger) Info(format string, args ...any)  {}
func (l *nopLogger) Warn(format string, args ...any)  {}
func (l *nopLogger) Error(format string, args ...any) {}
func (l *nopLogger) Fatal(format string, args ...any) {}

// nopMetrics 默认指标收集器（空操作）
type nopMetrics struct{}

func (m *nopMetrics) RecordHTTPRequest(duration time.Duration)     {}
func (m *nopMetrics) RecordHTTPError()                             {}
func (m *nopMetrics) RecordCacheHit()                              {}
func (m *nopMetrics) RecordCacheMiss()                             {}
func (m *nopMetrics) RecordToolValidation(duration time.Duration)  {}
func (m *nopMetrics) RecordAccountPoolWait(duration time.Duration) {}
func (m *nopMetrics) RecordAccountPoolError()                      {}
func (m *nopMetrics) UpdateSystemMetrics()                         {}
func (m *nopMetrics) ResetWindow()                                 {}
func (m *nopMetrics) GetQPS() float64                              { return 0 }
func (m *nopMetrics) GetMetricsString() string                     { return "" }
