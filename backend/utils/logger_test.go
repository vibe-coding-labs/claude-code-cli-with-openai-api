package utils

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// captureStdout 临时把 os.Stdout 重定向到内存缓冲区，用于断言 Logger 写到
// 控制台的内容；log() 内部直接调用 fmt.Print* 而非注入的 io.Writer，
// 只能通过这种方式观察输出。
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = w

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		io.Copy(&buf, r)
		close(done)
	}()

	fn()

	os.Stdout = orig
	w.Close()
	<-done
	return buf.String()
}

func newTestLogger() *Logger {
	return &Logger{level: DEBUG}
}

func TestGetLogger_Singleton(t *testing.T) {
	l1 := GetLogger()
	l2 := GetLogger()
	if l1 != l2 {
		t.Errorf("expected GetLogger() to return the same singleton instance")
	}
}

func TestSetLevelFromString(t *testing.T) {
	tests := []struct {
		in   string
		want LogLevel
	}{
		{"debug", DEBUG},
		{"DEBUG", DEBUG},
		{"info", INFO},
		{"INFO", INFO},
		{"warn", WARN},
		{"WARN", WARN},
		{"error", ERROR},
		{"ERROR", ERROR},
		{"garbage", INFO},
		{"", INFO},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			l := newTestLogger()
			l.SetLevelFromString(tt.in)
			if l.level != tt.want {
				t.Errorf("SetLevelFromString(%q) level = %v, want %v", tt.in, l.level, tt.want)
			}
		})
	}
}

func TestSetPrefix(t *testing.T) {
	l := newTestLogger()
	l.SetPrefix("worker-1")
	if l.prefix != "worker-1" {
		t.Errorf("prefix = %q, want worker-1", l.prefix)
	}
}

func TestSetLogFile_SuccessWritesToFile(t *testing.T) {
	l := newTestLogger()
	path := filepath.Join(t.TempDir(), "sub", "app.log")

	if err := l.SetLogFile(path); err != nil {
		t.Fatalf("SetLogFile() error = %v", err)
	}
	defer l.Close()

	l.Info("hello %s", "world")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(data), "hello world") {
		t.Errorf("log file content = %q, want it to contain %q", data, "hello world")
	}
	if !strings.Contains(string(data), "[INFO]") {
		t.Errorf("log file content = %q, want it to contain level tag", data)
	}
}

func TestSetLogFile_WritesPrefixToFileEntry(t *testing.T) {
	l := newTestLogger()
	l.SetPrefix("worker-1")
	path := filepath.Join(t.TempDir(), "app.log")

	if err := l.SetLogFile(path); err != nil {
		t.Fatalf("SetLogFile() error = %v", err)
	}
	defer l.Close()

	l.Info("hello")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(data), "[worker-1]") {
		t.Errorf("log file content = %q, want it to contain prefix tag", data)
	}
}

func TestSetLogFile_ClosesPreviousFile(t *testing.T) {
	l := newTestLogger()
	dir := t.TempDir()

	if err := l.SetLogFile(filepath.Join(dir, "first.log")); err != nil {
		t.Fatalf("SetLogFile(first) error = %v", err)
	}
	firstFile := l.logFile

	if err := l.SetLogFile(filepath.Join(dir, "second.log")); err != nil {
		t.Fatalf("SetLogFile(second) error = %v", err)
	}
	defer l.Close()

	// 写入已关闭的第一个文件描述符应返回错误，证明它确实被 Close 了
	if _, err := firstFile.Write([]byte("x")); err == nil {
		t.Errorf("expected previous log file to be closed")
	}
}

func TestSetLogFile_MkdirFailsWhenParentIsFile(t *testing.T) {
	l := newTestLogger()
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatalf("failed to write blocker file: %v", err)
	}

	if err := l.SetLogFile(filepath.Join(blocker, "sub", "app.log")); err == nil {
		t.Errorf("expected error when log directory cannot be created")
	}
}

func TestSetLogFile_OpenFailsWhenPathIsDirectory(t *testing.T) {
	l := newTestLogger()
	dir := t.TempDir()

	if err := l.SetLogFile(dir); err == nil {
		t.Errorf("expected error when log file path is itself a directory")
	}
}

func TestClose_IdempotentWithoutLogFile(t *testing.T) {
	l := newTestLogger()
	// 从未调用过 SetLogFile，Close 不应 panic
	l.Close()
	l.Close()
}

func TestLog_LevelFiltering(t *testing.T) {
	l := newTestLogger()
	l.SetLevel(WARN)

	out := captureStdout(t, func() {
		l.Debug("debug message")
		l.Info("info message")
		l.Warn("warn message")
		l.Error("error message")
	})

	if strings.Contains(out, "debug message") || strings.Contains(out, "info message") {
		t.Errorf("expected DEBUG/INFO to be filtered out at WARN level, got: %q", out)
	}
	if !strings.Contains(out, "warn message") || !strings.Contains(out, "error message") {
		t.Errorf("expected WARN/ERROR to be printed, got: %q", out)
	}
}

func TestLog_IncludesPrefix(t *testing.T) {
	l := newTestLogger()
	l.SetPrefix("myprefix")

	out := captureStdout(t, func() {
		l.Info("hi")
	})
	if !strings.Contains(out, "myprefix") {
		t.Errorf("expected output to contain prefix, got: %q", out)
	}
}

func TestErrorWithCause(t *testing.T) {
	l := newTestLogger()

	t.Run("with non-nil error appends cause", func(t *testing.T) {
		out := captureStdout(t, func() {
			l.ErrorWithCause(io.EOF, "operation %s failed", "read")
		})
		if !strings.Contains(out, "operation read failed") || !strings.Contains(out, "error: EOF") {
			t.Errorf("unexpected output: %q", out)
		}
	})

	t.Run("with nil error omits cause", func(t *testing.T) {
		out := captureStdout(t, func() {
			l.ErrorWithCause(nil, "operation %s failed", "write")
		})
		if !strings.Contains(out, "operation write failed") {
			t.Errorf("unexpected output: %q", out)
		}
		if strings.Contains(out, "error:") {
			t.Errorf("did not expect 'error:' cause suffix when err is nil, got: %q", out)
		}
	})
}

func TestLogUpstreamError(t *testing.T) {
	l := newTestLogger()

	t.Run("short body is not truncated", func(t *testing.T) {
		out := captureStdout(t, func() {
			l.LogUpstreamError("openai", "/v1/chat/completions", "gpt-4o", 500, "short body")
		})
		if !strings.Contains(out, "short body") || strings.Contains(out, "truncated") {
			t.Errorf("unexpected output: %q", out)
		}
	})

	t.Run("long body is truncated at 500 chars", func(t *testing.T) {
		longBody := strings.Repeat("x", 600)
		out := captureStdout(t, func() {
			l.LogUpstreamError("openai", "/v1/chat/completions", "gpt-4o", 500, longBody)
		})
		if !strings.Contains(out, "...(truncated)") {
			t.Errorf("expected truncation marker in output: %q", out)
		}
		if strings.Contains(out, strings.Repeat("x", 501)) {
			t.Errorf("expected body to be cut at 500 chars")
		}
	})
}

func TestLogRequestError(t *testing.T) {
	l := newTestLogger()
	out := captureStdout(t, func() {
		l.LogRequestError("req-1", "/v1/chat/completions", "gpt-4o", io.EOF, 2, 5)
	})
	if !strings.Contains(out, "req-1") || !strings.Contains(out, "attempt=2/5") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestLogRequest(t *testing.T) {
	l := newTestLogger()

	t.Run("without headers", func(t *testing.T) {
		out := captureStdout(t, func() {
			l.LogRequest("GET", "/health", nil)
		})
		if !strings.Contains(out, "GET") || !strings.Contains(out, "/health") {
			t.Errorf("unexpected output: %q", out)
		}
	})

	t.Run("with headers logs debug line", func(t *testing.T) {
		out := captureStdout(t, func() {
			l.LogRequest("POST", "/v1/messages", map[string]string{"X-Test": "1"})
		})
		if !strings.Contains(out, "Headers") {
			t.Errorf("expected headers debug line, got: %q", out)
		}
	})
}

func TestLogResponse(t *testing.T) {
	l := newTestLogger()

	tests := []struct {
		name   string
		status int
	}{
		{"2xx success", 200},
		{"3xx redirect", 304},
		{"4xx error", 404},
		{"5xx error", 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := captureStdout(t, func() {
				l.LogResponse(tt.status, 15*time.Millisecond)
			})
			if !strings.Contains(out, "Response") {
				t.Errorf("unexpected output: %q", out)
			}
		})
	}
}

func TestLogJSON(t *testing.T) {
	l := newTestLogger()
	out := captureStdout(t, func() {
		l.LogJSON("payload", map[string]int{"a": 1})
	})
	if !strings.Contains(out, "payload") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestPackageLevelConvenienceFuncs(t *testing.T) {
	GetLogger().SetLevel(DEBUG)
	out := captureStdout(t, func() {
		Debug("d")
		Info("i")
		Warn("w")
		Error("e")
	})
	for _, want := range []string{"d", "i", "w", "e"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got: %q", want, out)
		}
	}
}

// TestLog_ConcurrentAccessIsSafe 覆盖 Logger 内部锁的并发安全性——多个 goroutine
// 同时写日志和切换级别不应触发 race。
func TestLog_ConcurrentAccessIsSafe(t *testing.T) {
	l := newTestLogger()
	var wg sync.WaitGroup
	captureStdout(t, func() {
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				l.Info("message %d", n)
			}(i)
		}
		wg.Wait()
	})
}
