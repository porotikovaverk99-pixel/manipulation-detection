package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

type fakeAuditRepository struct {
	event  repository.AuditEventRecord
	called bool
	err    error
}

func (f *fakeAuditRepository) SaveAuditEvent(event repository.AuditEventRecord) (int64, error) {
	f.called = true
	f.event = event
	return 1, f.err
}

func TestAuditMiddlewareStoresHTTPEvent(t *testing.T) {
	fake := &fakeAuditRepository{}
	handler := NewAuditMiddleware(fake)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/cases?limit=1", nil)
	req.Header.Set("X-Request-ID", "req-audit-1")
	req.Header.Set("User-Agent", "audit-test")
	req.RemoteAddr = "127.0.0.1:12345"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if rr.Header().Get("X-Request-ID") != "req-audit-1" {
		t.Fatalf("expected propagated X-Request-ID, got %q", rr.Header().Get("X-Request-ID"))
	}
	if !fake.called {
		t.Fatalf("expected audit repository call")
	}
	if fake.event.Action != "http_request" ||
		fake.event.EntityType != "http_endpoint" ||
		fake.event.EntityID != "/api/cases" ||
		fake.event.Status != "succeeded" ||
		fake.event.RequestID != "req-audit-1" ||
		fake.event.ActorID != "127.0.0.1:12345" {
		t.Fatalf("unexpected audit event: %#v", fake.event)
	}
	if fake.event.Payload["method"] != http.MethodGet ||
		fake.event.Payload["uri"] != "/api/cases?limit=1" ||
		fake.event.Payload["status"] != http.StatusOK ||
		fake.event.Payload["size"] != 2 {
		t.Fatalf("unexpected audit payload: %#v", fake.event.Payload)
	}
}

func TestAuditMiddlewareMapsClientAndServerStatuses(t *testing.T) {
	tests := []struct {
		name       string
		httpStatus int
		wantAudit  string
	}{
		{name: "client error", httpStatus: http.StatusBadRequest, wantAudit: "skipped"},
		{name: "server error", httpStatus: http.StatusInternalServerError, wantAudit: "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeAuditRepository{}
			handler := NewAuditMiddleware(fake)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "error", tt.httpStatus)
			}))

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/analyze", nil))

			if fake.event.Status != tt.wantAudit {
				t.Fatalf("audit status = %q, want %q", fake.event.Status, tt.wantAudit)
			}
		})
	}
}
