package main

import (
	"fmt"
	"log"
	"os"
	"time"
)

// LogLevel 日志级别
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

var (
	currentLogLevel = DEBUG // 默认 DEBUG 级别
	logger          = log.New(os.Stdout, "", 0)
)

// SetLogLevel 设置日志级别
func SetLogLevel(level LogLevel) {
	currentLogLevel = level
}

// LogDebug 调试日志
func LogDebug(format string, args ...interface{}) {
	if currentLogLevel <= DEBUG {
		timestamp := time.Now().Format("15:04:05.000")
		msg := fmt.Sprintf(format, args...)
		logger.Printf("[%s] [DEBUG] %s", timestamp, msg)
	}
}

// LogInfo 信息日志
func LogInfo(format string, args ...interface{}) {
	if currentLogLevel <= INFO {
		timestamp := time.Now().Format("15:04:05.000")
		msg := fmt.Sprintf(format, args...)
		logger.Printf("[%s] [INFO] %s", timestamp, msg)
	}
}

// LogWarn 警告日志
func LogWarn(format string, args ...interface{}) {
	if currentLogLevel <= WARN {
		timestamp := time.Now().Format("15:04:05.000")
		msg := fmt.Sprintf(format, args...)
		logger.Printf("[%s] [WARN] %s", timestamp, msg)
	}
}

// LogError 错误日志
func LogError(format string, args ...interface{}) {
	if currentLogLevel <= ERROR {
		timestamp := time.Now().Format("15:04:05.000")
		msg := fmt.Sprintf(format, args...)
		logger.Printf("[%s] [ERROR] %s", timestamp, msg)
	}
}
