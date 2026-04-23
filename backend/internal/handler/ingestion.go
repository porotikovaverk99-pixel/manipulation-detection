package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

// IngestionHandler отдает наблюдаемость по dataset/live ingestion запускам.
type IngestionHandler struct {
	repo *repository.PostgresDB
}

func NewIngestionHandler(repo *repository.PostgresDB) *IngestionHandler {
	return &IngestionHandler{repo: repo}
}

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
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"items": runs,
			"count": len(runs),
		})
	}
}
