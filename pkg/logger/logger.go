// 日志系统：同时输出到 stdout 和日志文件
// 支持运行时切换 Debug 日志级别，无需重启网关
// 日志文件存储在 ~/.polarisagi-hermes/polarisagi-hermes.log
package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/polarisagi/polarisagi-hermes/internal/config"
	"gopkg.in/natefinch/lumberjack.v2"
)

var logWriter io.Writer // 多路输出 writer（stdout + 文件）
var mu sync.Mutex       // 保护 debugEnabled 和 slog handler 切换
var debugEnabled bool   // 记录当前 debug 状态

// GetLogPath 返回日志文件的完整路径
func GetLogPath() string {
	workDir := config.GlobalConfig.Server.WorkDir
	if workDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "./polarisagi-hermes.log"
		}
		workDir = filepath.Join(home, ".polarisagi-hermes")
	}

	if err := os.MkdirAll(workDir, 0755); err != nil {
		return "./polarisagi-hermes.log"
	}

	return filepath.Join(workDir, "polarisagi-hermes.log")
}

// SetDebug 运行时切换 Debug 日志级别，无需重启网关
// enabled=true 时输出 DEBUG 及以上级别日志；false 时仅输出 INFO 及以上
func SetDebug(enabled bool) {
	mu.Lock()
	defer mu.Unlock()
	debugEnabled = enabled
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

// IsDebugEnabled 返回当前是否开启了 debug 日志
func IsDebugEnabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return debugEnabled
}

// InitLogger initializes the global slog instance with lumberjack log rotation.
func InitLogger() {
	debugEnabled = true // 默认开启 debug 日志
	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug,
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
	handler := slog.NewJSONHandler(multiWriter, opts) // 改用 JSON 格式以方便后续解析
	logger := slog.New(handler)
	slog.SetDefault(logger)
}
