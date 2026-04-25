package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

type fakeCaseRepository struct {
	listFilter         repository.CaseFilter
	detailCaseID       int64
	detailScorerKey    string
	scoresCaseID       int64
	comparisonFilter   repository.ModelComparisonFilter
	listCasesResult    []repository.CaseListItem
	caseDetailsResult  repository.CaseDetails
	caseScoresResult   []repository.CaseModelScoreItem
	comparisonResult   repository.ModelComparisonSummary
	listCasesErr       error
	caseDetailsErr     error
	caseScoresErr      error
	modelComparisonErr error
}

func (f *fakeCaseRepository) ListCases(filter repository.CaseFilter) ([]repository.CaseListItem, error) {
	f.listFilter = filter
	return f.listCasesResult, f.listCasesErr
}

func (f *fakeCaseRepository) GetCaseDetails(caseID int64, scorerKey string) (repository.CaseDetails, error) {
	f.detailCaseID = caseID
	f.detailScorerKey = scorerKey
	return f.caseDetailsResult, f.caseDetailsErr
}

func (f *fakeCaseRepository) ListCaseModelScores(caseID int64) ([]repository.CaseModelScoreItem, error) {
	f.scoresCaseID = caseID
	return f.caseScoresResult, f.caseScoresErr
}

func (f *fakeCaseRepository) GetModelComparison(filter repository.ModelComparisonFilter) (repository.ModelComparisonSummary, error) {
	f.comparisonFilter = filter
	return f.comparisonResult, f.modelComparisonErr
}

func TestCasesHandlerListParsesFilters(t *testing.T) {
	riskScore := 0.91
	fake := &fakeCaseRepository{
		listCasesResult: []repository.CaseListItem{
			{
				ID:           236,
				SourceName:   "pheme_large",
				DatasetName:  "pheme",
				DatasetSplit: "eventcv_large",
				ScorerKey:    "case_ensemble_v1",
				RiskScore:    &riskScore,
				RiskLevel:    "high",
			},
		},
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/cases?source_name=pheme_large&dataset_name=pheme&dataset_split=eventcv_large&label=rumour&risk_level=high&scorer_key=case_ensemble_v1&only_unscored=true&limit=5",
		nil,
	)
	rr := httptest.NewRecorder()

	NewCasesHandler(fake).List().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	wantFilter := repository.CaseFilter{
		SourceName:   "pheme_large",
		DatasetName:  "pheme",
		DatasetSplit: "eventcv_large",
		Label:        "rumour",
		RiskLevel:    "high",
		ScorerKey:    "case_ensemble_v1",
		OnlyUnscored: true,
		Limit:        5,
	}
	if !reflect.DeepEqual(fake.listFilter, wantFilter) {
		t.Fatalf("unexpected filter:\nwant %#v\ngot  %#v", wantFilter, fake.listFilter)
	}

	var payload struct {
		Count int                       `json:"count"`
		Items []repository.CaseListItem `json:"items"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Count != 1 || len(payload.Items) != 1 || payload.Items[0].ID != 236 {
		t.Fatalf("unexpected list response: %#v", payload)
	}
}

func TestCasesHandlerListRejectsInvalidLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/cases?limit=9999", nil)
	rr := httptest.NewRecorder()

	NewCasesHandler(&fakeCaseRepository{}).List().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

func TestCasesHandlerDetailReturnsNotFound(t *testing.T) {
	fake := &fakeCaseRepository{caseDetailsErr: sql.ErrNoRows}
	router := chi.NewRouter()
	router.Get("/api/cases/{id}", NewCasesHandler(fake).Detail())

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/cases/404", nil))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
}

func TestCasesHandlerDetailPassesCaseIDAndScorer(t *testing.T) {
	fake := &fakeCaseRepository{
		caseDetailsResult: repository.CaseDetails{
			Case: repository.CaseListItem{ID: 236, ScorerKey: "case_ensemble_v1"},
		},
	}
	router := chi.NewRouter()
	router.Get("/api/cases/{id}", NewCasesHandler(fake).Detail())

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/cases/236?scorer_key=case_ensemble_v1", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if fake.detailCaseID != 236 || fake.detailScorerKey != "case_ensemble_v1" {
		t.Fatalf("unexpected detail args: caseID=%d scorer=%q", fake.detailCaseID, fake.detailScorerKey)
	}
}

func TestCasesHandlerScoresReturnsItems(t *testing.T) {
	fake := &fakeCaseRepository{
		caseScoresResult: []repository.CaseModelScoreItem{
			{CaseID: 236, ScorerKey: "case_ensemble_v1", RiskScore: 0.93, RiskLevel: "high"},
		},
	}
	router := chi.NewRouter()
	router.Get("/api/cases/{id}/scores", NewCasesHandler(fake).Scores())

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/cases/236/scores", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if fake.scoresCaseID != 236 {
		t.Fatalf("expected scores case id 236, got %d", fake.scoresCaseID)
	}

	var payload struct {
		CaseID int64                           `json:"case_id"`
		Count  int                             `json:"count"`
		Items  []repository.CaseModelScoreItem `json:"items"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.CaseID != 236 || payload.Count != 1 || payload.Items[0].ScorerKey != "case_ensemble_v1" {
		t.Fatalf("unexpected scores response: %#v", payload)
	}
}

func TestCasesHandlerModelComparisonParsesQuery(t *testing.T) {
	var summary repository.ModelComparisonSummary
	summary.CaseCount = 1000
	fake := &fakeCaseRepository{comparisonResult: summary}

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/model-comparison?source_name=pheme_large&dataset_name=pheme&dataset_split=eventcv_large&scorer_keys=a,b&positive_labels=rumour,rumor&top_k=7",
		nil,
	)
	rr := httptest.NewRecorder()

	NewCasesHandler(fake).ModelComparison().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	wantScorers := []string{"a", "b"}
	wantLabels := []string{"rumour", "rumor"}
	if fake.comparisonFilter.SourceName != "pheme_large" ||
		fake.comparisonFilter.DatasetName != "pheme" ||
		fake.comparisonFilter.DatasetSplit != "eventcv_large" ||
		fake.comparisonFilter.TopK != 7 ||
		!reflect.DeepEqual(fake.comparisonFilter.ScorerKeys, wantScorers) ||
		!reflect.DeepEqual(fake.comparisonFilter.PositiveLabels, wantLabels) {
		t.Fatalf("unexpected comparison filter: %#v", fake.comparisonFilter)
	}
}

func TestCasesHandlerMapsRepositoryErrorsToServerError(t *testing.T) {
	fake := &fakeCaseRepository{listCasesErr: errors.New("db down")}
	rr := httptest.NewRecorder()

	NewCasesHandler(fake).List().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/cases", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
}
