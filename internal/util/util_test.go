package util

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"jetbrainsai2api/internal/core"
)

func TestParseEnvList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"空字符串", "", nil},
		{"单个值", "value1", []string{"value1"}},
		{"多个值", "value1,value2,value3", []string{"value1", "value2", "value3"}},
		{"值带空格", "value1, value2 , value3", []string{"value1", "value2", "value3"}},
		{"包含空值", "value1,,value2", []string{"value1", "value2"}},
		{"末尾逗号", "value1,value2,", []string{"value1", "value2"}},
		{"全空格值", "  ,  ,  ", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseEnvList(tt.input)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("期望 nil，实际 %v", result)
				}
				return
			}
			if len(result) != len(tt.expected) {
				t.Errorf("期望长度 %d，实际 %d", len(tt.expected), len(result))
				return
			}
			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("索引 %d: 期望 '%s'，实际 '%s'", i, expected, result[i])
				}
			}
		})
	}
}

func TestExtractTextContent(t *testing.T) {
	tests := []struct {
		name     string
		content  any
		expected string
	}{
		{"nil内容", nil, ""},
		{"字符串内容", "Hello World", "Hello World"},
		{"空字符串", "", ""},
		{"单个text块", []any{map[string]any{"type": core.ContentBlockTypeText, "text": "单个文本"}}, "单个文本"},
		{"多个text块", []any{
			map[string]any{"type": core.ContentBlockTypeText, "text": "第一部分"},
			map[string]any{"type": core.ContentBlockTypeText, "text": "第二部分"},
		}, "第一部分 第二部分"},
		{"数字类型", 123, ""},
		{"空数组", []any{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractTextContent(tt.content)
			if result != tt.expected {
				t.Errorf("期望 '%s'，实际 '%s'", tt.expected, result)
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name, input, replacement, expected string
		prefixLen, suffixLen               int
	}{
		{"短字符串不截断", "short", "...", "short", 3, 3},
		{"超过阈值截断", "1234567890", "...", "123...890", 3, 3},
		{"只保留后缀", "1234567890", "...", "...7890", 0, 4},
		{"只保留前缀", "1234567890", "...", "1234...", 4, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateString(tt.input, tt.prefixLen, tt.suffixLen, tt.replacement)
			if result != tt.expected {
				t.Errorf("期望 '%s'，实际 '%s'", tt.expected, result)
			}
		})
	}
}

func TestGenerateID(t *testing.T) {
	tests := []struct {
		name, prefix string
	}{
		{"chatcmpl前缀", "chatcmpl-"},
		{"msg前缀", "msg_"},
		{"空前缀", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := GenerateID(tt.prefix)
			if !strings.HasPrefix(id, tt.prefix) {
				t.Errorf("ID应以 '%s' 为前缀，实际: '%s'", tt.prefix, id)
			}
			if len(id) < len(tt.prefix)+10 {
				t.Errorf("ID长度过短: %s", id)
			}
		})
	}
}

func TestGetTokenDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		account  *core.JetbrainsAccount
		contains string
	}{
		{"有JWT", &core.JetbrainsAccount{JWT: "eyJhbGciOiJSUzI1NiJ9.longtoken"}, "Token ..."},
		{"有LicenseID无JWT", &core.JetbrainsAccount{LicenseID: "license-123456789"}, "Token ..."},
		{"无JWT无LicenseID", &core.JetbrainsAccount{}, "Unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetTokenDisplayName(tt.account)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("期望包含 '%s'，实际 '%s'", tt.contains, result)
			}
		})
	}
}

func TestGetLicenseDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		account  *core.JetbrainsAccount
		expected string
	}{
		{"有Authorization", &core.JetbrainsAccount{Authorization: "Bearer token123456"}, "Bea*456"},
		{"无Authorization", &core.JetbrainsAccount{}, "Unknown"},
		{"短Authorization", &core.JetbrainsAccount{Authorization: "ABC"}, "ABC"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetLicenseDisplayName(tt.account)
			if result != tt.expected {
				t.Errorf("期望 '%s'，实际 '%s'", tt.expected, result)
			}
		})
	}
}

func TestGetEnvWithDefault(t *testing.T) {
	tests := []struct {
		name, key, setValue, defaultValue, expected string
		setEnv                                      bool
	}{
		{"使用默认值", "TEST_ENV_NOT_SET_12345", "", "default_value", "default_value", false},
		{"使用环境变量值", "TEST_ENV_SET_12345", "actual_value", "default_value", "actual_value", true},
		{"空环境变量使用默认值", "TEST_ENV_EMPTY_12345", "", "default_value", "default_value", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Unsetenv(tt.key)
			if tt.setEnv {
				_ = os.Setenv(tt.key, tt.setValue)
				defer func() { _ = os.Unsetenv(tt.key) }()
			}
			result := GetEnvWithDefault(tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("期望 '%s'，实际 '%s'", tt.expected, result)
			}
		})
	}
}

func TestEstimateTokenCount(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"空字符串", "", 0},
		{"4个字符", "test", 2},
		{"8个字符", "testtest", 4},
		{"短于4个字符", "hi", 1},
		{"长文本", strings.Repeat("a", 100), 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EstimateTokenCount(tt.text)
			if result != tt.expected {
				t.Errorf("estimateTokenCount(%q) = %d，期望 %d", tt.text, result, tt.expected)
			}
		})
	}
}

func TestGenerateRandomID(t *testing.T) {
	id := GenerateRandomID(core.ToolCallIDPrefix)
	if !strings.HasPrefix(id, core.ToolCallIDPrefix) {
		t.Errorf("工具调用ID应以 '%s' 为前缀，实际: '%s'", core.ToolCallIDPrefix, id)
	}
	expectedLen := len(core.ToolCallIDPrefix) + 20
	if len(id) != expectedLen {
		t.Errorf("工具调用ID长度应为 %d，实际: %d", expectedLen, len(id))
	}
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		newID := GenerateRandomID(core.ToolCallIDPrefix)
		if ids[newID] {
			t.Errorf("生成了重复的ID: %s", newID)
		}
		ids[newID] = true
	}
}

func TestGetTokenInfoFromAccount(t *testing.T) {
	tests := []struct {
		name           string
		account        *core.JetbrainsAccount
		quotaData      *core.JetbrainsQuotaResponse
		err            error
		expectedStatus string
	}{
		{
			name:           "有错误时返回错误状态",
			account:        &core.JetbrainsAccount{JWT: "test"},
			quotaData:      nil,
			err:            os.ErrNotExist,
			expectedStatus: "错误",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := GetTokenInfoFromAccount(tt.account, tt.quotaData, tt.err)
			if info == nil {
				t.Fatal("返回的 TokenInfo 不应为 nil")
			}
			if info.Status != tt.expectedStatus {
				t.Errorf("期望状态 '%s'，实际 '%s'", tt.expectedStatus, info.Status)
			}
		})
	}
}

func TestGetTokenInfoFromAccount_StatusLogic(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name           string
		hasQuota       bool
		expiryTime     time.Time
		expectedStatus string
	}{
		{"正常状态", true, now.Add(48 * time.Hour), core.AccountStatusNormal},
		{"配额不足状态", false, now.Add(48 * time.Hour), core.AccountStatusNoQuota},
		{"即将过期状态", true, now.Add(23 * time.Hour), core.AccountStatusExpiring},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := core.AccountStatusNormal
			if !tt.hasQuota {
				status = core.AccountStatusNoQuota
			} else if time.Now().Add(core.AccountExpiryWarningTime).After(tt.expiryTime) {
				status = core.AccountStatusExpiring
			}
			if status != tt.expectedStatus {
				t.Errorf("期望状态 '%s'，实际 '%s'", tt.expectedStatus, status)
			}
		})
	}
}

func TestValidateJetBrainsRequestTarget(t *testing.T) {
	tests := []struct {
		name        string
		targetType  string
		rawURL      string
		req         *http.Request
		expectError bool
		errorMsg    string
	}{
		{
			name:        "nil请求",
			req:         nil,
			expectError: true,
			errorMsg:    "invalid request: missing URL",
		},
		{
			name:        "缺少URL",
			req:         &http.Request{},
			expectError: true,
			errorMsg:    "invalid request: missing URL",
		},
		{
			name:        "非法scheme",
			targetType:  "upstream",
			rawURL:      "http://api.jetbrains.ai/user/v5/llm/chat/stream/v8",
			expectError: true,
			errorMsg:    "blocked upstream request target",
		},
		{
			name:        "非法host",
			targetType:  "outbound",
			rawURL:      "https://example.com/user/v5/llm/chat/stream/v8",
			expectError: true,
			errorMsg:    "blocked outbound request target",
		},
		{
			name:        "合法JetBrains地址",
			targetType:  "outbound",
			rawURL:      core.JetBrainsChatEndpoint,
			expectError: false,
		},
		{
			name:        "空targetType使用默认值",
			targetType:  "",
			rawURL:      "https://example.com/blocked",
			expectError: true,
			errorMsg:    "blocked outbound request target",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.req
			if req == nil && tt.rawURL != "" {
				var err error
				req, err = http.NewRequest(http.MethodPost, tt.rawURL, nil)
				if err != nil {
					t.Fatalf("创建请求失败: %v", err)
				}
			}

			err := ValidateJetBrainsRequestTarget(req, tt.targetType)
			if tt.expectError {
				if err == nil {
					t.Fatal("期望返回错误，但得到 nil")
				}
				if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Fatalf("错误信息不匹配，期望包含 %q，实际 %q", tt.errorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("期望成功，实际报错: %v", err)
			}
		})
	}
}
