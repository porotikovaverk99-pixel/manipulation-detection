package handler

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

type auditRepository interface {
	SaveAuditEvent(repository.AuditEventRecord) (int64, error)
}

type auditResponseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (w *auditResponseWriter) WriteHeader(statusCode int) {
	if w.status != 0 {
		return
	}
	w.status = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *auditResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	size, err := w.ResponseWriter.Write(body)
	w.size += size
	return size, err
}

func NewAuditMiddleware(repo auditRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := requestIDFromHeader(r)
			r.Header.Set("X-Request-ID", requestID)
			w.Header().Set("X-Request-ID", requestID)

			started := time.Now()
			recorder := &auditResponseWriter{ResponseWriter: w}
			next.ServeHTTP(recorder, r)

			status := recorder.status
			if status == 0 {
				status = http.StatusOK
			}

			if repo == nil {
				return
			}
			if _, err := repo.SaveAuditEvent(repository.AuditEventRecord{
				ActorType:  "api_client",
				ActorID:    r.RemoteAddr,
				Action:     "http_request",
				EntityType: "http_endpoint",
				EntityID:   r.URL.Path,
				Status:     auditStatus(status),
				RequestID:  requestID,
				Payload: map[string]interface{}{
					"method":      r.Method,
					"uri":         r.RequestURI,
					"path":        r.URL.Path,
					"status":      status,
					"duration_ms": time.Since(started).Milliseconds(),
					"size":        recorder.size,
					"remote_addr": r.RemoteAddr,
					"user_agent":  r.UserAgent(),
				},
			}); err != nil {
				log.Printf("save audit event failed: %v", err)
			}
		})
	}
}

func auditStatus(httpStatus int) string {
	switch {
	case httpStatus >= 500:
		return "failed"
	case httpStatus >= 400:
		return "skipped"
	default:
		return "succeeded"
	}
}

func requestIDFromHeader(r *http.Request) string {
	if id := r.Header.Get("X-Request-ID"); id != "" {
		return id
	}
	return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
}
