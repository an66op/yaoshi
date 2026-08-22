package errors

import (
	"errors"
	"fmt"
)

// 错误类型
type ErrorType string

const (
	// ErrTypeBusiness 业务错误 - 4xx
	ErrTypeBusiness ErrorType = "business"
	// ErrTypeSystem 系统错误 - 5xx
	ErrTypeSystem ErrorType = "system"
)

// AppError 应用错误
type AppError struct {
	Type    ErrorType // 错误类型
	Code    string    // 错误代码
	Message string    // 错误消息
	Err     error     // 原始错误（用于内部日志）
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Unwrap 实现 errors.Unwrap 接口
func (e *AppError) Unwrap() error {
	return e.Err
}

// NewBusinessError 创建业务错误（4xx）
func NewBusinessError(code, message string) *AppError {
	return &AppError{
		Type:    ErrTypeBusiness,
		Code:    code,
		Message: message,
	}
}

// NewSystemError 创建系统错误（5xx）
func NewSystemError(code, message string, err error) *AppError {
	return &AppError{
		Type:    ErrTypeSystem,
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// IsBusinessError 判断是否为业务错误
func IsBusinessError(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == ErrTypeBusiness
	}
	return false
}

// IsSystemError 判断是否为系统错误
func IsSystemError(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == ErrTypeSystem
	}
	return false
}

// GetErrorType 获取错误类型
func GetErrorType(err error) ErrorType {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type
	}
	return ErrTypeSystem // 默认为系统错误
}

// GetErrorCode 获取错误代码
func GetErrorCode(err error) string {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return "UNKNOWN_ERROR"
}
