package main

import (
	"io"
	"log"
	"os"
	"sync"

	"github.com/gin-gonic/gin"
)

// ==================== Logger接口定义 ====================

type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

// ==================== AppLogger实现 ====================

// AppLogger 应用日志实现
// 支持调试模式切换和文件输出
type AppLogger struct {
	logger     *log.Logger
	debug      bool
	fileHandle *os.File     // 可能为nil
	mu         sync.RWMutex // 保护文件句柄操作
}

// NewAppLogger 创建新的日志实例
// 支持依赖注入，可传入自定义输出和配置
func NewAppLogger() *AppLogger {
	output, fileHandle := createDebugFileOutput()
	return &AppLogger{
		logger:     log.New(output, "", log.LstdFlags),
		debug:      gin.Mode() == gin.DebugMode,
		fileHandle: fileHandle,
	}
}

// NewAppLoggerWithConfig 创建带配置的日志实例
// 支持依赖注入，完全避免全局状态
func NewAppLoggerWithConfig(output io.Writer, debugMode bool) *AppLogger {
	return &AppLogger{
		logger:     log.New(output, "", log.LstdFlags),
		debug:      debugMode,
		fileHandle: nil, // 外部管理输出时不持有文件句柄
	}
}

// Debug 输出调试日志（仅在debug模式下）
func (l *AppLogger) Debug(format string, args ...any) {
	if l != nil && l.debug {
		l.logger.Printf("[DEBUG] "+format, args...)
	}
}

// Info 输出信息日志
func (l *AppLogger) Info(format string, args ...any) {
	if l != nil {
		l.logger.Printf("[INFO] "+format, args...)
	}
}

// Warn 输出警告日志
func (l *AppLogger) Warn(format string, args ...any) {
	if l != nil {
		l.logger.Printf("[WARN] "+format, args...)
	}
}

// Error 输出错误日志
func (l *AppLogger) Error(format string, args ...any) {
	if l != nil {
		l.logger.Printf("[ERROR] "+format, args...)
	}
}

// Fatal 输出致命错误日志并退出程序
func (l *AppLogger) Fatal(format string, args ...any) {
	if l != nil {
		l.logger.Fatalf("[FATAL] "+format, args...)
	} else {
		// 兜底：即使logger为nil也要输出错误
		log.Fatalf("[FATAL] "+format, args...)
	}
}

// Close 安全关闭日志文件句柄
func (l *AppLogger) Close() error {
	if l == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.fileHandle != nil {
		err := l.fileHandle.Close()
		l.fileHandle = nil
		return err
	}
	return nil
}

// ==================== 私有辅助函数 ====================

// createDebugFileOutput 创建调试文件输出，失败时优雅降级
func createDebugFileOutput() (io.Writer, *os.File) {
	debugFile := os.Getenv("DEBUG_FILE")
	if debugFile == "" {
		return os.Stdout, nil
	}

	// 验证文件路径安全性
	if len(debugFile) > MaxDebugFilePathLength {
		log.Printf("[WARN] DEBUG_FILE path too long, falling back to stdout")
		return os.Stdout, nil
	}

	// 检查路径遍历攻击（防止 ../ 等相对路径）
	cleanPath := os.Getenv("DEBUG_FILE") // 使用原始值进行清理
	if len(cleanPath) > 0 {
		// 检查是否包含路径遍历字符
		if containsPathTraversal(cleanPath) {
			log.Printf("[WARN] DEBUG_FILE contains path traversal characters, falling back to stdout")
			return os.Stdout, nil
		}
	}

	// 尝试打开文件，使用安全标志
	file, err := os.OpenFile(debugFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, FilePermissionReadWrite)
	if err != nil {
		log.Printf("[WARN] Failed to open DEBUG_FILE '%s': %v, falling back to stdout", debugFile, err)
		return os.Stdout, nil
	}

	return file, file
}

// containsPathTraversal 检查路径是否包含路径遍历字符
func containsPathTraversal(path string) bool {
	// 检查常见的路径遍历模式
	dangerousPatterns := []string{
		"..",   // 相对路径
		"./",   // 当前目录
		"../",  // 上级目录
		"..\\", // Windows 上级目录
		".\\",  // Windows 当前目录
	}

	for _, pattern := range dangerousPatterns {
		if len(path) >= len(pattern) {
			for i := 0; i <= len(path)-len(pattern); i++ {
				if path[i:i+len(pattern)] == pattern {
					return true
				}
			}
		}
	}

	return false
}

// createLogger 创建日志实例（用于依赖注入）
// 根据环境变量配置调试模式和输出位置
func createLogger() Logger {
	debugMode := os.Getenv("GIN_MODE") == "debug"
	output, fileHandle := createDebugFileOutput()

	if fileHandle != nil {
		// 如果有文件句柄，使用默认构造函数以便自动管理文件生命周期
		return NewAppLogger()
	}

	// 使用配置式构造函数
	return NewAppLoggerWithConfig(output, debugMode)
}

// ==================== 全局日志实例（向后兼容）====================
// Deprecated: appLogger 全局变量已移除，请通过依赖注入获取
//
// 迁移状态：
// ✅ main.go: 已迁移至 createLogger() + 配置注入
// ✅ server.go: 已从 ServerConfig 获取 Logger 实例
// ⚠️  其他文件: 尚未迁移，仍使用全局日志函数
//
// 重构计划：
// 1. [已完成] ServerConfig 添加 Logger 字段
// 2. [已完成] main.go 使用 createLogger() 创建实例并注入
// 3. [已完成] NewServer() 从配置获取 Logger
// 4. [待完成] 逐步迁移其他模块使用注入的 Logger
//
// 注意：全局实例仅用于向后兼容，新代码应使用依赖注入

var (
	appLogger     Logger
	loggerInitMu  sync.Mutex
	loggerInitOne sync.Once
	// 默认日志实例（空指针保护）
	defaultLogger = NewAppLoggerWithConfig(os.Stdout, false)
)

// InitializeLogger 初始化全局日志系统，必须在加载环境变量后调用
// 使用 sync.Once 确保只初始化一次，避免并发竞态条件
// Deprecated: 全局日志已废弃，请使用 createLogger() 并通过依赖注入传递
func InitializeLogger() {
	loggerInitOne.Do(func() {
		appLogger = NewAppLogger()
	})
}

// CloseLogger 全局日志清理函数，供main.go调用
func CloseLogger() error {
	if appLogger != nil {
		if l, ok := appLogger.(*AppLogger); ok {
			return l.Close()
		}
	}
	return nil
}

// ==================== 全局日志函数（空指针安全）====================
// Deprecated: 全局日志函数将在未来版本移除，请使用注入的 Logger 接口
//
// 这些函数提供空指针保护，即使未初始化也能正常工作
// 但它们隐藏了依赖关系，违反了依赖注入原则（DIP）
//
// 当前使用场景：
// - 历史遗留代码：多个模块仍在使用全局日志函数
// - 向后兼容：保留以避免破坏现有功能
//
// 迁移指南：
// 旧代码：Info("Server started")
// 新代码：logger.Info("Server started") // 通过构造函数注入 logger
//
// 请在新模块中通过构造函数注入 Logger 接口，避免使用全局函数

// Debug 全局调试日志函数（带空指针保护）
// Deprecated: 请使用注入的 Logger 接口，而非全局日志函数
func Debug(format string, args ...any) {
	if appLogger != nil {
		appLogger.Debug(format, args...)
	} else {
		defaultLogger.Debug(format, args...)
	}
}

// Info 全局信息日志函数（带空指针保护）
// Deprecated: 请使用注入的 Logger 接口，而非全局日志函数
func Info(format string, args ...any) {
	if appLogger != nil {
		appLogger.Info(format, args...)
	} else {
		defaultLogger.Info(format, args...)
	}
}

// Warn 全局警告日志函数（带空指针保护）
// Deprecated: 请使用注入的 Logger 接口，而非全局日志函数
func Warn(format string, args ...any) {
	if appLogger != nil {
		appLogger.Warn(format, args...)
	} else {
		defaultLogger.Warn(format, args...)
	}
}

// Error 全局错误日志函数（带空指针保护）
// Deprecated: 请使用注入的 Logger 接口，而非全局日志函数
func Error(format string, args ...any) {
	if appLogger != nil {
		appLogger.Error(format, args...)
	} else {
		defaultLogger.Error(format, args...)
	}
}

// Fatal 全局致命错误日志函数（带空指针保护）
// Deprecated: 请使用注入的 Logger 接口，而非全局日志函数
func Fatal(format string, args ...any) {
	if appLogger != nil {
		appLogger.Fatal(format, args...)
	} else {
		defaultLogger.Fatal(format, args...)
	}
}
