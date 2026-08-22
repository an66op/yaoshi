package utils

import (
	"fmt"
	"log"
	"os"
	"time"
)

// LogLevel 日志级别
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

var (
	currentLogLevel = LogLevelInfo
	logger          = log.New(os.Stdout, "", 0)
)

// SetLogLevel 设置日志级别
func SetLogLevel(level LogLevel) {
	currentLogLevel = level
}

// getTimestamp 获取时间戳
func getTimestamp() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

// logf 格式化日志输出
func logf(level string, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	logger.Printf("[%s] [%s] %s", getTimestamp(), level, message)
}

// Debug 调试日志
func Debug(format string, args ...interface{}) {
	if currentLogLevel <= LogLevelDebug {
		logf("DEBUG", format, args...)
	}
}

// Info 信息日志
func Info(format string, args ...interface{}) {
	if currentLogLevel <= LogLevelInfo {
		logf("INFO", format, args...)
	}
}

// Warn 警告日志
func Warn(format string, args ...interface{}) {
	if currentLogLevel <= LogLevelWarn {
		logf("WARN", format, args...)
	}
}

// Error 错误日志
func Error(format string, args ...interface{}) {
	if currentLogLevel <= LogLevelError {
		logf("ERROR", format, args...)
	}
}
