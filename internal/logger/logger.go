// 日志系统：同时输出到 stdout 和日志文件
// 支持运行时切换 Debug 日志级别，无需重启网关
// 日志文件存储在 ~/.polaris-gateway/polaris-gateway.log
package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	
	"gopkg.in/natefinch/lumberjack.v2"
)

var logWriter io.Writer    // 多路输出 writer（stdout + 文件）
var mu sync.Mutex          // 保护 debugEnabled 和 slog handler 切换


// GetLogPath 返回日志文件的完整路径：~/.polaris-gateway/polaris-gateway.log
func GetLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./polaris-gateway.log"
	}
	
	dir := filepath.Join(home, ".polaris-gateway")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "./polaris-gateway.log"
	}
	
	return filepath.Join(dir, "polaris-gateway.log")
}

// SetDebug 运行时切换 Debug 日志级别，无需重启网关
// enabled=true 时输出 DEBUG 及以上级别日志；false 时仅输出 INFO 及以上
func SetDebug(enabled bool) {
	mu.Lock()
	defer mu.Unlock()
	level := slog.LevelInfo
	if enabled {
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	var w io.Writer
	if logWriter != nil {
		w = logWriter
	} else {
		w = os.Stdout
	}

	handler := slog.NewTextHandler(w, opts)
	slog.SetDefault(slog.New(handler))
}

// InitLogger initializes the global slog instance with lumberjack log rotation.
func InitLogger() {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	logPath := GetLogPath()
	
	// 使用 lumberjack 实现日志按大小、时间自动滚动归档
	lj := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    50,   // 每个日志文件最大 50 MB
		MaxBackups: 7,    // 最多保留 7 个旧日志文件
		MaxAge:     30,   // 旧文件最多保留 30 天
		Compress:   true, // 是否压缩旧的日志文件 (gzip)
	}

	multiWriter := io.MultiWriter(os.Stdout, lj)
	logWriter = multiWriter
	handler := slog.NewTextHandler(multiWriter, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)
}
