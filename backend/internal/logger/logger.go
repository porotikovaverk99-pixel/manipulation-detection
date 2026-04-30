package logger

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

var Log *zap.Logger = zap.NewNop()

type (
	responseData struct {
		status int
		size   int
	}

	loggingResponseWriter struct {
		http.ResponseWriter
		responseData *responseData
	}
)

func (r *loggingResponseWriter) WriteHeader(statusCode int) {
	if r.responseData.status != 0 {
		return
	}
	r.ResponseWriter.WriteHeader(statusCode)
	r.responseData.status = statusCode
}

func (r *loggingResponseWriter) Write(b []byte) (int, error) {
	if r.responseData.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	size, err := r.ResponseWriter.Write(b)
	r.responseData.size += size
	return size, err
}

func Initialize(level string) error {

	lvl, err := zap.ParseAtomicLevel(level)
	if err != nil {
		return err
	}

	cfg := zap.NewProductionConfig()
	cfg.Level = lvl
	cfg.OutputPaths = []string{"stderr"}
	cfg.ErrorOutputPaths = []string{"stderr"}

	if FileLoggingEnabled() {
		logPath, err := LogFilePath("api")
		if err != nil {
			return err
		}
		cfg.OutputPaths = append(cfg.OutputPaths, logPath)
		cfg.ErrorOutputPaths = append(cfg.ErrorOutputPaths, logPath)
	}

	zl, err := cfg.Build()
	if err != nil {
		return err
	}

	Log = zl
	if _, err := ConfigureStandardLog("api"); err != nil {
		return err
	}
	return nil

}

func ConfigureStandardLog(component string) (*os.File, error) {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.LUTC)
	if !FileLoggingEnabled() {
		return nil, nil
	}

	logPath, err := LogFilePath(component)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", logPath, err)
	}

	log.SetOutput(io.MultiWriter(os.Stderr, file))
	return file, nil
}

func FileLoggingEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_TO_FILE"))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func LogFilePath(component string) (string, error) {
	logDir := strings.TrimSpace(os.Getenv("LOG_DIR"))
	if logDir == "" {
		logDir = "logs"
	}

	logFile := strings.TrimSpace(os.Getenv("LOG_FILE"))
	if logFile == "" {
		logFile = sanitizeLogComponent(component) + ".log"
	}
	if filepath.IsAbs(logFile) {
		if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
			return "", fmt.Errorf("create log directory: %w", err)
		}
		return logFile, nil
	}

	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return "", fmt.Errorf("create log directory: %w", err)
	}
	return filepath.Join(logDir, logFile), nil
}

func sanitizeLogComponent(component string) string {
	component = strings.ToLower(strings.TrimSpace(component))
	var b strings.Builder
	for _, ch := range component {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			b.WriteRune(ch)
			continue
		}
		b.WriteByte('_')
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "app"
	}
	return out
}

func HTTPLogger(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		uri := r.RequestURI
		method := r.Method
		requestID := requestID(r)
		if requestID != "" {
			w.Header().Set("X-Request-ID", requestID)
		}

		responseData := &responseData{
			status: 0,
			size:   0,
		}

		lw := &loggingResponseWriter{
			ResponseWriter: w,
			responseData:   responseData,
		}

		start := time.Now()

		h.ServeHTTP(lw, r)

		duration := time.Since(start)
		if responseData.status == 0 {
			responseData.status = http.StatusOK
		}

		Log.Info("HTTP request",
			zap.String("uri", uri),
			zap.String("path", r.URL.Path),
			zap.String("method", method),
			zap.String("remote_addr", r.RemoteAddr),
			zap.String("user_agent", r.UserAgent()),
			zap.String("request_id", requestID),
			zap.Duration("duration", duration),
			zap.Int("status", responseData.status),
			zap.Int("size", responseData.size),
		)
	})
}

func requestID(r *http.Request) string {
	if id := r.Header.Get("X-Request-ID"); id != "" {
		return id
	}
	return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
}
