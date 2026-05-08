package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

var LogFile *os.File
var logWriter io.Writer
var mu sync.Mutex
var debugEnabled bool

func getLogPath() string {
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

// InitLogger initializes the global slog instance.
func InitLogger() {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	logPath := getLogPath()
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0666)
	if err == nil {
		LogFile = f
		multiWriter := io.MultiWriter(os.Stdout, f)
		logWriter = multiWriter
		handler := slog.NewTextHandler(multiWriter, opts)
		logger := slog.New(handler)
		slog.SetDefault(logger)
	} else {
		handler := slog.NewTextHandler(os.Stdout, opts)
		logger := slog.New(handler)
		slog.SetDefault(logger)
	}
}
