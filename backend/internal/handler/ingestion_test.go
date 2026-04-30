package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

type fakeIngestionRepository struct {
	limit  int
	runs   []repository.IngestionRun
	err    error
	called bool
}

func (f *fakeIngestionRepository) GetIngestionRuns(limit int) ([]repository.IngestionRun, error) {
	f.called = true
	f.limit = limit
	return f.runs, f.err
}

func TestIngestionHandlerListRunsUsesDefaultLimit(t *testing.T) {
	startedAt := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	fake := &fakeIngestionRepository{
		runs: []repository.IngestionRun{
			{
				ID:            7,
				SourceType:    "dataset",
				SourceName:    "pheme_large",
				DatasetName:   "pheme",
				DatasetSplit:  "eventcv_large",
				Status:        "completed",
				StartedAt:     startedAt,
				PostsIngested: 1000,
			},
		},
	}

	rr := httptest.NewRecorder()
	NewIngestionHandler(fake).ListRuns().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/ingestion/runs", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if fake.limit != 20 {
		t.Fatalf("expected default limit 20, got %d", fake.limit)
	}

	var payload struct {
		Count int                       `json:"count"`
		Items []repository.IngestionRun `json:"items"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Count != 1 || payload.Items[0].ID != 7 || payload.Items[0].PostsIngested != 1000 {
		t.Fatalf("unexpected response: %#v", payload)
	}
}

func TestIngestionHandlerListRunsParsesLimit(t *testing.T) {
	fake := &fakeIngestionRepository{}

	rr := httptest.NewRecorder()
	NewIngestionHandler(fake).ListRuns().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/ingestion/runs?limit=5", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if fake.limit != 5 {
		t.Fatalf("expected limit 5, got %d", fake.limit)
	}
}

func TestIngestionHandlerListRunsRejectsInvalidLimit(t *testing.T) {
	fake := &fakeIngestionRepository{}

	for _, target := range []string{"/api/ingestion/runs?limit=0", "/api/ingestion/runs?limit=201", "/api/ingestion/runs?limit=abc"} {
		rr := httptest.NewRecorder()
		NewIngestionHandler(fake).ListRuns().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, target, nil))

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected status 400, got %d", target, rr.Code)
		}
	}
	if fake.called {
		t.Fatalf("repository should not be called for invalid limit")
	}
}

func TestIngestionHandlerListRunsMapsRepositoryError(t *testing.T) {
	fake := &fakeIngestionRepository{err: errors.New("db down")}

	rr := httptest.NewRecorder()
	NewIngestionHandler(fake).ListRuns().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/ingestion/runs", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
}
