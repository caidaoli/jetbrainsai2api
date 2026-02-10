package log

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

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

func TestAppLogger_Debug(t *testing.T) {
	tests := []struct {
		name      string
		debugMode bool
		message   string
		expectLog bool
	}{
		{"调试模式下输出", true, "测试调试消息", true},
		{"非调试模式下不输出", false, "这条不应该出现", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := NewAppLoggerWithConfig(&buf, tt.debugMode)
			logger.Debug(tt.message)
			output := buf.String()
			hasLog := strings.Contains(output, tt.message)
			if hasLog != tt.expectLog {
				t.Errorf("期望有日志输出=%v，实际=%v", tt.expectLog, hasLog)
			}
			if tt.expectLog && !strings.Contains(output, "[DEBUG]") {
				t.Error("调试日志应包含 [DEBUG] 前缀")
			}
		})
	}
}

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

func TestAppLogger_NilSafety(t *testing.T) {
	var logger *AppLogger = nil
	logger.Debug("不应panic")
	logger.Info("不应panic")
	logger.Warn("不应panic")
	logger.Error("不应panic")
}

func TestAppLogger_Close(t *testing.T) {
	tests := []struct {
		name string
		fn   func() (*AppLogger, error)
	}{
		{"关闭无文件句柄的日志", func() (*AppLogger, error) {
			var buf bytes.Buffer
			logger := NewAppLoggerWithConfig(&buf, false)
			return logger, logger.Close()
		}},
		{"关闭nil日志", func() (*AppLogger, error) {
			var logger *AppLogger = nil
			return logger, logger.Close()
		}},
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

func TestContainsPathTraversal(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"正常路径", "/var/log/app.log", false},
		{"包含..", "/var/../etc/passwd", true},
		{"包含../", "../secret.txt", true},
		{"包含./", "./local.log", false},
		{"Windows上级目录", "..\\config.ini", true},
		{"空路径", "", false},
		{"文件名包含点", "/var/log/app.2024.log", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsPathTraversal(tt.path)
			if result != tt.expected {
				t.Errorf("containsPathTraversal(%q) = %v，期望 %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIsDebug(t *testing.T) {
	// IsDebug() in internal/log reads os.Getenv("GIN_MODE") directly
	originalMode := os.Getenv("GIN_MODE")
	defer func() {
		if originalMode == "" {
			_ = os.Unsetenv("GIN_MODE")
		} else {
			_ = os.Setenv("GIN_MODE", originalMode)
		}
	}()

	tests := []struct {
		name     string
		ginMode  string
		expected bool
	}{
		{"debug模式", "debug", true},
		{"release模式", "release", false},
		{"test模式", "test", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Setenv("GIN_MODE", tt.ginMode)
			result := IsDebug()
			if result != tt.expected {
				t.Errorf("IsDebug() = %v，期望 %v (GIN_MODE=%s)", result, tt.expected, tt.ginMode)
			}
		})
	}
}

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
