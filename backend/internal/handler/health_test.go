package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeHealthDB struct {
	err error
}

func (f fakeHealthDB) Ping() error {
	return f.err
}

func TestHealthHandlerReturnsOKWhenDependenciesAreHealthy(t *testing.T) {
	mlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("unexpected ML health path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer mlServer.Close()

	rr := httptest.NewRecorder()
	NewHealthHandler(fakeHealthDB{}, mlServer.URL).Health().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var payload HealthResponse
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Status != "ok" || payload.Checks["database"].Status != "ok" || payload.Checks["ml_service"].Status != "ok" {
		t.Fatalf("unexpected health response: %#v", payload)
	}
}

func TestHealthHandlerReturnsUnavailableWhenDatabaseFails(t *testing.T) {
	mlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mlServer.Close()

	rr := httptest.NewRecorder()
	NewHealthHandler(fakeHealthDB{err: errors.New("db offline")}, mlServer.URL).Health().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d: %s", rr.Code, rr.Body.String())
	}

	var payload HealthResponse
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Status != "degraded" || payload.Checks["database"].Status != "fail" {
		t.Fatalf("unexpected health response: %#v", payload)
	}
}

func TestHealthHandlerReturnsUnavailableWhenMLServiceFails(t *testing.T) {
	mlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mlServer.Close()

	rr := httptest.NewRecorder()
	NewHealthHandler(fakeHealthDB{}, mlServer.URL).Health().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d: %s", rr.Code, rr.Body.String())
	}

	var payload HealthResponse
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Status != "degraded" || payload.Checks["ml_service"].Status != "fail" {
		t.Fatalf("unexpected health response: %#v", payload)
	}
}
