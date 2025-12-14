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
