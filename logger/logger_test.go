package logger

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"CompeManage_backend/config"

	"github.com/stretchr/testify/assert"
)

// 记录经过包装函数的日志级别（回归 Debug 被写成 Error 的 bug）
type recordHandler struct {
	mu   sync.Mutex
	recs []slog.Record
}

func (h *recordHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *recordHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.recs = append(h.recs, r)
	return nil
}
func (h *recordHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *recordHandler) WithGroup(_ string) slog.Handler      { return h }

func TestSlogWrappers_UseCorrectLevels(t *testing.T) {
	h := &recordHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })

	Info("i")
	Error("e")
	Debug("d")

	h.mu.Lock()
	defer h.mu.Unlock()
	assert.Equal(t, 3, len(h.recs))
	assert.Equal(t, slog.LevelInfo, h.recs[0].Level)
	assert.Equal(t, slog.LevelError, h.recs[1].Level)
	assert.Equal(t, slog.LevelDebug, h.recs[2].Level) // 修复前这里是 Error
}

func TestAsyncWriter_Consume(t *testing.T) {
	var buf bytes.Buffer
	aw := NewAsyncWriter(&buf, 2)
	aw.Write([]byte("hello "))
	aw.Write([]byte("world"))
	// 异步落盘
	assert.Eventually(t, func() bool {
		return bytes.Contains(buf.Bytes(), []byte("hello world"))
	}, time.Second, 5*time.Millisecond)
}

func TestSetupLogger_AllLevelBranches(t *testing.T) {
	origApp := config.AppConfig
	prev := slog.Default()
	t.Cleanup(func() {
		config.AppConfig = origApp
		slog.SetDefault(prev)
	})

	for _, level := range []string{"debug", "info", "warn", "error", "unknown"} {
		config.AppConfig = &config.Config{Logger: config.LoggerConfig{
			Level:    level,
			Filename: filepath.Join(os.TempDir(), fmt.Sprintf("logger_test_%d.log", time.Now().UnixNano())),
			ShowLine: true,
		}}
		SetupLogger()
		slog.Info("smoke-" + level)
	}
}

type gatedWriter struct {
	gate chan struct{}
	mu   sync.Mutex
	buf  []byte
}

func (g *gatedWriter) Write(p []byte) (int, error) {
	<-g.gate
	g.mu.Lock()
	defer g.mu.Unlock()
	g.buf = append(g.buf, p...)
	return len(p), nil
}

func (g *gatedWriter) String() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return string(g.buf)
}

// 队列满时丢弃旧消息保护业务协程（回归）
func TestAsyncWriter_DropsWhenQueueFull(t *testing.T) {
	gw := &gatedWriter{gate: make(chan struct{})}
	aw := NewAsyncWriter(gw, 1)

	aw.Write([]byte("a"))
	time.Sleep(20 * time.Millisecond) // 消费者取走 a 并阻塞
	aw.Write([]byte("b"))
	aw.Write([]byte("c")) // 队列已满 → 丢弃

	close(gw.gate)
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, "ab", gw.String())
}
