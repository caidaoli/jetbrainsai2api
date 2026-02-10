package account

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"jetbrainsai2api/internal/core"

	"github.com/bytedance/sonic"
)

// TestParseJWTExpiry 测试JWT过期时间解析
func TestParseJWTExpiry(t *testing.T) {
	makeTestJWT := func(payload map[string]any) string {
		header := map[string]string{
			"alg": "RS256",
			"typ": "JWT",
		}
		headerJSON, _ := sonic.Marshal(header)
		headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)

		payloadJSON, _ := sonic.Marshal(payload)
		payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)

		signature := base64.RawURLEncoding.EncodeToString([]byte("test-signature"))

		return fmt.Sprintf("%s.%s.%s", headerB64, payloadB64, signature)
	}

	tests := []struct {
		name        string
		tokenStr    string
		expectError bool
		errorMsg    string
		expectTime  *time.Time
	}{
		{
			name:        "无效格式-单段",
			tokenStr:    "single-part",
			expectError: true,
			errorMsg:    "expected 3 parts, got 1",
		},
		{
			name:        "无效格式-两段",
			tokenStr:    "part1.part2",
			expectError: true,
			errorMsg:    "expected 3 parts, got 2",
		},
		{
			name:        "无效格式-四段",
			tokenStr:    "part1.part2.part3.part4",
			expectError: true,
			errorMsg:    "expected 3 parts, got 4",
		},
		{
			name:        "无效格式-空字符串",
			tokenStr:    "",
			expectError: true,
			errorMsg:    "expected 3 parts, got 1",
		},
		{
			name:        "无效JWT内容-非法base64 payload",
			tokenStr:    "eyJhbGciOiJSUzI1NiJ9.invalid!!!base64.signature",
			expectError: true,
			errorMsg:    "could not parse JWT",
		},
		{
			name:        "无效JWT内容-payload非JSON",
			tokenStr:    "eyJhbGciOiJSUzI1NiJ9." + base64.RawURLEncoding.EncodeToString([]byte("not-json")) + ".sig",
			expectError: true,
			errorMsg:    "could not parse JWT",
		},
		{
			name:        "缺少exp字段",
			tokenStr:    makeTestJWT(map[string]any{"sub": "user123", "iat": 1700000000}),
			expectError: true,
			errorMsg:    "JWT missing exp claim",
		},
		{
			name:        "exp字段类型错误-字符串",
			tokenStr:    makeTestJWT(map[string]any{"exp": "not-a-number"}),
			expectError: true,
			errorMsg:    "JWT missing exp claim",
		},
		{
			name:        "exp字段类型错误-对象",
			tokenStr:    makeTestJWT(map[string]any{"exp": map[string]any{"time": 1700000000}}),
			expectError: true,
			errorMsg:    "JWT missing exp claim",
		},
		{
			name:        "正常JWT-过去时间",
			tokenStr:    makeTestJWT(map[string]any{"exp": float64(1600000000), "sub": "user123"}),
			expectError: false,
			expectTime:  ptrTime(time.Unix(1600000000, 0)),
		},
		{
			name:        "正常JWT-未来时间",
			tokenStr:    makeTestJWT(map[string]any{"exp": float64(2000000000), "sub": "user123"}),
			expectError: false,
			expectTime:  ptrTime(time.Unix(2000000000, 0)),
		},
		{
			name:        "正常JWT-当前时间附近",
			tokenStr:    makeTestJWT(map[string]any{"exp": float64(time.Now().Unix() + 3600)}),
			expectError: false,
			expectTime:  ptrTime(time.Unix(time.Now().Unix()+3600, 0)),
		},
		{
			name: "正常JWT-完整claims",
			tokenStr: makeTestJWT(map[string]any{
				"aud":       "ai-authorization-server",
				"exp":       float64(1719249258),
				"accountId": "b8520461-53e9-4a03-b52f-f1405a4de309",
				"realm":     "jb",
				"iss":       "https://oauth.jetbrains.com/oauth/okta",
				"iat":       float64(1719162858),
			}),
			expectError: false,
			expectTime:  ptrTime(time.Unix(1719249258, 0)),
		},
		{
			name:        "边界情况-exp为0",
			tokenStr:    makeTestJWT(map[string]any{"exp": float64(0)}),
			expectError: false,
			expectTime:  ptrTime(time.Unix(0, 0)),
		},
		{
			name:        "边界情况-exp为负数",
			tokenStr:    makeTestJWT(map[string]any{"exp": float64(-1)}),
			expectError: false,
			expectTime:  ptrTime(time.Unix(-1, 0)),
		},
		{
			name:        "边界情况-超大exp值",
			tokenStr:    makeTestJWT(map[string]any{"exp": float64(9999999999)}),
			expectError: false,
			expectTime:  ptrTime(time.Unix(9999999999, 0)),
		},
		{
			name:        "真实JWT样例-JetBrains格式",
			tokenStr:    "eyJhbGciOiJSUzI1NiJ9.eyJhdWQiOiJhaS1hdXRob3JpemF0aW9uLXNlcnZlciIsImV4cCI6MTcxOTI0OTI1OCwiYWNjb3VudElkIjoiYjg1MjA0NjEtNTNlOS00YTAzLWI1MmYtZjE0MDVhNGRlMzA5IiwicmVhbG0iOiJqYiIsImlzcyI6Imh0dHBzOi8vb2F1dGguamV0YnJhaW5zLmNvbS9vYXV0aC9va3RhIiwiaWF0IjoxNzE5MTYyODU4LCJ0b2tlbklkIjoiYWM0NTJmYmMtY2ZmNS00YjI2LTg2YzgtZjlkMzE2ZGUwYzI3IiwiYWNjZXNzTGljZW5zZUlkcyI6W119.SIGNATURE",
			expectError: false,
			expectTime:  ptrTime(time.Unix(1719249258, 0)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseJWTExpiry(tt.tokenStr)

			if tt.expectError {
				if err == nil {
					t.Errorf("期望错误，但函数成功返回")
					return
				}
				if tt.errorMsg != "" && !containsErrorMsg(err.Error(), tt.errorMsg) {
					t.Errorf("错误消息不匹配\n期望包含: %s\n实际: %s", tt.errorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("期望成功，但返回错误: %v", err)
				return
			}

			if tt.expectTime == nil {
				t.Errorf("测试用例配置错误：expectTime为nil但expectError为false")
				return
			}

			if !timesEqual(result, *tt.expectTime, time.Second) {
				t.Errorf("时间不匹配\n期望: %s (Unix: %d)\n实际: %s (Unix: %d)",
					tt.expectTime.Format(time.RFC3339),
					tt.expectTime.Unix(),
					result.Format(time.RFC3339),
					result.Unix(),
				)
			}
		})
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func containsErrorMsg(actual, expected string) bool {
	return len(expected) == 0 || (len(actual) > 0 && strings.Contains(actual, expected))
}

func timesEqual(t1, t2 time.Time, tolerance time.Duration) bool {
	diff := t1.Sub(t2)
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}

// TestProcessQuotaData 测试配额数据处理函数
func TestProcessQuotaData(t *testing.T) {
	tests := []struct {
		name           string
		quotaData      *core.JetbrainsQuotaResponse
		initialAccount *core.JetbrainsAccount
		expectHasQuota bool
		description    string
	}{
		{
			name: "正常配额-有剩余",
			quotaData: &core.JetbrainsQuotaResponse{
				Current: core.QuotaUsage{
					Current: core.QuotaAmount{Amount: "50.5"},
					Maximum: core.QuotaAmount{Amount: "100.0"},
				},
				Until: "2025-12-15T00:00:00Z",
			},
			initialAccount: &core.JetbrainsAccount{
				LicenseID: "test-license-1",
				HasQuota:  false,
			},
			expectHasQuota: true,
			description:    "使用量 50.5 < 最大值 100.0，应该有配额",
		},
		{
			name: "配额耗尽-相等",
			quotaData: &core.JetbrainsQuotaResponse{
				Current: core.QuotaUsage{
					Current: core.QuotaAmount{Amount: "100.0"},
					Maximum: core.QuotaAmount{Amount: "100.0"},
				},
				Until: "2025-12-15T00:00:00Z",
			},
			initialAccount: &core.JetbrainsAccount{
				LicenseID: "test-license-2",
				HasQuota:  true,
			},
			expectHasQuota: false,
			description:    "使用量 100.0 = 最大值 100.0，应该无配额",
		},
		{
			name: "边界值-零使用量",
			quotaData: &core.JetbrainsQuotaResponse{
				Current: core.QuotaUsage{
					Current: core.QuotaAmount{Amount: "0"},
					Maximum: core.QuotaAmount{Amount: "100.0"},
				},
				Until: "2025-12-15T00:00:00Z",
			},
			initialAccount: &core.JetbrainsAccount{
				LicenseID: "test-license-4",
				HasQuota:  false,
			},
			expectHasQuota: true,
			description:    "使用量为 0，应该有配额",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beforeTime := time.Now().Unix()

			ProcessQuotaData(tt.quotaData, tt.initialAccount, &core.NopLogger{})

			afterTime := time.Now().Unix()

			if tt.initialAccount.HasQuota != tt.expectHasQuota {
				t.Errorf("%s\n期望 HasQuota = %v，实际 = %v",
					tt.description, tt.expectHasQuota, tt.initialAccount.HasQuota)
			}

			if tt.initialAccount.LastQuotaCheck < float64(beforeTime) ||
				tt.initialAccount.LastQuotaCheck > float64(afterTime) {
				t.Errorf("LastQuotaCheck 未正确更新\n期望范围: [%d, %d]\n实际: %.0f",
					beforeTime, afterTime, tt.initialAccount.LastQuotaCheck)
			}
		})
	}
}

// TestProcessQuotaData_ConcurrentAccess 测试并发访问场景
func TestProcessQuotaData_ConcurrentAccess(t *testing.T) {
	account := &core.JetbrainsAccount{
		LicenseID: "concurrent-test",
		HasQuota:  false,
	}

	quotaData := &core.JetbrainsQuotaResponse{
		Current: core.QuotaUsage{
			Current: core.QuotaAmount{Amount: "50.0"},
			Maximum: core.QuotaAmount{Amount: "100.0"},
		},
		Until: "2025-12-15T00:00:00Z",
	}

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			ProcessQuotaData(quotaData, account, &core.NopLogger{})
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if !account.HasQuota {
		t.Errorf("并发调用后 HasQuota 应该为 true")
	}

	if account.LastQuotaCheck == 0 {
		t.Errorf("并发调用后 LastQuotaCheck 应该被更新")
	}
}

// TestMarkAccountNoQuota_ConcurrentAccess 验证并发标记配额不足不会触发数据竞争
func TestMarkAccountNoQuota_ConcurrentAccess(t *testing.T) {
	account := &core.JetbrainsAccount{
		LicenseID: "concurrent-noquota-test",
		HasQuota:  true,
	}

	done := make(chan bool)
	for i := 0; i < 20; i++ {
		go func() {
			MarkAccountNoQuota(account)
			done <- true
		}()
	}

	for i := 0; i < 20; i++ {
		<-done
	}

	if account.HasQuota {
		t.Errorf("并发标记后 HasQuota 应该为 false")
	}

	if account.LastQuotaCheck == 0 {
		t.Errorf("并发标记后 LastQuotaCheck 应该被更新")
	}
}
