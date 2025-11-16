package main

import (
	"io"
	"log"
	"os"
	"sync"

	"github.com/gin-gonic/gin"
)

type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

type Logger interface {
	Debug(format string, args ...any)
	Info(format string, args ...any)
	Warn(format string, args ...any)
	Error(format string, args ...any)
	Fatal(format string, args ...any)
}

type AppLogger struct {
	logger     *log.Logger
	debug      bool
	fileHandle *os.File     // 可能为nil
	mu         sync.RWMutex // 保护文件句柄操作
}

func NewAppLogger() *AppLogger {
	output, fileHandle := createDebugFileOutput()
	return &AppLogger{
		logger:     log.New(output, "", log.LstdFlags),
		debug:      gin.Mode() == gin.DebugMode,
		fileHandle: fileHandle,
	}
}

func (l *AppLogger) Debug(format string, args ...any) {
	if l.debug {
		l.logger.Printf("[DEBUG] "+format, args...)
	}
}

func (l *AppLogger) Info(format string, args ...any) {
	l.logger.Printf("[INFO] "+format, args...)
}

func (l *AppLogger) Warn(format string, args ...any) {
	l.logger.Printf("[WARN] "+format, args...)
}

func (l *AppLogger) Error(format string, args ...any) {
	l.logger.Printf("[ERROR] "+format, args...)
}

func (l *AppLogger) Fatal(format string, args ...any) {
	l.logger.Fatalf("[FATAL] "+format, args...)
}

// 全局日志实例 - 延迟初始化
var (
	appLogger     Logger
	loggerInitMu  sync.Mutex
	loggerInitOne sync.Once
)

// Close 安全关闭日志文件句柄
func (l *AppLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.fileHandle != nil {
		err := l.fileHandle.Close()
		l.fileHandle = nil
		return err
	}
	return nil
}

// createDebugFileOutput 创建调试文件输出，失败时优雅降级
func createDebugFileOutput() (io.Writer, *os.File) {
	debugFile := os.Getenv("DEBUG_FILE")
	if debugFile == "" {
		return os.Stdout, nil
	}

	// 验证文件路径安全性
	if len(debugFile) > 260 { // 防止路径过长攻击
		log.Printf("[WARN] DEBUG_FILE path too long, falling back to stdout")
		return os.Stdout, nil
	}

	// 尝试打开文件，使用安全标志
	file, err := os.OpenFile(debugFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("[WARN] Failed to open DEBUG_FILE '%s': %v, falling back to stdout", debugFile, err)
		return os.Stdout, nil
	}

	return file, file
}

// InitializeLogger 初始化全局日志系统，必须在加载环境变量后调用
// 使用 sync.Once 确保只初始化一次，避免并发竞态条件
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

// 全局日志函数 - 直接使用全局实例
// CRITICAL: appLogger 必须在 main.go 中通过 InitializeLogger() 初始化
// 如果未初始化会 panic，这是正确的行为，能更早发现初始化顺序问题
func Debug(format string, args ...any) {
	appLogger.Debug(format, args...)
}

func Info(format string, args ...any) {
	appLogger.Info(format, args...)
}

func Warn(format string, args ...any) {
	appLogger.Warn(format, args...)
}

func Error(format string, args ...any) {
	appLogger.Error(format, args...)
}

func Fatal(format string, args ...any) {
	appLogger.Fatal(format, args...)
}
