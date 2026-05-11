package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

// mockAnalysisRepository implements analysisRepository interface for testing
type mockAnalysisRepository struct {
	getAnalysisSummaryFunc func(repository.AnalysisPostFilter) (repository.AnalysisSummary, error)
}

func (m *mockAnalysisRepository) GetAnalysisSummary(filter repository.AnalysisPostFilter) (repository.AnalysisSummary, error) {
	if m.getAnalysisSummaryFunc != nil {
		return m.getAnalysisSummaryFunc(filter)
	}
	return repository.AnalysisSummary{}, nil
}

func TestAnalysisHandler_Summary(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		mockResponse   repository.AnalysisSummary
		mockError      error
		expectedStatus int
		expectedBody   *repository.AnalysisSummary
	}{
		{
			name:        "successful summary with all filters",
			queryParams: "?source_type=dataset&dataset_name=pheme&dataset_split=eventcv_large",
			mockResponse: repository.AnalysisSummary{
				SourceType:    "dataset",
				DatasetName:   "pheme",
				DatasetSplit:  "eventcv_large",
				TotalAnalyzed: 15885,
				HighRisk:      0,
				MediumRisk:    24,
				LowRisk:       1197,
				AverageScore:  0.105428,
			},
			expectedStatus: http.StatusOK,
			expectedBody: &repository.AnalysisSummary{
				SourceType:    "dataset",
				DatasetName:   "pheme",
				DatasetSplit:  "eventcv_large",
				TotalAnalyzed: 15885,
				HighRisk:      0,
				MediumRisk:    24,
				LowRisk:       1197,
				AverageScore:  0.105428,
			},
		},
		{
			name:           "successful summary without filters",
			queryParams:    "",
			mockResponse:   repository.AnalysisSummary{TotalAnalyzed: 15885, AverageScore: 0.105},
			expectedStatus: http.StatusOK,
			expectedBody:   &repository.AnalysisSummary{TotalAnalyzed: 15885, AverageScore: 0.105},
		},
		{
			name:           "successful summary with ingestion_run_id",
			queryParams:    "?ingestion_run_id=5",
			mockResponse:   repository.AnalysisSummary{TotalAnalyzed: 5000, AverageScore: 0.12},
			expectedStatus: http.StatusOK,
			expectedBody:   &repository.AnalysisSummary{TotalAnalyzed: 5000, AverageScore: 0.12},
		},
		{
			name:           "invalid ingestion_run_id - non numeric",
			queryParams:    "?ingestion_run_id=abc",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   nil,
		},
		{
			name:           "invalid ingestion_run_id - negative",
			queryParams:    "?ingestion_run_id=-5",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   nil,
		},
		{
			name:           "repository error",
			queryParams:    "?source_type=dataset",
			mockError:      errors.New("database connection failed"),
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &mockAnalysisRepository{
				getAnalysisSummaryFunc: func(filter repository.AnalysisPostFilter) (repository.AnalysisSummary, error) {
					if tt.mockError != nil {
						return repository.AnalysisSummary{}, tt.mockError
					}
					return tt.mockResponse, nil
				},
			}

			handler := NewAnalysisHandler(mockRepo)

			req := httptest.NewRequest(http.MethodGet, "/api/analysis/summary"+tt.queryParams, nil)
			w := httptest.NewRecorder()

			handler.Summary()(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedBody != nil {
				var response repository.AnalysisSummary
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Errorf("failed to decode response: %v", err)
				}
				if response.TotalAnalyzed != tt.expectedBody.TotalAnalyzed {
					t.Errorf("expected TotalAnalyzed %d, got %d", tt.expectedBody.TotalAnalyzed, response.TotalAnalyzed)
				}
			}
		})
	}
}

func TestAnalysisHandler_Summary_ParameterPassing(t *testing.T) {
	var capturedFilter repository.AnalysisPostFilter

	mockRepo := &mockAnalysisRepository{
		getAnalysisSummaryFunc: func(filter repository.AnalysisPostFilter) (repository.AnalysisSummary, error) {
			capturedFilter = filter
			return repository.AnalysisSummary{}, nil
		},
	}

	handler := NewAnalysisHandler(mockRepo)

	req := httptest.NewRequest(http.MethodGet, "/api/analysis/summary?source_type=dataset&dataset_name=pheme&dataset_split=eventcv_large&ingestion_run_id=42", nil)
	w := httptest.NewRecorder()

	handler.Summary()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if capturedFilter.SourceType != "dataset" {
		t.Errorf("expected SourceType 'dataset', got '%s'", capturedFilter.SourceType)
	}
	if capturedFilter.DatasetName != "pheme" {
		t.Errorf("expected DatasetName 'pheme', got '%s'", capturedFilter.DatasetName)
	}
	if capturedFilter.DatasetSplit != "eventcv_large" {
		t.Errorf("expected DatasetSplit 'eventcv_large', got '%s'", capturedFilter.DatasetSplit)
	}
	if capturedFilter.IngestionRunID == nil || *capturedFilter.IngestionRunID != 42 {
		t.Errorf("expected IngestionRunID 42, got %v", capturedFilter.IngestionRunID)
	}
}
