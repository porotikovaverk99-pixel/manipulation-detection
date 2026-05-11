package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

// ingestionRepository defines interface for ingestion data access
type ingestionRepository interface {
	GetIngestionRuns(limit int) ([]repository.IngestionRun, error)
}

// IngestionHandler отдает наблюдаемость по dataset/live ingestion запускам.
type IngestionHandler struct {
	repo ingestionRepository
}

// NewIngestionHandler создаёт новый обработчик ingestion
func NewIngestionHandler(repo ingestionRepository) *IngestionHandler {
	return &IngestionHandler{repo: repo}
}

// ListRuns возвращает список запусков ingestion
func (h *IngestionHandler) ListRuns() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 20
		if raw := r.URL.Query().Get("limit"); raw != "" {
			if v, err := strconv.Atoi(raw); err == nil && v > 0 && v <= 200 {
				limit = v
			}
		}

		runs, err := h.repo.GetIngestionRuns(limit)
		if err != nil {
			http.Error(w, "failed to fetch ingestion runs", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"items": runs,
			"count": len(runs),
		})
	}
}
