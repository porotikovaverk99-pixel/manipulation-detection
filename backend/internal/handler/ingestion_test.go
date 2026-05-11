package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

// mockIngestionRepository implements ingestionRepository interface
type mockIngestionRepository struct {
	getIngestionRunsFunc func(limit int) ([]repository.IngestionRun, error)
}

func (m *mockIngestionRepository) GetIngestionRuns(limit int) ([]repository.IngestionRun, error) {
	if m.getIngestionRunsFunc != nil {
		return m.getIngestionRunsFunc(limit)
	}
	return []repository.IngestionRun{}, nil
}

func TestNewIngestionHandler(t *testing.T) {
	mockRepo := &mockIngestionRepository{}
	handler := NewIngestionHandler(mockRepo)

	if handler == nil {
		t.Error("expected handler to be created")
	}
	if handler.repo != mockRepo {
		t.Error("expected repo to be set correctly")
	}
}

func TestIngestionHandler_ListRuns(t *testing.T) {
	now := time.Now()
	finishedAt := now.Add(-1 * time.Hour)

	tests := []struct {
		name           string
		queryParams    string
		mockRuns       []repository.IngestionRun
		mockError      error
		expectedStatus int
		expectedCount  int
	}{
		{
			name:        "successful list runs with default limit",
			queryParams: "",
			mockRuns: []repository.IngestionRun{
				{
					ID:            1,
					SourceType:    "dataset",
					SourceName:    "pheme",
					DatasetName:   "pheme",
					DatasetSplit:  "eventcv_large",
					Status:        "completed",
					StartedAt:     now,
					FinishedAt:    &finishedAt,
					Notes:         "",
					PostsIngested: 1000,
				},
				{
					ID:            2,
					SourceType:    "dataset",
					SourceName:    "pheme",
					DatasetName:   "pheme",
					DatasetSplit:  "eventcv",
					Status:        "running",
					StartedAt:     now,
					FinishedAt:    nil,
					Notes:         "in progress",
					PostsIngested: 500,
				},
			},
			expectedStatus: http.StatusOK,
			expectedCount:  2,
		},
		{
			name:        "successful list runs with custom limit",
			queryParams: "?limit=10",
			mockRuns: []repository.IngestionRun{
				{ID: 1, SourceType: "dataset", SourceName: "pheme", Status: "completed", StartedAt: now, PostsIngested: 1000},
				{ID: 2, SourceType: "dataset", SourceName: "pheme", Status: "completed", StartedAt: now, PostsIngested: 800},
				{ID: 3, SourceType: "dataset", SourceName: "pheme", Status: "completed", StartedAt: now, PostsIngested: 600},
			},
			expectedStatus: http.StatusOK,
			expectedCount:  3,
		},
		{
			name:           "empty list",
			queryParams:    "",
			mockRuns:       []repository.IngestionRun{},
			expectedStatus: http.StatusOK,
			expectedCount:  0,
		},
		{
			name:        "invalid limit - non numeric",
			queryParams: "?limit=abc",
			mockRuns: []repository.IngestionRun{
				{ID: 1, SourceType: "dataset", SourceName: "pheme", Status: "completed", StartedAt: now, PostsIngested: 1000},
			},
			expectedStatus: http.StatusOK,
			expectedCount:  1,
		},
		{
			name:        "invalid limit - negative",
			queryParams: "?limit=-5",
			mockRuns: []repository.IngestionRun{
				{ID: 1, SourceType: "dataset", SourceName: "pheme", Status: "completed", StartedAt: now, PostsIngested: 1000},
			},
			expectedStatus: http.StatusOK,
			expectedCount:  1,
		},
		{
			name:        "invalid limit - too high (>200)",
			queryParams: "?limit=300",
			mockRuns: []repository.IngestionRun{
				{ID: 1, SourceType: "dataset", SourceName: "pheme", Status: "completed", StartedAt: now, PostsIngested: 1000},
			},
			expectedStatus: http.StatusOK,
			expectedCount:  1,
		},
		{
			name:        "limit zero",
			queryParams: "?limit=0",
			mockRuns: []repository.IngestionRun{
				{ID: 1, SourceType: "dataset", SourceName: "pheme", Status: "completed", StartedAt: now, PostsIngested: 1000},
			},
			expectedStatus: http.StatusOK,
			expectedCount:  1,
		},
		{
			name:           "repository error",
			queryParams:    "",
			mockError:      &testError{msg: "database connection failed"},
			expectedStatus: http.StatusInternalServerError,
			expectedCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &mockIngestionRepository{
				getIngestionRunsFunc: func(limit int) ([]repository.IngestionRun, error) {
					if tt.mockError != nil {
						return nil, tt.mockError
					}
					return tt.mockRuns, nil
				},
			}

			handler := NewIngestionHandler(mockRepo)

			req := httptest.NewRequest(http.MethodGet, "/api/ingestion/runs"+tt.queryParams, nil)
			w := httptest.NewRecorder()

			handler.ListRuns()(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				var response struct {
					Items []repository.IngestionRun `json:"items"`
					Count int                       `json:"count"`
				}
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Errorf("failed to decode response: %v", err)
				}

				if response.Count != tt.expectedCount {
					t.Errorf("expected count %d, got %d", tt.expectedCount, response.Count)
				}

				if len(response.Items) != tt.expectedCount {
					t.Errorf("expected %d items, got %d", tt.expectedCount, len(response.Items))
				}
			}
		})
	}
}

func TestIngestionHandler_ListRuns_ParameterPassing(t *testing.T) {
	var capturedLimit int

	mockRepo := &mockIngestionRepository{
		getIngestionRunsFunc: func(limit int) ([]repository.IngestionRun, error) {
			capturedLimit = limit
			return []repository.IngestionRun{}, nil
		},
	}

	handler := NewIngestionHandler(mockRepo)

	tests := []struct {
		name          string
		queryParams   string
		expectedLimit int
	}{
		{"default limit", "", 20},
		{"custom limit 5", "?limit=5", 5},
		{"custom limit 50", "?limit=50", 50},
		{"invalid limit falls back to default", "?limit=invalid", 20},
		{"negative limit falls back to default", "?limit=-10", 20},
		{"zero limit falls back to default", "?limit=0", 20},
		{"too high limit (>200) falls back to default", "?limit=300", 20},
		{"limit 200 allowed", "?limit=200", 200},
		{"limit 1 allowed", "?limit=1", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/ingestion/runs"+tt.queryParams, nil)
			w := httptest.NewRecorder()

			handler.ListRuns()(w, req)

			if capturedLimit != tt.expectedLimit {
				t.Errorf("expected limit %d, got %d", tt.expectedLimit, capturedLimit)
			}
		})
	}
}

func TestIngestionHandler_CORSHeaders(t *testing.T) {
	mockRepo := &mockIngestionRepository{
		getIngestionRunsFunc: func(limit int) ([]repository.IngestionRun, error) {
			return []repository.IngestionRun{}, nil
		},
	}

	handler := NewIngestionHandler(mockRepo)

	req := httptest.NewRequest(http.MethodGet, "/api/ingestion/runs", nil)
	w := httptest.NewRecorder()

	handler.ListRuns()(w, req)

	corsOrigin := w.Header().Get("Access-Control-Allow-Origin")
	if corsOrigin != "*" {
		t.Errorf("expected Access-Control-Allow-Origin: *, got %s", corsOrigin)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type: application/json, got %s", contentType)
	}
}

// Бенчмарк тест
func BenchmarkIngestionHandler_ListRuns(b *testing.B) {
	mockRepo := &mockIngestionRepository{
		getIngestionRunsFunc: func(limit int) ([]repository.IngestionRun, error) {
			runs := make([]repository.IngestionRun, 20)
			now := time.Now()
			for i := 0; i < 20; i++ {
				runs[i] = repository.IngestionRun{
					ID:            int64(i + 1),
					SourceType:    "dataset",
					SourceName:    "pheme",
					Status:        "completed",
					StartedAt:     now,
					PostsIngested: 1000,
				}
			}
			return runs, nil
		},
	}

	handler := NewIngestionHandler(mockRepo)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/ingestion/runs", nil)
		w := httptest.NewRecorder()
		handler.ListRuns()(w, req)
	}
}
