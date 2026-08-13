package common

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Level 日志级别。
type Level int

// 日志级别定义。
const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// 级别显示名。
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

// logger 是线程安全的简易日志器。
// 注意：普通日志仅记录操作摘要，绝不记录密码/密钥；敏感审计由 AuditLogger 加密保存。
type logger struct {
	mu     sync.Mutex
	out    io.Writer
	file   *os.File
	level  Level
	closed bool
}

var std = &logger{out: os.Stdout, level: LevelInfo}

// InitLog 初始化日志输出到文件（同时保留控制台输出）。
func InitLog(dir string) error {
	if dir == "" {
		return nil
	}
	if err := EnsureDir(dir); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "steggo.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	std.mu.Lock()
	std.file = f
	std.mu.Unlock()
	return nil
}

// SetLevel 设置全局日志级别。
func SetLevel(l Level) {
	std.mu.Lock()
	std.level = l
	std.mu.Unlock()
}

// CloseLog 关闭日志文件。
func CloseLog() error {
	std.mu.Lock()
	defer std.mu.Unlock()
	if std.closed {
		return nil
	}
	std.closed = true
	if std.file != nil {
		return std.file.Close()
	}
	return nil
}

func (l *logger) write(lv Level, format string, args ...any) {
	if lv < l.level {
		return
	}
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("[%s] [%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), lv.String(), strings.TrimSpace(msg))
	l.mu.Lock()
	if l.out != nil {
		_, _ = io.WriteString(l.out, line)
	}
	if l.file != nil {
		_, _ = l.file.WriteString(line)
	}
	l.mu.Unlock()
}

// Debugf 输出调试日志。
func Debugf(format string, args ...any) { std.write(LevelDebug, format, args...) }

// Infof 输出信息日志。
func Infof(format string, args ...any) { std.write(LevelInfo, format, args...) }

// Warnf 输出警告日志。
func Warnf(format string, args ...any) { std.write(LevelWarn, format, args...) }

// Errorf 输出错误日志。
func Errorf(format string, args ...any) { std.write(LevelError, format, args...) }
