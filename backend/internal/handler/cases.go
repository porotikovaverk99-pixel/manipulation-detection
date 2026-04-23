package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

// CasesHandler отдает case-level сущности и их score summary.
type CasesHandler struct {
	repo *repository.PostgresDB
}

func NewCasesHandler(repo *repository.PostgresDB) *CasesHandler {
	return &CasesHandler{repo: repo}
}

func (h *CasesHandler) List() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := repository.CaseFilter{
			SourceName:   r.URL.Query().Get("source_name"),
			DatasetName:  r.URL.Query().Get("dataset_name"),
			DatasetSplit: r.URL.Query().Get("dataset_split"),
			Label:        r.URL.Query().Get("label"),
			OnlyUnscored: false,
			Limit:        20,
		}

		if raw := r.URL.Query().Get("limit"); raw != "" {
			limit, err := strconv.Atoi(raw)
			if err != nil || limit <= 0 || limit > 500 {
				http.Error(w, "invalid limit", http.StatusBadRequest)
				return
			}
			filter.Limit = limit
		}
		if raw := r.URL.Query().Get("only_unscored"); raw != "" {
			switch raw {
			case "1", "true", "yes":
				filter.OnlyUnscored = true
			case "0", "false", "no":
				filter.OnlyUnscored = false
			default:
				http.Error(w, "invalid only_unscored", http.StatusBadRequest)
				return
			}
		}

		items, err := h.repo.ListCases(filter)
		if err != nil {
			http.Error(w, "failed to fetch cases", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"items": items,
			"count": len(items),
		})
	}
}
