// Package observability 提供结构化日志与基础指标。
package observability

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Logger 是结构化日志包装器。
type Logger struct {
	inner *slog.Logger
}

// NewLogger 创建新的 Logger，使用文本格式输出到 os.Stderr。
func NewLogger(level string) *Logger {
	lvl := parseLevel(level)
	opts := &slog.HandlerOptions{
		Level:     lvl,
		AddSource: lvl == slog.LevelDebug,
	}
	handler := slog.NewTextHandler(os.Stderr, opts)
	return &Logger{inner: slog.New(handler)}
}

// NewLoggerWithWriter 创建输出到指定 Writer 的 Logger（用于测试）。
func NewLoggerWithWriter(w io.Writer, level string) *Logger {
	lvl := parseLevel(level)
	opts := &slog.HandlerOptions{Level: lvl}
	handler := slog.NewTextHandler(w, opts)
	return &Logger{inner: slog.New(handler)}
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "trace", "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Debug 输出 DEBUG 级别日志。
func (l *Logger) Debug(msg string, attrs ...slog.Attr) {
	l.inner.LogAttrs(context.Background(), slog.LevelDebug, msg, attrs...)
}

// Info 输出 INFO 级别日志。
func (l *Logger) Info(msg string, attrs ...slog.Attr) {
	l.inner.LogAttrs(context.Background(), slog.LevelInfo, msg, attrs...)
}

// Warn 输出 WARN 级别日志。
func (l *Logger) Warn(msg string, attrs ...slog.Attr) {
	l.inner.LogAttrs(context.Background(), slog.LevelWarn, msg, attrs...)
}

// Error 输出 ERROR 级别日志。
func (l *Logger) Error(msg string, err error, attrs ...slog.Attr) {
	allAttrs := make([]slog.Attr, 0, len(attrs)+1)
	if err != nil {
		allAttrs = append(allAttrs, slog.String("error", err.Error()))
	}
	allAttrs = append(allAttrs, attrs...)
	l.inner.LogAttrs(context.Background(), slog.LevelError, msg, allAttrs...)
}

// With 返回绑定了一组默认字段的 Logger。
func (l *Logger) With(attrs ...slog.Attr) *Logger {
	args := make([]any, 0, len(attrs)*2)
	for _, a := range attrs {
		args = append(args, a.Key, a.Value.Any())
	}
	return &Logger{inner: l.inner.With(args...)}
}

// SimpleTimer 是一个简易计时器，用于记录操作耗时。
type SimpleTimer struct {
	start time.Time
	name  string
	l     *Logger
}

// StartTimer 开始一个计时器，返回的函数在调用时记录耗时。
func (l *Logger) StartTimer(name string) func() {
	start := time.Now()
	return func() {
		d := time.Since(start)
		l.Info("timer", slog.String("name", name), slog.Duration("duration", d))
	}
}
