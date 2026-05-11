package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

// mockDashboardRepository implements dashboardRepository interface
type mockDashboardRepository struct {
	getExtendedDashboardMetricsFunc func(repository.DashboardFilter) (repository.ExtendedDashboardMetrics, error)
	getRadarDataForCaseFunc         func(int64, string) ([]repository.RadarDataPoint, error)
}

func (m *mockDashboardRepository) GetExtendedDashboardMetrics(filter repository.DashboardFilter) (repository.ExtendedDashboardMetrics, error) {
	if m.getExtendedDashboardMetricsFunc != nil {
		return m.getExtendedDashboardMetricsFunc(filter)
	}
	return repository.ExtendedDashboardMetrics{}, nil
}

func (m *mockDashboardRepository) GetRadarDataForCase(caseID int64, scorerKey string) ([]repository.RadarDataPoint, error) {
	if m.getRadarDataForCaseFunc != nil {
		return m.getRadarDataForCaseFunc(caseID, scorerKey)
	}
	return []repository.RadarDataPoint{}, nil
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestNewDashboardHandler(t *testing.T) {
	mockRepo := &mockDashboardRepository{}
	handler := NewDashboardHandler(mockRepo)

	if handler == nil {
		t.Error("expected handler to be created")
	}
	if handler.repo != mockRepo {
		t.Error("expected repo to be set correctly")
	}
}

func TestDashboardHandler_Metrics(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		mockMetrics    repository.ExtendedDashboardMetrics
		mockError      error
		expectedStatus int
	}{
		{
			name:        "successful metrics retrieval",
			queryParams: "?days=30&source_name=pheme&label=rumour",
			mockMetrics: repository.ExtendedDashboardMetrics{
				Summary: repository.DashboardSummary{
					TotalCases: 1221,
					MeanRisk:   0.264,
				},
				ManipulationStats: &repository.ManipulationStats{
					AvgManipulationScore: 0.105,
					TotalAnalyzed:        15885,
				},
				TopTactics: []repository.TacticStats{
					{TacticName: "эмоциональное давление", Count: 45},
				},
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "successful with default days",
			queryParams:    "",
			mockMetrics:    repository.ExtendedDashboardMetrics{},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid days parameter",
			queryParams:    "?days=invalid",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "days out of range - too high",
			queryParams:    "?days=400",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "days out of range - negative",
			queryParams:    "?days=-5",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "days zero",
			queryParams:    "?days=0",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "repository error",
			queryParams:    "?days=30",
			mockError:      &testError{msg: "database error"},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &mockDashboardRepository{
				getExtendedDashboardMetricsFunc: func(filter repository.DashboardFilter) (repository.ExtendedDashboardMetrics, error) {
					if tt.mockError != nil {
						return repository.ExtendedDashboardMetrics{}, tt.mockError
					}
					return tt.mockMetrics, nil
				},
			}

			handler := NewDashboardHandler(mockRepo)

			req := httptest.NewRequest(http.MethodGet, "/api/dashboard/metrics"+tt.queryParams, nil)
			w := httptest.NewRecorder()

			handler.Metrics()(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				var response repository.ExtendedDashboardMetrics
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Errorf("failed to decode response: %v", err)
				}
			}
		})
	}
}

func TestDashboardHandler_RadarData(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		queryParams    string
		mockRadarData  []repository.RadarDataPoint
		mockError      error
		expectedStatus int
	}{
		{
			name:        "successful radar data retrieval",
			path:        "/api/dashboard/radar/123",
			queryParams: "?scorer_key=case_ensemble_v1",
			mockRadarData: []repository.RadarDataPoint{
				{Branch: "coordination", Value: 0.78},
				{Branch: "temporal", Value: 0.65},
				{Branch: "narrative", Value: 0.88},
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid case id",
			path:           "/api/dashboard/radar/invalid",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing case id",
			path:           "/api/dashboard/radar/",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "repository error",
			path:           "/api/dashboard/radar/123",
			mockError:      &testError{msg: "database error"},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &mockDashboardRepository{
				getRadarDataForCaseFunc: func(caseID int64, scorerKey string) ([]repository.RadarDataPoint, error) {
					if tt.mockError != nil {
						return nil, tt.mockError
					}
					return tt.mockRadarData, nil
				},
			}

			handler := NewDashboardHandler(mockRepo)

			req := httptest.NewRequest(http.MethodGet, tt.path+tt.queryParams, nil)
			w := httptest.NewRecorder()

			handler.RadarData()(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				var response []repository.RadarDataPoint
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Errorf("failed to decode response: %v", err)
				}
				if len(response) != 3 {
					t.Errorf("expected 3 radar data points, got %d", len(response))
				}
			}
		})
	}
}

func TestDashboardHandler_Summary(t *testing.T) {
	mockRepo := &mockDashboardRepository{
		getExtendedDashboardMetricsFunc: func(filter repository.DashboardFilter) (repository.ExtendedDashboardMetrics, error) {
			return repository.ExtendedDashboardMetrics{
				Summary: repository.DashboardSummary{
					TotalCases: 1221,
					MeanRisk:   0.264,
				},
				ManipulationStats: &repository.ManipulationStats{
					AvgManipulationScore: 0.105,
					TotalAnalyzed:        15885,
				},
				RiskDistribution: struct {
					High   int `json:"high"`
					Medium int `json:"medium"`
					Low    int `json:"low"`
				}{High: 0, Medium: 24, Low: 1197},
				TopTactics: []repository.TacticStats{
					{TacticName: "эмоциональное давление", Count: 45},
				},
			}, nil
		},
	}

	handler := NewDashboardHandler(mockRepo)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/summary?days=30", nil)
	w := httptest.NewRecorder()

	handler.Summary()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Errorf("failed to decode response: %v", err)
	}
}

func TestDashboardHandler_RecentEvidence(t *testing.T) {
	mockRepo := &mockDashboardRepository{
		getExtendedDashboardMetricsFunc: func(filter repository.DashboardFilter) (repository.ExtendedDashboardMetrics, error) {
			return repository.ExtendedDashboardMetrics{
				RecentEvidence: []repository.EvidenceItem{
					{
						CaseID:            123,
						CaseTitle:         "Test Case",
						ManipulationScore: 0.85,
						Coordination:      0.78,
						Temporal:          0.65,
						Narrative:         0.88,
						CreatedAt:         time.Now(),
					},
				},
			}, nil
		},
	}

	handler := NewDashboardHandler(mockRepo)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/evidence?days=30", nil)
	w := httptest.NewRecorder()

	handler.RecentEvidence()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestDashboardHandler_BranchTimeSeries(t *testing.T) {
	mockRepo := &mockDashboardRepository{
		getExtendedDashboardMetricsFunc: func(filter repository.DashboardFilter) (repository.ExtendedDashboardMetrics, error) {
			return repository.ExtendedDashboardMetrics{
				BranchTimeSeries: []repository.BranchTimeSeriesPoint{
					{Date: "2024-01-01", Coordination: 0.78, Temporal: 0.65, Narrative: 0.88, Count: 10},
					{Date: "2024-01-02", Coordination: 0.72, Temporal: 0.60, Narrative: 0.82, Count: 15},
				},
			}, nil
		},
	}

	handler := NewDashboardHandler(mockRepo)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/branch-series?days=30", nil)
	w := httptest.NewRecorder()

	handler.BranchTimeSeries()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestDashboardHandler_CORSHeaders(t *testing.T) {
	mockRepo := &mockDashboardRepository{
		getExtendedDashboardMetricsFunc: func(filter repository.DashboardFilter) (repository.ExtendedDashboardMetrics, error) {
			return repository.ExtendedDashboardMetrics{}, nil
		},
	}

	handler := NewDashboardHandler(mockRepo)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/metrics", nil)
	w := httptest.NewRecorder()

	handler.Metrics()(w, req)

	corsOrigin := w.Header().Get("Access-Control-Allow-Origin")
	if corsOrigin != "*" {
		t.Errorf("expected Access-Control-Allow-Origin: *, got %s", corsOrigin)
	}
}
