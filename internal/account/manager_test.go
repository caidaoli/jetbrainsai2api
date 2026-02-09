package account

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"jetbrainsai2api/internal/core"
)

// TestPooledAccountManager_BasicAcquireRelease 测试基本的账户获取和释放
func TestPooledAccountManager_BasicAcquireRelease(t *testing.T) {
	accounts := []core.JetbrainsAccount{
		{
			JWT:            "test-jwt-1",
			HasQuota:       true,
			ExpiryTime:     time.Now().Add(24 * time.Hour),
			LastQuotaCheck: float64(time.Now().Unix()),
			LicenseID:      "test-license-1",
		},
	}

	am, err := NewPooledAccountManager(AccountManagerConfig{
		Accounts:   accounts,
		HTTPClient: &http.Client{},
	})
	if err != nil {
		t.Fatalf("Failed to create account manager: %v", err)
	}
	defer func() { _ = am.Close() }()

	ctx := context.Background()
	account, err := am.AcquireAccount(ctx)
	if err != nil {
		t.Fatalf("Failed to acquire account: %v", err)
	}

	if account.JWT != "test-jwt-1" {
		t.Errorf("Expected JWT 'test-jwt-1', got '%s'", account.JWT)
	}

	am.ReleaseAccount(account)

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
	accounts := []core.JetbrainsAccount{
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
	defer func() { _ = am.Close() }()

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

			time.Sleep(10 * time.Millisecond)

			am.ReleaseAccount(account)
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Goroutine error: %v", err)
	}

	if am.GetAvailableCount() != 2 {
		t.Errorf("Expected 2 available accounts, got %d", am.GetAvailableCount())
	}
}

// TestPooledAccountManager_AcquireTimeout 测试获取超时
func TestPooledAccountManager_AcquireTimeout(t *testing.T) {
	now := time.Now()
	accounts := []core.JetbrainsAccount{
		{JWT: "test-jwt-1", HasQuota: true, ExpiryTime: now.Add(24 * time.Hour), LastQuotaCheck: float64(now.Unix()), LicenseID: "test-1"},
	}

	am, err := NewPooledAccountManager(AccountManagerConfig{
		Accounts:   accounts,
		HTTPClient: &http.Client{},
	})
	if err != nil {
		t.Fatalf("Failed to create account manager: %v", err)
	}
	defer func() { _ = am.Close() }()

	ctx := context.Background()
	account1, err := am.AcquireAccount(ctx)
	if err != nil {
		t.Fatalf("Failed to acquire first account: %v", err)
	}

	ctx2, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = am.AcquireAccount(ctx2)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	am.ReleaseAccount(account1)

	account2, err := am.AcquireAccount(context.Background())
	if err != nil {
		t.Errorf("Failed to acquire after release: %v", err)
	}
	am.ReleaseAccount(account2)
}

// TestPooledAccountManager_ContextCancellation 测试上下文取消
func TestPooledAccountManager_ContextCancellation(t *testing.T) {
	now := time.Now()
	accounts := []core.JetbrainsAccount{
		{JWT: "test-jwt-1", HasQuota: true, ExpiryTime: now.Add(24 * time.Hour), LastQuotaCheck: float64(now.Unix()), LicenseID: "test-1"},
	}

	am, err := NewPooledAccountManager(AccountManagerConfig{
		Accounts:   accounts,
		HTTPClient: &http.Client{},
	})
	if err != nil {
		t.Fatalf("Failed to create account manager: %v", err)
	}
	defer func() { _ = am.Close() }()

	account1, _ := am.AcquireAccount(context.Background())

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := am.AcquireAccount(ctx)
		done <- err
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	err = <-done
	if err == nil {
		t.Error("Expected cancellation error, got nil")
	}

	am.ReleaseAccount(account1)
}

// TestPooledAccountManager_ReleaseNil 测试释放 nil 账户
func TestPooledAccountManager_ReleaseNil(t *testing.T) {
	now := time.Now()
	accounts := []core.JetbrainsAccount{
		{JWT: "test-jwt-1", HasQuota: true, ExpiryTime: now.Add(24 * time.Hour), LastQuotaCheck: float64(now.Unix()), LicenseID: "test-1"},
	}

	am, err := NewPooledAccountManager(AccountManagerConfig{
		Accounts:   accounts,
		HTTPClient: &http.Client{},
	})
	if err != nil {
		t.Fatalf("Failed to create account manager: %v", err)
	}
	defer func() { _ = am.Close() }()

	am.ReleaseAccount(nil)

	if am.GetAvailableCount() != 1 {
		t.Errorf("Expected 1 available account, got %d", am.GetAvailableCount())
	}
}

// TestPooledAccountManager_GetAccountCount 测试账户计数
func TestPooledAccountManager_GetAccountCount(t *testing.T) {
	now := time.Now()
	accounts := []core.JetbrainsAccount{
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
	defer func() { _ = am.Close() }()

	if am.GetAccountCount() != 3 {
		t.Errorf("Expected 3 total accounts, got %d", am.GetAccountCount())
	}

	if am.GetAvailableCount() != 3 {
		t.Errorf("Expected 3 available accounts, got %d", am.GetAvailableCount())
	}

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
		Accounts:   []core.JetbrainsAccount{},
		HTTPClient: &http.Client{},
	})
	if err == nil {
		t.Error("Expected error when creating manager with no accounts")
	}
}

// TestPooledAccountManager_GetAllAccounts 测试获取所有账户信息
func TestPooledAccountManager_GetAllAccounts(t *testing.T) {
	now := time.Now()
	accounts := []core.JetbrainsAccount{
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
	defer func() { _ = am.Close() }()

	allAccounts := am.GetAllAccounts()
	if len(allAccounts) != 2 {
		t.Errorf("Expected 2 accounts, got %d", len(allAccounts))
	}

	allAccounts[0].JWT = "modified"
	originalAccounts := am.GetAllAccounts()
	if originalAccounts[0].JWT == "modified" {
		t.Error("GetAllAccounts should return a copy, not the original slice")
	}
}

// TestPooledAccountManager_GetAllAccounts_ConcurrentSnapshot 验证并发快照读取无数据竞争
func TestPooledAccountManager_GetAllAccounts_ConcurrentSnapshot(t *testing.T) {
	now := time.Now()
	accounts := []core.JetbrainsAccount{
		{JWT: "test-jwt-1", HasQuota: true, ExpiryTime: now.Add(24 * time.Hour), LastQuotaCheck: float64(now.Unix()), LicenseID: "test-1"},
	}

	am, err := NewPooledAccountManager(AccountManagerConfig{
		Accounts:   accounts,
		HTTPClient: &http.Client{},
	})
	if err != nil {
		t.Fatalf("Failed to create account manager: %v", err)
	}
	defer func() { _ = am.Close() }()

	var wg sync.WaitGroup
	errCh := make(chan error, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			SetAccountQuotaStatus(&am.accounts[0], i%2 == 0, time.Now())
			am.accounts[0].Mu.Lock()
			am.accounts[0].JWT = "test-jwt-updated"
			am.accounts[0].Mu.Unlock()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			snapshot := am.GetAllAccounts()
			if len(snapshot) != 1 {
				select {
				case errCh <- fmt.Errorf("expected 1 account, got %d", len(snapshot)):
				default:
				}
				return
			}
		}
	}()

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestPooledAccountManager_CheckQuotaErrorShouldNotMutateState 验证配额检查失败不会污染账户状态
func TestPooledAccountManager_CheckQuotaErrorShouldNotMutateState(t *testing.T) {
	accounts := []core.JetbrainsAccount{
		{
			LicenseID:      "",
			Authorization:  "",
			JWT:            "",
			HasQuota:       true,
			LastQuotaCheck: 0,
		},
	}

	am, err := NewPooledAccountManager(AccountManagerConfig{
		Accounts:   accounts,
		HTTPClient: &http.Client{},
	})
	if err != nil {
		t.Fatalf("Failed to create account manager: %v", err)
	}
	defer func() { _ = am.Close() }()

	err = am.CheckQuota(&am.accounts[0])
	if err == nil {
		t.Fatal("期望配额检查失败，但返回 nil")
	}

	if !am.accounts[0].HasQuota {
		t.Fatalf("配额检查失败不应把账户标记为无配额")
	}
	if am.accounts[0].LastQuotaCheck != 0 {
		t.Fatalf("配额检查失败不应更新时间戳，实际: %v", am.accounts[0].LastQuotaCheck)
	}
}
