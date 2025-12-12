package main

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"
)

// TestPooledAccountManager_BasicAcquireRelease 测试基本的账户获取和释放
func TestPooledAccountManager_BasicAcquireRelease(t *testing.T) {
	accounts := []JetbrainsAccount{
		{
			JWT:            "test-jwt-1",
			HasQuota:       true,
			ExpiryTime:     time.Now().Add(24 * time.Hour),
			LastQuotaCheck: float64(time.Now().Unix()), // 设置最近检查时间，避免真实API调用
			LicenseID:      "test-license-1",           // 设置LicenseID避免JWT刷新
		},
	}

	am, err := NewPooledAccountManager(AccountManagerConfig{
		Accounts:   accounts,
		HTTPClient: &http.Client{},
	})
	if err != nil {
		t.Fatalf("Failed to create account manager: %v", err)
	}
	defer am.Close()

	// 获取账户
	ctx := context.Background()
	account, err := am.AcquireAccount(ctx)
	if err != nil {
		t.Fatalf("Failed to acquire account: %v", err)
	}

	if account.JWT != "test-jwt-1" {
		t.Errorf("Expected JWT 'test-jwt-1', got '%s'", account.JWT)
	}

	// 释放账户
	am.ReleaseAccount(account)

	// 再次获取应该成功
	account2, err := am.AcquireAccount(ctx)
	if err != nil {
		t.Fatalf("Failed to acquire account second time: %v", err)
	}

	if account2.JWT != "test-jwt-1" {
		t.Errorf("Expected same account, got different JWT")
	}

	am.ReleaseAccount(account2)
}

// TestPooledAccountManager_ConcurrentAcquire 测试并发获取账户
func TestPooledAccountManager_ConcurrentAcquire(t *testing.T) {
	now := time.Now()
	accounts := []JetbrainsAccount{
		{JWT: "test-jwt-1", HasQuota: true, ExpiryTime: now.Add(24 * time.Hour), LastQuotaCheck: float64(now.Unix()), LicenseID: "test-1"},
		{JWT: "test-jwt-2", HasQuota: true, ExpiryTime: now.Add(24 * time.Hour), LastQuotaCheck: float64(now.Unix()), LicenseID: "test-2"},
	}

	am, err := NewPooledAccountManager(AccountManagerConfig{
		Accounts:   accounts,
		HTTPClient: &http.Client{},
	})
	if err != nil {
		t.Fatalf("Failed to create account manager: %v", err)
	}
	defer am.Close()

	const numGoroutines = 10
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			ctx := context.Background()
			account, err := am.AcquireAccount(ctx)
			if err != nil {
				errors <- err
				return
			}

			// 模拟使用账户
			time.Sleep(10 * time.Millisecond)

			am.ReleaseAccount(account)
		}(i)
	}

	wg.Wait()
	close(errors)

	// 检查是否有错误
	for err := range errors {
		t.Errorf("Goroutine error: %v", err)
	}

	// 验证所有账户都被释放
	if am.GetAvailableCount() != 2 {
		t.Errorf("Expected 2 available accounts, got %d", am.GetAvailableCount())
	}
}

// TestPooledAccountManager_AcquireTimeout 测试获取超时
func TestPooledAccountManager_AcquireTimeout(t *testing.T) {
	now := time.Now()
	accounts := []JetbrainsAccount{
		{JWT: "test-jwt-1", HasQuota: true, ExpiryTime: now.Add(24 * time.Hour), LastQuotaCheck: float64(now.Unix()), LicenseID: "test-1"},
	}

	am, err := NewPooledAccountManager(AccountManagerConfig{
		Accounts:   accounts,
		HTTPClient: &http.Client{},
	})
	if err != nil {
		t.Fatalf("Failed to create account manager: %v", err)
	}
	defer am.Close()

	// 占用唯一账户
	ctx := context.Background()
	account1, err := am.AcquireAccount(ctx)
	if err != nil {
		t.Fatalf("Failed to acquire first account: %v", err)
	}

	// 第二个请求应该超时
	ctx2, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = am.AcquireAccount(ctx2)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	// 释放账户
	am.ReleaseAccount(account1)

	// 现在应该能获取
	account2, err := am.AcquireAccount(context.Background())
	if err != nil {
		t.Errorf("Failed to acquire after release: %v", err)
	}
	am.ReleaseAccount(account2)
}

// TestPooledAccountManager_ContextCancellation 测试上下文取消
func TestPooledAccountManager_ContextCancellation(t *testing.T) {
	now := time.Now()
	accounts := []JetbrainsAccount{
		{JWT: "test-jwt-1", HasQuota: true, ExpiryTime: now.Add(24 * time.Hour), LastQuotaCheck: float64(now.Unix()), LicenseID: "test-1"},
	}

	am, err := NewPooledAccountManager(AccountManagerConfig{
		Accounts:   accounts,
		HTTPClient: &http.Client{},
	})
	if err != nil {
		t.Fatalf("Failed to create account manager: %v", err)
	}
	defer am.Close()

	// 占用唯一账户
	account1, _ := am.AcquireAccount(context.Background())

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())

	// 在另一个 goroutine 中尝试获取
	done := make(chan error, 1)
	go func() {
		_, err := am.AcquireAccount(ctx)
		done <- err
	}()

	// 等待一小段时间后取消
	time.Sleep(50 * time.Millisecond)
	cancel()

	// 应该收到取消错误
	err = <-done
	if err == nil {
		t.Error("Expected cancellation error, got nil")
	}

	am.ReleaseAccount(account1)
}

// TestPooledAccountManager_ReleaseNil 测试释放 nil 账户
func TestPooledAccountManager_ReleaseNil(t *testing.T) {
	now := time.Now()
	accounts := []JetbrainsAccount{
		{JWT: "test-jwt-1", HasQuota: true, ExpiryTime: now.Add(24 * time.Hour), LastQuotaCheck: float64(now.Unix()), LicenseID: "test-1"},
	}

	am, err := NewPooledAccountManager(AccountManagerConfig{
		Accounts:   accounts,
		HTTPClient: &http.Client{},
	})
	if err != nil {
		t.Fatalf("Failed to create account manager: %v", err)
	}
	defer am.Close()

	// 释放 nil 不应该 panic
	am.ReleaseAccount(nil)

	// 验证账户池仍然正常
	if am.GetAvailableCount() != 1 {
		t.Errorf("Expected 1 available account, got %d", am.GetAvailableCount())
	}
}

// TestPooledAccountManager_GetAccountCount 测试账户计数
func TestPooledAccountManager_GetAccountCount(t *testing.T) {
	now := time.Now()
	accounts := []JetbrainsAccount{
		{JWT: "test-jwt-1", HasQuota: true, ExpiryTime: now.Add(24 * time.Hour), LastQuotaCheck: float64(now.Unix()), LicenseID: "test-1"},
		{JWT: "test-jwt-2", HasQuota: true, ExpiryTime: now.Add(24 * time.Hour), LastQuotaCheck: float64(now.Unix()), LicenseID: "test-2"},
		{JWT: "test-jwt-3", HasQuota: true, ExpiryTime: now.Add(24 * time.Hour), LastQuotaCheck: float64(now.Unix()), LicenseID: "test-3"},
	}

	am, err := NewPooledAccountManager(AccountManagerConfig{
		Accounts:   accounts,
		HTTPClient: &http.Client{},
	})
	if err != nil {
		t.Fatalf("Failed to create account manager: %v", err)
	}
	defer am.Close()

	if am.GetAccountCount() != 3 {
		t.Errorf("Expected 3 total accounts, got %d", am.GetAccountCount())
	}

	if am.GetAvailableCount() != 3 {
		t.Errorf("Expected 3 available accounts, got %d", am.GetAvailableCount())
	}

	// 获取一个账户
	account, err := am.AcquireAccount(context.Background())
	if err != nil {
		t.Fatalf("Failed to acquire account: %v", err)
	}

	if am.GetAccountCount() != 3 {
		t.Errorf("Total count should not change, got %d", am.GetAccountCount())
	}

	if am.GetAvailableCount() != 2 {
		t.Errorf("Expected 2 available accounts, got %d", am.GetAvailableCount())
	}

	am.ReleaseAccount(account)

	if am.GetAvailableCount() != 3 {
		t.Errorf("Expected 3 available accounts after release, got %d", am.GetAvailableCount())
	}
}

// TestPooledAccountManager_NoAccounts 测试没有账户的情况
func TestPooledAccountManager_NoAccounts(t *testing.T) {
	_, err := NewPooledAccountManager(AccountManagerConfig{
		Accounts:   []JetbrainsAccount{},
		HTTPClient: &http.Client{},
	})
	if err == nil {
		t.Error("Expected error when creating manager with no accounts")
	}
}

// TestPooledAccountManager_GetAllAccounts 测试获取所有账户信息
func TestPooledAccountManager_GetAllAccounts(t *testing.T) {
	now := time.Now()
	accounts := []JetbrainsAccount{
		{JWT: "test-jwt-1", HasQuota: true, ExpiryTime: now.Add(24 * time.Hour), LastQuotaCheck: float64(now.Unix()), LicenseID: "test-1"},
		{JWT: "test-jwt-2", HasQuota: true, ExpiryTime: now.Add(24 * time.Hour), LastQuotaCheck: float64(now.Unix()), LicenseID: "test-2"},
	}

	am, err := NewPooledAccountManager(AccountManagerConfig{
		Accounts:   accounts,
		HTTPClient: &http.Client{},
	})
	if err != nil {
		t.Fatalf("Failed to create account manager: %v", err)
	}
	defer am.Close()

	allAccounts := am.GetAllAccounts()
	if len(allAccounts) != 2 {
		t.Errorf("Expected 2 accounts, got %d", len(allAccounts))
	}

	// 验证返回的是副本，不是原始切片
	allAccounts[0].JWT = "modified"
	originalAccounts := am.GetAllAccounts()
	if originalAccounts[0].JWT == "modified" {
		t.Error("GetAllAccounts should return a copy, not the original slice")
	}
}
