package logger

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestHTTPLoggerCapturesImplicitOKStatusAndResponseSize(t *testing.T) {
	observed, logs := observer.New(zapcore.InfoLevel)
	previous := Log
	Log = zap.New(observed)
	defer func() { Log = previous }()

	handler := HTTPLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("pong"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/ping?verbose=1", nil)
	req.Header.Set("X-Request-ID", "req-123")
	req.Header.Set("User-Agent", "backend-test")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if rr.Header().Get("X-Request-ID") != "req-123" {
		t.Fatalf("expected propagated X-Request-ID, got %q", rr.Header().Get("X-Request-ID"))
	}

	entry := singleLogEntry(t, logs)
	if entry.Message != "HTTP request" {
		t.Fatalf("unexpected message: %q", entry.Message)
	}
	assertLogString(t, entry.Context, "uri", "/ping?verbose=1")
	assertLogString(t, entry.Context, "path", "/ping")
	assertLogString(t, entry.Context, "method", http.MethodGet)
	assertLogString(t, entry.Context, "request_id", "req-123")
	assertLogString(t, entry.Context, "user_agent", "backend-test")
	assertLogInt(t, entry.Context, "status", http.StatusOK)
	assertLogInt(t, entry.Context, "size", 4)
}

func TestHTTPLoggerCapturesExplicitStatus(t *testing.T) {
	observed, logs := observer.New(zapcore.InfoLevel)
	previous := Log
	Log = zap.New(observed)
	defer func() { Log = previous }()

	handler := HTTPLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad input", http.StatusBadRequest)
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/cases", nil))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}

	entry := singleLogEntry(t, logs)
	assertLogString(t, entry.Context, "method", http.MethodPost)
	assertLogInt(t, entry.Context, "status", http.StatusBadRequest)
	if fieldExists(entry.Context, "request_id") == false {
		t.Fatalf("expected generated request_id field")
	}
}

func TestConfigureStandardLogWritesToFileWhenEnabled(t *testing.T) {
	t.Setenv("LOG_TO_FILE", "true")
	t.Setenv("LOG_DIR", t.TempDir())
	t.Setenv("LOG_FILE", "")
	defer log.SetOutput(os.Stderr)

	file, err := ConfigureStandardLog("case scorer")
	if err != nil {
		t.Fatalf("configure standard log: %v", err)
	}
	if file == nil {
		t.Fatalf("expected log file handle")
	}

	log.Print("file logging works")
	if err := file.Close(); err != nil {
		t.Fatalf("close log file: %v", err)
	}
	log.SetOutput(io.Discard)

	logPath, err := LogFilePath("case scorer")
	if err != nil {
		t.Fatalf("log file path: %v", err)
	}
	if filepath.Base(logPath) != "case_scorer.log" {
		t.Fatalf("unexpected log file path: %s", logPath)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "file logging works") {
		t.Fatalf("expected log message in file, got %q", string(data))
	}
}

func TestConfigureStandardLogIsNoopWhenDisabled(t *testing.T) {
	t.Setenv("LOG_TO_FILE", "")

	file, err := ConfigureStandardLog("dataset_loader")
	if err != nil {
		t.Fatalf("configure standard log: %v", err)
	}
	if file != nil {
		t.Fatalf("expected no log file when LOG_TO_FILE is disabled")
	}
}

func singleLogEntry(t *testing.T, logs *observer.ObservedLogs) observer.LoggedEntry {
	t.Helper()
	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected one log entry, got %d", len(entries))
	}
	return entries[0]
}

func assertLogString(t *testing.T, fields []zapcore.Field, key, want string) {
	t.Helper()
	for _, field := range fields {
		if field.Key == key {
			if field.String != want {
				t.Fatalf("field %s = %q, want %q", key, field.String, want)
			}
			return
		}
	}
	t.Fatalf("missing string field %s", key)
}

func assertLogInt(t *testing.T, fields []zapcore.Field, key string, want int) {
	t.Helper()
	for _, field := range fields {
		if field.Key == key {
			if int(field.Integer) != want {
				t.Fatalf("field %s = %d, want %d", key, field.Integer, want)
			}
			return
		}
	}
	t.Fatalf("missing int field %s", key)
}

func fieldExists(fields []zapcore.Field, key string) bool {
	for _, field := range fields {
		if field.Key == key {
			return true
		}
	}
	return false
}
