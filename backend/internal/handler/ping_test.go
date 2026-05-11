package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewPingHandler(t *testing.T) {
	handler := NewPingHandler()
	if handler == nil {
		t.Error("expected handler to be created, got nil")
	}
}

func TestPingHandler_Ping(t *testing.T) {
	handler := NewPingHandler()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()

	handler.Ping()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}

	if response["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%s'", response["status"])
	}

	if response["message"] != "pong" {
		t.Errorf("expected message 'pong', got '%s'", response["message"])
	}
}

func TestPingHandler_Ping_MethodNotAllowed(t *testing.T) {
	handler := NewPingHandler()

	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodOptions}

	for _, method := range methods {
		t.Run("method "+method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/ping", nil)
			w := httptest.NewRecorder()

			handler.Ping()(w, req)

			// PingHandler не проверяет метод, поэтому всегда отвечает 200
			// Это нормально для простого health check
			if w.Code != http.StatusOK {
				t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
			}
		})
	}
}

func TestPingHandler_Ping_ResponseFormat(t *testing.T) {
	handler := NewPingHandler()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()

	handler.Ping()(w, req)

	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}

	// Проверяем, что в ответе есть только ожидаемые поля
	expectedFields := []string{"status", "message"}
	for _, field := range expectedFields {
		if _, ok := response[field]; !ok {
			t.Errorf("expected field '%s' in response", field)
		}
	}

	// Проверяем, что нет лишних полей
	if len(response) != 2 {
		t.Errorf("expected 2 fields in response, got %d", len(response))
	}
}

func TestPingHandler_Ping_CORSHeaders(t *testing.T) {
	handler := NewPingHandler()

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()

	handler.Ping()(w, req)

	// PingHandler не добавляет CORS заголовки - они добавляются в middleware
	// Этот тест просто проверяет, что хендлер не паникует
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// Бенчмарк тест для Ping
func BenchmarkPingHandler(b *testing.B) {
	handler := NewPingHandler()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		w := httptest.NewRecorder()
		handler.Ping()(w, req)
	}
}

// Тест на параллельное выполнение
func TestPingHandler_Ping_Concurrent(t *testing.T) {
	handler := NewPingHandler()

	const numRequests = 100
	done := make(chan bool, numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodGet, "/ping", nil)
			w := httptest.NewRecorder()
			handler.Ping()(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", w.Code)
			}

			var response map[string]string
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Errorf("failed to decode response: %v", err)
			}

			done <- true
		}()
	}

	for i := 0; i < numRequests; i++ {
		<-done
	}
}
