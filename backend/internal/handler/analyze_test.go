package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mockMLServer создает тестовый ML сервер
func mockMLServer(responseCode int, responseBody interface{}) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(responseCode)
		json.NewEncoder(w).Encode(responseBody)
	}))
}

func TestNewAnalyzeHandler(t *testing.T) {
	mlURL := "http://test-ml:8000"
	handler := NewAnalyzeHandler(mlURL)

	if handler.mlURL != mlURL {
		t.Errorf("expected mlURL %s, got %s", mlURL, handler.mlURL)
	}
	if handler.httpClient == nil {
		t.Error("expected httpClient to be initialized")
	}
	if handler.httpClient.Timeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %v", handler.httpClient.Timeout)
	}
}

func TestAnalyzeHandler_Analyze(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		requestBody    interface{}
		mlResponseCode int
		mlResponseBody interface{}
		expectedStatus int
		expectedError  string
	}{
		{
			name:   "successful analysis - high manipulation",
			method: http.MethodPost,
			requestBody: AnalyzeRequest{
				Text:   "СРОЧНО! Все православные, внимание! Нас обманывают!",
				Source: "test",
			},
			mlResponseCode: http.StatusOK,
			mlResponseBody: MLResponse{
				ManipulationScore:        0.85,
				ConfidenceScore:          0.92,
				CoordinationContribution: 0.78,
				TemporalContribution:     0.65,
				NarrativeContribution:    0.88,
				ConfidenceNote:           "Высокая уверенность",
				KeyEvidence:              []string{"эмоциональный маркер", "призыв к действию"},
				Tactics:                  []string{"эмоциональное давление", "пропаганда"},
			},
			expectedStatus: http.StatusOK,
			expectedError:  "",
		},
		{
			name:   "successful analysis - medium manipulation",
			method: http.MethodPost,
			requestBody: AnalyzeRequest{
				Text:   "Обычный текст без манипуляций",
				Source: "test",
			},
			mlResponseCode: http.StatusOK,
			mlResponseBody: MLResponse{
				ManipulationScore:        0.45,
				ConfidenceScore:          0.60,
				CoordinationContribution: 0.30,
				TemporalContribution:     0.50,
				NarrativeContribution:    0.55,
				ConfidenceNote:           "Средняя уверенность",
				KeyEvidence:              []string{},
				Tactics:                  []string{},
			},
			expectedStatus: http.StatusOK,
			expectedError:  "",
		},
		{
			name:   "successful analysis - low manipulation",
			method: http.MethodPost,
			requestBody: AnalyzeRequest{
				Text:   "Hello world",
				Source: "test",
			},
			mlResponseCode: http.StatusOK,
			mlResponseBody: MLResponse{
				ManipulationScore:        0.10,
				ConfidenceScore:          0.50,
				CoordinationContribution: 0.10,
				TemporalContribution:     0.10,
				NarrativeContribution:    0.10,
				ConfidenceNote:           "Низкая уверенность",
			},
			expectedStatus: http.StatusOK,
			expectedError:  "",
		},
		{
			name:           "wrong method - GET",
			method:         http.MethodGet,
			requestBody:    nil,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedError:  "Method not allowed",
		},
		{
			name:           "wrong method - PUT",
			method:         http.MethodPut,
			requestBody:    nil,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedError:  "Method not allowed",
		},
		{
			name:           "empty text",
			method:         http.MethodPost,
			requestBody:    AnalyzeRequest{Text: "", Source: "test"},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Text is required",
		},
		{
			name:           "invalid JSON",
			method:         http.MethodPost,
			requestBody:    "invalid json",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Invalid request",
		},
		{
			name:   "ML service error",
			method: http.MethodPost,
			requestBody: AnalyzeRequest{
				Text:   "test text",
				Source: "test",
			},
			mlResponseCode: http.StatusInternalServerError,
			mlResponseBody: map[string]string{"error": "ML error"},
			expectedStatus: http.StatusInternalServerError,
			expectedError:  "ML service error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Создаем тестовый ML сервер
			var mlServer *httptest.Server
			if tt.mlResponseCode != 0 {
				mlServer = mockMLServer(tt.mlResponseCode, tt.mlResponseBody)
				defer mlServer.Close()
			} else {
				mlServer = mockMLServer(http.StatusOK, MLResponse{})
				defer mlServer.Close()
			}

			handler := NewAnalyzeHandler(mlServer.URL)

			// Подготовка запроса
			var reqBody *bytes.Reader
			if tt.requestBody != nil {
				bodyBytes, _ := json.Marshal(tt.requestBody)
				reqBody = bytes.NewReader(bodyBytes)
			} else {
				reqBody = bytes.NewReader([]byte(`{"text":"test"}`))
			}

			req := httptest.NewRequest(tt.method, "/api/analyze", reqBody)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.Analyze()(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedError != "" && !bytes.Contains(w.Body.Bytes(), []byte(tt.expectedError)) {
				t.Errorf("expected error message containing '%s', got '%s'", tt.expectedError, w.Body.String())
			}

			// Проверяем успешный ответ
			if tt.expectedStatus == http.StatusOK {
				var response AnalyzeResponse
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Errorf("failed to decode response: %v", err)
				}

				// Проверяем приоритет эскалации
				if response.EscalationPriority < 1 || response.EscalationPriority > 3 {
					t.Errorf("invalid escalation priority: %d", response.EscalationPriority)
				}

				// Проверяем, что analyzed_at установлено
				if response.AnalyzedAt.IsZero() {
					t.Error("analyzed_at should be set")
				}
			}
		})
	}
}

func TestAnalyzeHandler_EscalationPriority(t *testing.T) {
	tests := []struct {
		name              string
		manipulationScore float64
		confidenceScore   float64
		expectedPriority  int
	}{
		{
			name:              "high priority - high manipulation and high confidence",
			manipulationScore: 0.85,
			confidenceScore:   0.80,
			expectedPriority:  1,
		},
		{
			name:              "high priority - very high manipulation",
			manipulationScore: 0.95,
			confidenceScore:   0.75,
			expectedPriority:  1,
		},
		{
			name:              "medium priority - moderate manipulation",
			manipulationScore: 0.55,
			confidenceScore:   0.70,
			expectedPriority:  2,
		},
		{
			name:              "medium priority - high manipulation but low confidence",
			manipulationScore: 0.72,
			confidenceScore:   0.65,
			expectedPriority:  2,
		},
		{
			name:              "low priority - low manipulation",
			manipulationScore: 0.35,
			confidenceScore:   0.80,
			expectedPriority:  3,
		},
		{
			name:              "low priority - very low manipulation",
			manipulationScore: 0.10,
			confidenceScore:   0.50,
			expectedPriority:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mlServer := mockMLServer(http.StatusOK, MLResponse{
				ManipulationScore: tt.manipulationScore,
				ConfidenceScore:   tt.confidenceScore,
			})
			defer mlServer.Close()

			handler := NewAnalyzeHandler(mlServer.URL)

			reqBody := AnalyzeRequest{Text: "test text", Source: "test"}
			bodyBytes, _ := json.Marshal(reqBody)

			req := httptest.NewRequest(http.MethodPost, "/api/analyze", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.Analyze()(w, req)

			var response AnalyzeResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Errorf("failed to decode response: %v", err)
			}

			if response.EscalationPriority != tt.expectedPriority {
				t.Errorf("expected escalation priority %d, got %d", tt.expectedPriority, response.EscalationPriority)
			}
		})
	}
}

func TestAnalyzeHandler_CORSHeaders(t *testing.T) {
	mlServer := mockMLServer(http.StatusOK, MLResponse{ManipulationScore: 0.5, ConfidenceScore: 0.7})
	defer mlServer.Close()

	handler := NewAnalyzeHandler(mlServer.URL)

	reqBody := AnalyzeRequest{Text: "test text", Source: "test"}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/analyze", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Analyze()(w, req)

	corsOrigin := w.Header().Get("Access-Control-Allow-Origin")
	if corsOrigin != "*" {
		t.Errorf("expected Access-Control-Allow-Origin: *, got %s", corsOrigin)
	}
}

func TestAnalyzeHandler_MLServiceUnavailable(t *testing.T) {
	// Используем несуществующий URL
	handler := NewAnalyzeHandler("http://localhost:9999")

	reqBody := AnalyzeRequest{Text: "test text", Source: "test"}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/analyze", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Analyze()(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

// Бенчмарк тест
func BenchmarkAnalyzeHandler(b *testing.B) {
	mlServer := mockMLServer(http.StatusOK, MLResponse{
		ManipulationScore: 0.75,
		ConfidenceScore:   0.85,
	})
	defer mlServer.Close()

	handler := NewAnalyzeHandler(mlServer.URL)

	reqBody := AnalyzeRequest{Text: "test text for benchmarking", Source: "test"}
	bodyBytes, _ := json.Marshal(reqBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/analyze", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.Analyze()(w, req)
	}
}
