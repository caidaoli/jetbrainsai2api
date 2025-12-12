package main

import (
	"fmt"
)

// ==================== 错误码常量 ====================

const (
	// 配置相关错误码
	ErrCodeConfigLoadFailed     = "CONFIG_LOAD_FAILED"
	ErrCodeInvalidConfig        = "INVALID_CONFIG"
	ErrCodeNoAccountsConfigured = "NO_ACCOUNTS_CONFIGURED"
)

// ==================== AppError - 统一错误类型 ====================

// AppError 应用错误结构
// 提供统一的错误处理机制，包含错误码、消息和底层原因
type AppError struct {
	Code    string // 错误码（用于客户端识别错误类型）
	Message string // 人类可读的错误消息
	Cause   error  // 底层原因（可选）
}

// Error 实现 error 接口
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap 支持 Go 1.13+ 的 errors.Unwrap
func (e *AppError) Unwrap() error {
	return e.Cause
}

// ==================== 错误构造函数 ====================

// NewAppError 创建新的应用错误
func NewAppError(code, message string, cause error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// NewAppErrorf 创建新的应用错误（带格式化消息）
func NewAppErrorf(code string, cause error, format string, args ...any) *AppError {
	return &AppError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
		Cause:   cause,
	}
}

// ==================== 配置错误 ====================

// ErrConfigLoadFailed 配置加载失败错误
func ErrConfigLoadFailed(configType string, cause error) *AppError {
	return NewAppErrorf(
		ErrCodeConfigLoadFailed,
		cause,
		"Failed to load %s configuration",
		configType,
	)
}

// ErrInvalidConfig 无效配置错误
func ErrInvalidConfig(field string, reason string) *AppError {
	return NewAppErrorf(
		ErrCodeInvalidConfig,
		nil,
		"Invalid configuration for %s: %s",
		field, reason,
	)
}

// ErrNoAccountsConfigured 未配置账户错误
func ErrNoAccountsConfigured() *AppError {
	return NewAppError(
		ErrCodeNoAccountsConfigured,
		"No JetBrains accounts configured",
		nil,
	)
}
