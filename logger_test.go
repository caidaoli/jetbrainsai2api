package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestNewAppLoggerWithConfig 测试创建带配置的日志实例
func TestNewAppLoggerWithConfig(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAppLoggerWithConfig(&buf, true)

	if logger == nil {
		t.Fatal("日志实例不应为nil")
	}

	if !logger.debug {
		t.Error("调试模式应为true")
	}

	if logger.fileHandle != nil {
		t.Error("外部输出时不应持有文件句柄")
	}
}

// TestAppLogger_Debug 测试调试日志
func TestAppLogger_Debug(t *testing.T) {
	tests := []struct {
		name      string
		debugMode bool
		message   string
		expectLog bool
	}{
		{
			name:      "调试模式下输出",
			debugMode: true,
			message:   "测试调试消息",
			expectLog: true,
		},
		{
			name:      "非调试模式下不输出",
			debugMode: false,
			message:   "这条不应该出现",
			expectLog: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := NewAppLoggerWithConfig(&buf, tt.debugMode)

			logger.Debug(tt.message)

			output := buf.String()
			hasLog := strings.Contains(output, tt.message)

			if hasLog != tt.expectLog {
				t.Errorf("期望有日志输出=%v，实际=%v，输出内容：'%s'",
					tt.expectLog, hasLog, output)
			}

			if tt.expectLog && !strings.Contains(output, "[DEBUG]") {
				t.Error("调试日志应包含 [DEBUG] 前缀")
			}
		})
	}
}

// TestAppLogger_Info 测试信息日志
func TestAppLogger_Info(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAppLoggerWithConfig(&buf, false)

	logger.Info("测试信息: %s", "参数值")

	output := buf.String()
	if !strings.Contains(output, "[INFO]") {
		t.Error("信息日志应包含 [INFO] 前缀")
	}
	if !strings.Contains(output, "测试信息: 参数值") {
		t.Error("日志应包含格式化后的消息")
	}
}

// TestAppLogger_Warn 测试警告日志
func TestAppLogger_Warn(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAppLoggerWithConfig(&buf, false)

	logger.Warn("测试警告: %d", 123)

	output := buf.String()
	if !strings.Contains(output, "[WARN]") {
		t.Error("警告日志应包含 [WARN] 前缀")
	}
	if !strings.Contains(output, "测试警告: 123") {
		t.Error("日志应包含格式化后的消息")
	}
}

// TestAppLogger_Error 测试错误日志
func TestAppLogger_Error(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAppLoggerWithConfig(&buf, false)

	logger.Error("测试错误: %v", "详细信息")

	output := buf.String()
	if !strings.Contains(output, "[ERROR]") {
		t.Error("错误日志应包含 [ERROR] 前缀")
	}
	if !strings.Contains(output, "测试错误: 详细信息") {
		t.Error("日志应包含格式化后的消息")
	}
}

// TestAppLogger_NilSafety 测试nil日志实例的安全性
func TestAppLogger_NilSafety(t *testing.T) {
	var logger *AppLogger = nil

	// 这些调用不应该panic
	logger.Debug("不应panic")
	logger.Info("不应panic")
	logger.Warn("不应panic")
	logger.Error("不应panic")
}

// TestAppLogger_Close 测试关闭日志
func TestAppLogger_Close(t *testing.T) {
	tests := []struct {
		name string
		fn   func() (*AppLogger, error)
	}{
		{
			name: "关闭无文件句柄的日志",
			fn: func() (*AppLogger, error) {
				var buf bytes.Buffer
				logger := NewAppLoggerWithConfig(&buf, false)
				return logger, logger.Close()
			},
		},
		{
			name: "关闭nil日志",
			fn: func() (*AppLogger, error) {
				var logger *AppLogger = nil
				return logger, logger.Close()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.fn()
			if err != nil {
				t.Errorf("关闭日志不应返回错误: %v", err)
			}
		})
	}
}

// TestContainsPathTraversal 测试路径遍历检测
func TestContainsPathTraversal(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "正常路径",
			path:     "/var/log/app.log",
			expected: false,
		},
		{
			name:     "包含..",
			path:     "/var/../etc/passwd",
			expected: true,
		},
		{
			name:     "包含../",
			path:     "../secret.txt",
			expected: true,
		},
		{
			name:     "包含./",
			path:     "./local.log",
			expected: true,
		},
		{
			name:     "Windows上级目录",
			path:     "..\\config.ini",
			expected: true,
		},
		{
			name:     "Windows当前目录",
			path:     ".\\data.txt",
			expected: true,
		},
		{
			name:     "空路径",
			path:     "",
			expected: false,
		},
		{
			name:     "单个点",
			path:     "/path/to/file.txt",
			expected: false,
		},
		{
			name:     "文件名包含点",
			path:     "/var/log/app.2024.log",
			expected: false,
		},
		{
			name:     "中间包含..",
			path:     "/var/log/../etc/passwd",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsPathTraversal(tt.path)
			if result != tt.expected {
				t.Errorf("containsPathTraversal(%q) = %v，期望 %v",
					tt.path, result, tt.expected)
			}
		})
	}
}

// TestIsDebug 测试调试模式检测
func TestIsDebug(t *testing.T) {
	// 保存原始值
	originalMode := gin.Mode()
	defer gin.SetMode(originalMode)

	tests := []struct {
		name     string
		ginMode  string
		expected bool
	}{
		{
			name:     "debug模式",
			ginMode:  gin.DebugMode,
			expected: true,
		},
		{
			name:     "release模式",
			ginMode:  gin.ReleaseMode,
			expected: false,
		},
		{
			name:     "test模式",
			ginMode:  gin.TestMode,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(tt.ginMode)
			result := IsDebug()
			if result != tt.expected {
				t.Errorf("IsDebug() = %v，期望 %v (GIN_MODE=%s)",
					result, tt.expected, tt.ginMode)
			}
		})
	}
}

// TestAppLogger_DebugWithFormat 测试格式化调试日志
func TestAppLogger_DebugWithFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAppLoggerWithConfig(&buf, true)

	logger.Debug("用户 %s 登录，ID=%d", "张三", 12345)

	output := buf.String()
	if !strings.Contains(output, "用户 张三 登录，ID=12345") {
		t.Errorf("格式化日志内容不正确: %s", output)
	}
}

// TestAppLogger_MultipleWrites 测试多次写入
func TestAppLogger_MultipleWrites(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAppLoggerWithConfig(&buf, true)

	logger.Debug("第一条")
	logger.Info("第二条")
	logger.Warn("第三条")
	logger.Error("第四条")

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 4 {
		t.Errorf("期望4行日志，实际 %d 行", len(lines))
	}
}
