package main

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/sonic"
)

// TestParseJWTExpiry 测试JWT过期时间解析
func TestParseJWTExpiry(t *testing.T) {
	// 辅助函数：构造简单的测试JWT（header.payload.signature格式）
	makeTestJWT := func(payload map[string]any) string {
		// JWT Header（标准）
		header := map[string]string{
			"alg": "RS256",
			"typ": "JWT",
		}
		headerJSON, _ := sonic.Marshal(header)
		headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)

		// JWT Payload
		payloadJSON, _ := sonic.Marshal(payload)
		payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)

		// JWT Signature（测试用，不需要真实签名）
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
			result, err := parseJWTExpiry(tt.tokenStr)

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

			// 期望成功
			if err != nil {
				t.Errorf("期望成功，但返回错误: %v", err)
				return
			}

			if tt.expectTime == nil {
				t.Errorf("测试用例配置错误：expectTime为nil但expectError为false")
				return
			}

			// 验证时间是否匹配（允许1秒误差，处理时区等问题）
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

// 辅助函数：创建time.Time指针
func ptrTime(t time.Time) *time.Time {
	return &t
}

// 辅助函数：检查错误消息是否包含预期文本
func containsErrorMsg(actual, expected string) bool {
	return len(expected) == 0 || (len(actual) > 0 && strings.Contains(actual, expected))
}

// 辅助函数：比较两个时间是否相等（允许误差）
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
		quotaData      *JetbrainsQuotaResponse
		initialAccount *JetbrainsAccount
		expectHasQuota bool
		description    string
	}{
		{
			name: "正常配额-有剩余",
			quotaData: &JetbrainsQuotaResponse{
				Current: struct {
					Current struct {
						Amount string `json:"amount"`
					} `json:"current"`
					Maximum struct {
						Amount string `json:"amount"`
					} `json:"maximum"`
				}{
					Current: struct {
						Amount string `json:"amount"`
					}{Amount: "50.5"},
					Maximum: struct {
						Amount string `json:"amount"`
					}{Amount: "100.0"},
				},
				Until: "2025-12-15T00:00:00Z",
			},
			initialAccount: &JetbrainsAccount{
				LicenseID: "test-license-1",
				HasQuota:  false,
			},
			expectHasQuota: true,
			description:    "使用量 50.5 < 最大值 100.0，应该有配额",
		},
		{
			name: "配额耗尽-相等",
			quotaData: &JetbrainsQuotaResponse{
				Current: struct {
					Current struct {
						Amount string `json:"amount"`
					} `json:"current"`
					Maximum struct {
						Amount string `json:"amount"`
					} `json:"maximum"`
				}{
					Current: struct {
						Amount string `json:"amount"`
					}{Amount: "100.0"},
					Maximum: struct {
						Amount string `json:"amount"`
					}{Amount: "100.0"},
				},
				Until: "2025-12-15T00:00:00Z",
			},
			initialAccount: &JetbrainsAccount{
				LicenseID: "test-license-2",
				HasQuota:  true,
			},
			expectHasQuota: false,
			description:    "使用量 100.0 = 最大值 100.0，应该无配额",
		},
		{
			name: "配额超额",
			quotaData: &JetbrainsQuotaResponse{
				Current: struct {
					Current struct {
						Amount string `json:"amount"`
					} `json:"current"`
					Maximum struct {
						Amount string `json:"amount"`
					} `json:"maximum"`
				}{
					Current: struct {
						Amount string `json:"amount"`
					}{Amount: "150.0"},
					Maximum: struct {
						Amount string `json:"amount"`
					}{Amount: "100.0"},
				},
				Until: "2025-12-15T00:00:00Z",
			},
			initialAccount: &JetbrainsAccount{
				LicenseID: "test-license-3",
				HasQuota:  true,
			},
			expectHasQuota: false,
			description:    "使用量 150.0 > 最大值 100.0，应该无配额",
		},
		{
			name: "边界值-零使用量",
			quotaData: &JetbrainsQuotaResponse{
				Current: struct {
					Current struct {
						Amount string `json:"amount"`
					} `json:"current"`
					Maximum struct {
						Amount string `json:"amount"`
					} `json:"maximum"`
				}{
					Current: struct {
						Amount string `json:"amount"`
					}{Amount: "0"},
					Maximum: struct {
						Amount string `json:"amount"`
					}{Amount: "100.0"},
				},
				Until: "2025-12-15T00:00:00Z",
			},
			initialAccount: &JetbrainsAccount{
				LicenseID: "test-license-4",
				HasQuota:  false,
			},
			expectHasQuota: true,
			description:    "使用量为 0，应该有配额",
		},
		{
			name: "边界值-零最大配额",
			quotaData: &JetbrainsQuotaResponse{
				Current: struct {
					Current struct {
						Amount string `json:"amount"`
					} `json:"current"`
					Maximum struct {
						Amount string `json:"amount"`
					} `json:"maximum"`
				}{
					Current: struct {
						Amount string `json:"amount"`
					}{Amount: "0"},
					Maximum: struct {
						Amount string `json:"amount"`
					}{Amount: "0"},
				},
				Until: "2025-12-15T00:00:00Z",
			},
			initialAccount: &JetbrainsAccount{
				LicenseID: "test-license-5",
				HasQuota:  true,
			},
			expectHasQuota: true,
			description:    "最大配额为 0 时防止除零错误，容错值设为 1，使用量 0 < 1，应该有配额",
		},
		{
			name: "无效数据-空字符串",
			quotaData: &JetbrainsQuotaResponse{
				Current: struct {
					Current struct {
						Amount string `json:"amount"`
					} `json:"current"`
					Maximum struct {
						Amount string `json:"amount"`
					} `json:"maximum"`
				}{
					Current: struct {
						Amount string `json:"amount"`
					}{Amount: ""},
					Maximum: struct {
						Amount string `json:"amount"`
					}{Amount: ""},
				},
				Until: "2025-12-15T00:00:00Z",
			},
			initialAccount: &JetbrainsAccount{
				LicenseID: "test-license-6",
				HasQuota:  true,
			},
			expectHasQuota: true,
			description:    "空字符串解析为 0，最大值容错为 1，使用量 0 < 1，应该有配额",
		},
		{
			name: "无效数据-非数字字符串",
			quotaData: &JetbrainsQuotaResponse{
				Current: struct {
					Current struct {
						Amount string `json:"amount"`
					} `json:"current"`
					Maximum struct {
						Amount string `json:"amount"`
					} `json:"maximum"`
				}{
					Current: struct {
						Amount string `json:"amount"`
					}{Amount: "invalid"},
					Maximum: struct {
						Amount string `json:"amount"`
					}{Amount: "not-a-number"},
				},
				Until: "2025-12-15T00:00:00Z",
			},
			initialAccount: &JetbrainsAccount{
				LicenseID: "test-license-7",
				HasQuota:  true,
			},
			expectHasQuota: true,
			description:    "非数字字符串解析失败默认为 0，最大值容错为 1，使用量 0 < 1，应该有配额",
		},
		{
			name: "浮点数精度测试",
			quotaData: &JetbrainsQuotaResponse{
				Current: struct {
					Current struct {
						Amount string `json:"amount"`
					} `json:"current"`
					Maximum struct {
						Amount string `json:"amount"`
					} `json:"maximum"`
				}{
					Current: struct {
						Amount string `json:"amount"`
					}{Amount: "99.9999999"},
					Maximum: struct {
						Amount string `json:"amount"`
					}{Amount: "100.0"},
				},
				Until: "2025-12-15T00:00:00Z",
			},
			initialAccount: &JetbrainsAccount{
				LicenseID: "test-license-8",
				HasQuota:  false,
			},
			expectHasQuota: true,
			description:    "浮点数精度测试，99.9999999 < 100.0，应该有配额",
		},
		{
			name: "Premium用户-高配额",
			quotaData: &JetbrainsQuotaResponse{
				Current: struct {
					Current struct {
						Amount string `json:"amount"`
					} `json:"current"`
					Maximum struct {
						Amount string `json:"amount"`
					} `json:"maximum"`
				}{
					Current: struct {
						Amount string `json:"amount"`
					}{Amount: "500.0"},
					Maximum: struct {
						Amount string `json:"amount"`
					}{Amount: "10000.0"},
				},
				Until: "2025-12-31T23:59:59Z",
			},
			initialAccount: &JetbrainsAccount{
				LicenseID: "premium-license",
				HasQuota:  false,
			},
			expectHasQuota: true,
			description:    "Premium 用户高配额测试，500 < 10000，应该有配额",
		},
		{
			name: "非Premium用户-接近配额上限",
			quotaData: &JetbrainsQuotaResponse{
				Current: struct {
					Current struct {
						Amount string `json:"amount"`
					} `json:"current"`
					Maximum struct {
						Amount string `json:"amount"`
					} `json:"maximum"`
				}{
					Current: struct {
						Amount string `json:"amount"`
					}{Amount: "99.0"},
					Maximum: struct {
						Amount string `json:"amount"`
					}{Amount: "100.0"},
				},
				Until: "2025-12-15T00:00:00Z",
			},
			initialAccount: &JetbrainsAccount{
				LicenseID: "regular-license",
				HasQuota:  false,
			},
			expectHasQuota: true,
			description:    "普通用户接近配额上限，99 < 100，应该有配额",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 记录测试前的时间，用于验证 LastQuotaCheck 更新
			beforeTime := time.Now().Unix()

			// 调用被测函数
			processQuotaData(tt.quotaData, tt.initialAccount, &NopLogger{})

			// 记录测试后的时间
			afterTime := time.Now().Unix()

			// 验证 HasQuota 状态
			if tt.initialAccount.HasQuota != tt.expectHasQuota {
				t.Errorf("%s\n期望 HasQuota = %v，实际 = %v",
					tt.description,
					tt.expectHasQuota,
					tt.initialAccount.HasQuota,
				)
			}

			// 验证 LastQuotaCheck 已更新
			if tt.initialAccount.LastQuotaCheck < float64(beforeTime) ||
				tt.initialAccount.LastQuotaCheck > float64(afterTime) {
				t.Errorf("LastQuotaCheck 未正确更新\n期望范围: [%d, %d]\n实际: %.0f",
					beforeTime,
					afterTime,
					tt.initialAccount.LastQuotaCheck,
				)
			}
		})
	}
}

// TestProcessQuotaData_ConcurrentAccess 测试并发访问场景
func TestProcessQuotaData_ConcurrentAccess(t *testing.T) {
	account := &JetbrainsAccount{
		LicenseID: "concurrent-test",
		HasQuota:  false,
	}

	quotaData := &JetbrainsQuotaResponse{
		Current: struct {
			Current struct {
				Amount string `json:"amount"`
			} `json:"current"`
			Maximum struct {
				Amount string `json:"amount"`
			} `json:"maximum"`
		}{
			Current: struct {
				Amount string `json:"amount"`
			}{Amount: "50.0"},
			Maximum: struct {
				Amount string `json:"amount"`
			}{Amount: "100.0"},
		},
		Until: "2025-12-15T00:00:00Z",
	}

	// 并发调用 processQuotaData
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			processQuotaData(quotaData, account, &NopLogger{})
			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证最终状态一致
	if !account.HasQuota {
		t.Errorf("并发调用后 HasQuota 应该为 true")
	}

	if account.LastQuotaCheck == 0 {
		t.Errorf("并发调用后 LastQuotaCheck 应该被更新")
	}
}

// TestMarkAccountNoQuota_ConcurrentAccess 验证并发标记配额不足不会触发数据竞争
func TestMarkAccountNoQuota_ConcurrentAccess(t *testing.T) {
	account := &JetbrainsAccount{
		LicenseID: "concurrent-noquota-test",
		HasQuota:  true,
	}

	done := make(chan bool)
	for i := 0; i < 20; i++ {
		go func() {
			markAccountNoQuota(account)
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
