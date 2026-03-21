package logger

import (
	"CompeManage_backend/config"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/natefinch/lumberjack"
)

// AsyncWriter 实现 io.Writer 接口，将写入操作变为异步
type AsyncWriter struct {
	writer io.Writer
	queue  chan []byte
}

func NewAsyncWriter(w io.Writer, bufferSize int) *AsyncWriter {
	aw := &AsyncWriter{
		writer: w,
		queue:  make(chan []byte, bufferSize),
	}
	// 启动后台协程消费日志
	go aw.consume()
	return aw
}

// Write 实现 io.Writer，业务协程调用 slog 时会进入这里
func (aw *AsyncWriter) Write(p []byte) (int, error) {
	// 注意：由于 p 是切片缓冲区，异步处理必须拷贝一份，否则会被后续日志覆盖
	data := make([]byte, len(p))
	copy(data, p)

	// 非阻塞写入：如果队列满了则丢弃（保护业务性能），或者阻塞等待
	select {
	case aw.queue <- data:
	default:
		// 队列满了，可以在这里输出到标准错误，或者直接丢弃
		os.Stderr.WriteString("log queue full, dropping message\n")
	}
	return len(p), nil
}

func (aw *AsyncWriter) consume() {
	for data := range aw.queue {
		aw.writer.Write(data)
	}
}

func SetupLogger() {
	cfg := config.AppConfig.Logger
	var level slog.Level
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	lumberjackLogger := &lumberjack.Logger{
		Filename:   cfg.Filename,
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   cfg.Compress,
	}
	asyncFileWriter := NewAsyncWriter(lumberjackLogger, 10000)
	multiWriter := io.MultiWriter(os.Stdout, asyncFileWriter)
	handler := slog.NewJSONHandler(multiWriter, &slog.HandlerOptions{Level: level, AddSource: cfg.ShowLine})
	slog.SetDefault(slog.New(handler))
}

// Info 封装，支持更强的语义
func Info(msg string, args ...any) {
	slog.Default().Info(msg, args...)
}

// Error 封装，自动处理 error 对象
func Error(msg string, args ...any) {
	slog.Default().Error(msg, args...)
}

func Debug(msg string, args ...any) {
	slog.Default().Error(msg, args...)
}

func Fatal(msg string, args ...any) {
	slog.Default().Error(msg, args...)
	os.Exit(1)
}
