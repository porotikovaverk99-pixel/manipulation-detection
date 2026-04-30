package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

type analysisRepository interface {
	GetAnalysisSummary(repository.AnalysisPostFilter) (repository.AnalysisSummary, error)
}

// AnalysisHandler отдает агрегаты по результатам batch/live анализа.
type AnalysisHandler struct {
	repo analysisRepository
}

func NewAnalysisHandler(repo analysisRepository) *AnalysisHandler {
	return &AnalysisHandler{repo: repo}
}

func (h *AnalysisHandler) Summary() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := repository.AnalysisPostFilter{
			SourceType:   r.URL.Query().Get("source_type"),
			DatasetName:  r.URL.Query().Get("dataset_name"),
			DatasetSplit: r.URL.Query().Get("dataset_split"),
		}

		if raw := r.URL.Query().Get("ingestion_run_id"); raw != "" {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || id <= 0 {
				http.Error(w, "invalid ingestion_run_id", http.StatusBadRequest)
				return
			}
			filter.IngestionRunID = &id
		}

		summary, err := h.repo.GetAnalysisSummary(filter)
		if err != nil {
			http.Error(w, "failed to fetch analysis summary", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(summary)
	}
}
