package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

// DashboardHandler отдает агрегаты для аналитического дашборда.
type DashboardHandler struct {
	repo *repository.PostgresDB
}

func NewDashboardHandler(repo *repository.PostgresDB) *DashboardHandler {
	return &DashboardHandler{repo: repo}
}

func (h *DashboardHandler) Metrics() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := repository.DashboardFilter{
			SourceName:   r.URL.Query().Get("source_name"),
			DatasetName:  r.URL.Query().Get("dataset_name"),
			DatasetSplit: r.URL.Query().Get("dataset_split"),
			EventName:    r.URL.Query().Get("event_name"),
			Label:        r.URL.Query().Get("label"),
			Status:       r.URL.Query().Get("status"),
			ScorerKey:    r.URL.Query().Get("scorer_key"),
			Days:         30,
		}

		if raw := r.URL.Query().Get("days"); raw != "" {
			days, err := strconv.Atoi(raw)
			if err != nil || days <= 0 || days > 365 {
				http.Error(w, "invalid days", http.StatusBadRequest)
				return
			}
			filter.Days = days
		}

		metrics, err := h.repo.GetDashboardMetrics(filter)
		if err != nil {
			http.Error(w, "failed to fetch dashboard metrics", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(metrics)
	}
}
