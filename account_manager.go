package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// AccountManager 账户管理器接口
// DIP: 依赖倒置原则 - 依赖抽象而非具体实现
type AccountManager interface {
	// AcquireAccount 获取一个可用账户
	AcquireAccount(ctx context.Context) (*JetbrainsAccount, error)

	// ReleaseAccount 释放账户回池
	ReleaseAccount(account *JetbrainsAccount)

	// GetAccountCount 获取账户总数
	GetAccountCount() int

	// GetAvailableCount 获取可用账户数
	GetAvailableCount() int

	// RefreshJWT 刷新账户 JWT
	RefreshJWT(account *JetbrainsAccount) error

	// CheckQuota 检查账户配额
	CheckQuota(account *JetbrainsAccount) error

	// GetAllAccounts 获取所有账户信息
	GetAllAccounts() []JetbrainsAccount

	// Close 关闭账户管理器
	Close() error
}

// PooledAccountManager 基于池的账户管理器实现
// SRP: 单一职责 - 只负责账户的获取、释放和生命周期管理
type PooledAccountManager struct {
	accounts   []JetbrainsAccount
	pool       chan *JetbrainsAccount
	mu         sync.RWMutex
	jwtRefresh sync.Mutex
	httpClient *http.Client
}

// NewPooledAccountManager 创建新的账户管理器
func NewPooledAccountManager(configs []JetbrainsAccount, httpClient *http.Client) (*PooledAccountManager, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("no accounts provided")
	}

	am := &PooledAccountManager{
		accounts:   make([]JetbrainsAccount, len(configs)),
		pool:       make(chan *JetbrainsAccount, len(configs)),
		httpClient: httpClient,
	}

	// 复制账户配置
	copy(am.accounts, configs)

	// 初始化账户池
	for i := range am.accounts {
		am.pool <- &am.accounts[i]
	}

	Info("Account manager initialized with %d accounts", len(configs))
	return am, nil
}

// AcquireAccount 获取一个可用账户
// 支持 context 取消和超时
func (am *PooledAccountManager) AcquireAccount(ctx context.Context) (*JetbrainsAccount, error) {
	accountWaitStart := time.Now()

	// 最多重试3次，避免无限循环
	const maxRetries = 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			RecordAccountPoolError()
			return nil, fmt.Errorf("request cancelled while waiting for account: %w", ctx.Err())

		case account := <-am.pool:
			// 记录等待时间
			if attempt == 0 {
				waitDuration := time.Since(accountWaitStart)
				if waitDuration > 100*time.Millisecond {
					RecordAccountPoolWait(waitDuration)
				}
			}

			// 检查并刷新 JWT（如果需要）
			if account.LicenseID != "" {
				if account.JWT == "" || time.Now().After(account.ExpiryTime.Add(-JWTRefreshTime)) {
					if err := refreshJetbrainsJWT(account, am.httpClient); err != nil {
						Error("Failed to refresh JWT for %s (attempt %d/%d): %v", getTokenDisplayName(account), attempt+1, maxRetries, err)
						RecordAccountPoolError()
						// JWT刷新失败，放回池中，重试下一个账户
						am.ReleaseAccount(account)
						continue
					}
				}
			}

			// 检查配额
			if err := checkQuota(account, am.httpClient); err != nil {
				Error("Failed to check quota for %s (attempt %d/%d): %v", getTokenDisplayName(account), attempt+1, maxRetries, err)
				RecordAccountPoolError()
				// 配额检查失败，放回池中，重试下一个账户
				am.ReleaseAccount(account)
				continue
			}

			if !account.HasQuota {
				Warn("Account %s is over quota (attempt %d/%d)", getTokenDisplayName(account), attempt+1, maxRetries)
				// 配额不足，放回池中，重试下一个账户
				am.ReleaseAccount(account)
				continue
			}

			// 成功获取账户
			return account, nil

		case <-time.After(60 * time.Second):
			RecordAccountPoolError()
			return nil, fmt.Errorf("timed out waiting for an available JetBrains account")
		}
	}

	// 所有重试都失败
	RecordAccountPoolError()
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
		Warn("account pool is full, could not return account %s", getTokenDisplayName(account))
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

	return refreshJetbrainsJWT(account, am.httpClient)
}

// CheckQuota 检查账户配额
func (am *PooledAccountManager) CheckQuota(account *JetbrainsAccount) error {
	return checkQuota(account, am.httpClient)
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
	Info("Account manager shutting down")
	return nil
}
