package main

import (
	"os"
	"strings"
	"testing"
)

// TestParseEnvList 测试环境变量列表解析
func TestParseEnvList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "空字符串",
			input:    "",
			expected: nil,
		},
		{
			name:     "单个值",
			input:    "value1",
			expected: []string{"value1"},
		},
		{
			name:     "多个值",
			input:    "value1,value2,value3",
			expected: []string{"value1", "value2", "value3"},
		},
		{
			name:     "值带空格",
			input:    "value1, value2 , value3",
			expected: []string{"value1", "value2", "value3"},
		},
		{
			name:     "包含空值",
			input:    "value1,,value2",
			expected: []string{"value1", "value2"},
		},
		{
			name:     "末尾逗号",
			input:    "value1,value2,",
			expected: []string{"value1", "value2"},
		},
		{
			name:     "全空格值",
			input:    "  ,  ,  ",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseEnvList(tt.input)

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

// TestExtractTextContent 测试文本内容提取
func TestExtractTextContent(t *testing.T) {
	tests := []struct {
		name     string
		content  any
		expected string
	}{
		{
			name:     "nil内容",
			content:  nil,
			expected: "",
		},
		{
			name:     "字符串内容",
			content:  "Hello World",
			expected: "Hello World",
		},
		{
			name:     "空字符串",
			content:  "",
			expected: "",
		},
		{
			name: "单个text块",
			content: []any{
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "单个文本",
				},
			},
			expected: "单个文本",
		},
		{
			name: "多个text块",
			content: []any{
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "第一部分",
				},
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "第二部分",
				},
			},
			expected: "第一部分 第二部分",
		},
		{
			name: "混合类型块",
			content: []any{
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "文本",
				},
				map[string]any{
					"type": "image",
					"data": "base64data",
				},
				map[string]any{
					"type": ContentBlockTypeText,
					"text": "更多文本",
				},
			},
			expected: "文本 更多文本",
		},
		{
			name:     "数字类型",
			content:  123,
			expected: "",
		},
		{
			name:     "空数组",
			content:  []any{},
			expected: "",
		},
		{
			name: "text块缺少text字段",
			content: []any{
				map[string]any{
					"type": ContentBlockTypeText,
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTextContent(tt.content)
			if result != tt.expected {
				t.Errorf("期望 '%s'，实际 '%s'", tt.expected, result)
			}
		})
	}
}

// TestTruncateString 测试字符串截断
func TestTruncateString(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		prefixLen   int
		suffixLen   int
		replacement string
		expected    string
	}{
		{
			name:        "短字符串不截断",
			input:       "short",
			prefixLen:   3,
			suffixLen:   3,
			replacement: "...",
			expected:    "short",
		},
		{
			name:        "正好等于阈值",
			input:       "123456",
			prefixLen:   3,
			suffixLen:   3,
			replacement: "...",
			expected:    "123456",
		},
		{
			name:        "超过阈值截断",
			input:       "1234567890",
			prefixLen:   3,
			suffixLen:   3,
			replacement: "...",
			expected:    "123...890",
		},
		{
			name:        "自定义替换符",
			input:       "abcdefghij",
			prefixLen:   2,
			suffixLen:   2,
			replacement: "***",
			expected:    "ab***ij",
		},
		{
			name:        "只保留后缀",
			input:       "1234567890",
			prefixLen:   0,
			suffixLen:   4,
			replacement: "...",
			expected:    "...7890",
		},
		{
			name:        "只保留前缀",
			input:       "1234567890",
			prefixLen:   4,
			suffixLen:   0,
			replacement: "...",
			expected:    "1234...",
		},
		{
			name:        "Token格式",
			input:       "eyJhbGciOiJSUzI1NiJ9.eyJhdWQiOiJhaS1hdXRob3JpemF0aW9uLXNlcnZlciIsImV4cCI6MTcxOTI0OTI1OCwiYWNjb3VudElkIjoiYjg1MjA0NjEtNTNlOS00YTAzLWI1MmYtZjE0MDVhNGRlMzA5IiwicmVhbG0iOiJqYiIsImlzcyI6Imh0dHBzOi8vb2F1dGguamV0YnJhaW5zLmNvbS9vYXV0aC9va3RhIiwiaWF0IjoxNzE5MTYyODU4LCJ0b2tlbklkIjoiYWM0NTJmYmMtY2ZmNS00YjI2LTg2YzgtZjlkMzE2ZGUwYzI3IiwiYWNjZXNzTGljZW5zZUlkcyI6W119.SIGNATURE",
			prefixLen:   0,
			suffixLen:   6,
			replacement: "Token ...",
			expected:    "Token ...NATURE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateString(tt.input, tt.prefixLen, tt.suffixLen, tt.replacement)
			if result != tt.expected {
				t.Errorf("期望 '%s'，实际 '%s'", tt.expected, result)
			}
		})
	}
}

// TestGenerateID 测试ID生成
func TestGenerateID(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
	}{
		{
			name:   "chatcmpl前缀",
			prefix: "chatcmpl-",
		},
		{
			name:   "msg前缀",
			prefix: "msg_",
		},
		{
			name:   "空前缀",
			prefix: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := generateID(tt.prefix)

			// 验证前缀
			if !strings.HasPrefix(id, tt.prefix) {
				t.Errorf("ID应以 '%s' 为前缀，实际: '%s'", tt.prefix, id)
			}

			// 验证长度
			if len(id) < len(tt.prefix)+10 {
				t.Errorf("ID长度过短: %s", id)
			}

			// 验证唯一性（注意：纳秒级时间戳在极快调用下可能相同，这里只验证格式）
			// 实际使用中连续调用间隔通常大于1纳秒
		})
	}
}

// TestGetTokenDisplayName 测试Token显示名生成
func TestGetTokenDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		account  *JetbrainsAccount
		contains string
	}{
		{
			name: "有JWT",
			account: &JetbrainsAccount{
				JWT: "eyJhbGciOiJSUzI1NiJ9.longtoken",
			},
			contains: "Token ...",
		},
		{
			name: "有LicenseID无JWT",
			account: &JetbrainsAccount{
				LicenseID: "license-123456789",
			},
			contains: "Token ...",
		},
		{
			name:     "无JWT无LicenseID",
			account:  &JetbrainsAccount{},
			contains: "Unknown",
		},
		{
			name: "JWT优先于LicenseID",
			account: &JetbrainsAccount{
				JWT:       "jwt-token-12345",
				LicenseID: "license-67890",
			},
			contains: "Token ...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getTokenDisplayName(tt.account)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("期望包含 '%s'，实际 '%s'", tt.contains, result)
			}
		})
	}
}

// TestGetLicenseDisplayName 测试License显示名生成
func TestGetLicenseDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		account  *JetbrainsAccount
		expected string
	}{
		{
			name: "有Authorization",
			account: &JetbrainsAccount{
				Authorization: "Bearer token123456",
			},
			expected: "Bea*456",
		},
		{
			name:     "无Authorization",
			account:  &JetbrainsAccount{},
			expected: "Unknown",
		},
		{
			name: "短Authorization",
			account: &JetbrainsAccount{
				Authorization: "ABC",
			},
			expected: "ABC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getLicenseDisplayName(tt.account)
			if result != tt.expected {
				t.Errorf("期望 '%s'，实际 '%s'", tt.expected, result)
			}
		})
	}
}

// TestGetEnvWithDefault 测试环境变量获取
func TestGetEnvWithDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		setValue     string
		defaultValue string
		expected     string
		setEnv       bool
	}{
		{
			name:         "使用默认值",
			key:          "TEST_ENV_NOT_SET_12345",
			defaultValue: "default_value",
			expected:     "default_value",
			setEnv:       false,
		},
		{
			name:         "使用环境变量值",
			key:          "TEST_ENV_SET_12345",
			setValue:     "actual_value",
			defaultValue: "default_value",
			expected:     "actual_value",
			setEnv:       true,
		},
		{
			name:         "空环境变量使用默认值",
			key:          "TEST_ENV_EMPTY_12345",
			setValue:     "",
			defaultValue: "default_value",
			expected:     "default_value",
			setEnv:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 清理可能的旧环境变量
			os.Unsetenv(tt.key)

			if tt.setEnv {
				os.Setenv(tt.key, tt.setValue)
				defer os.Unsetenv(tt.key)
			}

			result := getEnvWithDefault(tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("期望 '%s'，实际 '%s'", tt.expected, result)
			}
		})
	}
}
