package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

type fakeAnalysisRepository struct {
	filter  repository.AnalysisPostFilter
	summary repository.AnalysisSummary
	err     error
	called  bool
}

func (f *fakeAnalysisRepository) GetAnalysisSummary(filter repository.AnalysisPostFilter) (repository.AnalysisSummary, error) {
	f.called = true
	f.filter = filter
	return f.summary, f.err
}

func TestAnalysisHandlerSummaryParsesFilters(t *testing.T) {
	ingestionRunID := int64(11)
	fake := &fakeAnalysisRepository{
		summary: repository.AnalysisSummary{
			SourceType:     "dataset",
			DatasetName:    "pheme",
			DatasetSplit:   "eventcv_large",
			IngestionRunID: &ingestionRunID,
			TotalAnalyzed:  1000,
			HighRisk:       120,
			MediumRisk:     300,
			LowRisk:        580,
			AverageScore:   0.42,
		},
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/analysis/summary?source_type=dataset&dataset_name=pheme&dataset_split=eventcv_large&ingestion_run_id=11",
		nil,
	)
	rr := httptest.NewRecorder()

	NewAnalysisHandler(fake).Summary().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	wantFilter := repository.AnalysisPostFilter{
		SourceType:     "dataset",
		DatasetName:    "pheme",
		DatasetSplit:   "eventcv_large",
		IngestionRunID: &ingestionRunID,
	}
	if !reflect.DeepEqual(fake.filter, wantFilter) {
		t.Fatalf("unexpected filter:\nwant %#v\ngot  %#v", wantFilter, fake.filter)
	}

	var payload repository.AnalysisSummary
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.TotalAnalyzed != 1000 || payload.HighRisk != 120 || payload.AverageScore != 0.42 {
		t.Fatalf("unexpected response: %#v", payload)
	}
}

func TestAnalysisHandlerSummaryRejectsInvalidIngestionRunID(t *testing.T) {
	fake := &fakeAnalysisRepository{}

	for _, target := range []string{"/api/analysis/summary?ingestion_run_id=0", "/api/analysis/summary?ingestion_run_id=abc"} {
		rr := httptest.NewRecorder()
		NewAnalysisHandler(fake).Summary().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, target, nil))

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected status 400, got %d", target, rr.Code)
		}
	}
	if fake.called {
		t.Fatalf("repository should not be called for invalid ingestion_run_id")
	}
}

func TestAnalysisHandlerSummaryMapsRepositoryError(t *testing.T) {
	fake := &fakeAnalysisRepository{err: errors.New("db down")}

	rr := httptest.NewRecorder()
	NewAnalysisHandler(fake).Summary().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/analysis/summary", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
}
