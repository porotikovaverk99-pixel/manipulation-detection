// Package handler предоставляет HTTP обработчики для API.
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

// dashboardRepository defines interface for dashboard data access
type dashboardRepository interface {
	GetExtendedDashboardMetrics(repository.DashboardFilter) (repository.ExtendedDashboardMetrics, error)
	GetRadarDataForCase(int64, string) ([]repository.RadarDataPoint, error)
}

// DashboardHandler отдает агрегаты для аналитического дашборда
type DashboardHandler struct {
	repo dashboardRepository
}

// NewDashboardHandler создаёт новый обработчик дашборда
func NewDashboardHandler(repo dashboardRepository) *DashboardHandler {
	return &DashboardHandler{repo: repo}
}

// Metrics возвращает расширенные метрики для дашборда (включая 3 ветки анализа)
func (h *DashboardHandler) Metrics() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Парсинг параметров запроса
		filter := repository.DashboardFilter{
			SourceName:   r.URL.Query().Get("source_name"),
			DatasetName:  r.URL.Query().Get("dataset_name"),
			DatasetSplit: r.URL.Query().Get("dataset_split"),
			EventName:    r.URL.Query().Get("event_name"),
			Label:        r.URL.Query().Get("label"),
			Status:       r.URL.Query().Get("status"),
			RiskLevel:    r.URL.Query().Get("risk_level"),
			ScorerKey:    r.URL.Query().Get("scorer_key"),
		}

		// Парсинг количества дней
		if raw := r.URL.Query().Get("days"); raw != "" {
			days, err := strconv.Atoi(raw)
			if err != nil || days <= 0 || days > 365 {
				http.Error(w, "invalid days parameter", http.StatusBadRequest)
				return
			}
			filter.Days = days
		}

		// Получение метрик из базы данных
		metrics, err := h.repo.GetExtendedDashboardMetrics(filter)
		if err != nil {
			http.Error(w, "failed to fetch dashboard metrics: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Отправка ответа
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(metrics)
	}
}

// RadarData возвращает данные для радар-диаграммы по конкретному кейсу
func (h *DashboardHandler) RadarData() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Извлекаем ID из пути /api/dashboard/radar/{id}
		pathParts := strings.Split(r.URL.Path, "/")
		var caseID int64
		for i, part := range pathParts {
			if part == "radar" && i+1 < len(pathParts) {
				id, err := strconv.ParseInt(pathParts[i+1], 10, 64)
				if err == nil {
					caseID = id
				}
				break
			}
		}

		if caseID == 0 {
			http.Error(w, "case id required", http.StatusBadRequest)
			return
		}

		scorerKey := r.URL.Query().Get("scorer_key")
		radarData, err := h.repo.GetRadarDataForCase(caseID, scorerKey)
		if err != nil {
			http.Error(w, "failed to fetch radar data: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(radarData)
	}
}

// Summary возвращает краткую сводку для дашборда
func (h *DashboardHandler) Summary() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := repository.DashboardFilter{
			SourceName:   r.URL.Query().Get("source_name"),
			DatasetName:  r.URL.Query().Get("dataset_name"),
			DatasetSplit: r.URL.Query().Get("dataset_split"),
			EventName:    r.URL.Query().Get("event_name"),
			Label:        r.URL.Query().Get("label"),
			Status:       r.URL.Query().Get("status"),
			RiskLevel:    r.URL.Query().Get("risk_level"),
			ScorerKey:    r.URL.Query().Get("scorer_key"),
		}

		if raw := r.URL.Query().Get("days"); raw != "" {
			days, err := strconv.Atoi(raw)
			if err == nil && days > 0 && days <= 365 {
				filter.Days = days
			}
		}

		metrics, err := h.repo.GetExtendedDashboardMetrics(filter)
		if err != nil {
			http.Error(w, "failed to fetch dashboard summary: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Возвращаем только summary и manipulation_stats
		response := map[string]interface{}{
			"summary":            metrics.Summary,
			"manipulation_stats": metrics.ManipulationStats,
			"risk_distribution":  metrics.RiskDistribution,
			"top_tactics":        metrics.TopTactics,
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

// RecentEvidence возвращает последние карточки доказательств
func (h *DashboardHandler) RecentEvidence() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := repository.DashboardFilter{
			SourceName:   r.URL.Query().Get("source_name"),
			DatasetName:  r.URL.Query().Get("dataset_name"),
			DatasetSplit: r.URL.Query().Get("dataset_split"),
			EventName:    r.URL.Query().Get("event_name"),
			Label:        r.URL.Query().Get("label"),
			Status:       r.URL.Query().Get("status"),
		}

		if raw := r.URL.Query().Get("days"); raw != "" {
			days, err := strconv.Atoi(raw)
			if err == nil && days > 0 && days <= 365 {
				filter.Days = days
			}
		}

		metrics, err := h.repo.GetExtendedDashboardMetrics(filter)
		if err != nil {
			http.Error(w, "failed to fetch evidence: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(metrics.RecentEvidence)
	}
}

// BranchTimeSeries возвращает динамику трёх веток по дням
func (h *DashboardHandler) BranchTimeSeries() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := repository.DashboardFilter{
			SourceName:   r.URL.Query().Get("source_name"),
			DatasetName:  r.URL.Query().Get("dataset_name"),
			DatasetSplit: r.URL.Query().Get("dataset_split"),
			EventName:    r.URL.Query().Get("event_name"),
			Label:        r.URL.Query().Get("label"),
			Status:       r.URL.Query().Get("status"),
		}

		if raw := r.URL.Query().Get("days"); raw != "" {
			days, err := strconv.Atoi(raw)
			if err == nil && days > 0 && days <= 365 {
				filter.Days = days
			}
		}

		metrics, err := h.repo.GetExtendedDashboardMetrics(filter)
		if err != nil {
			http.Error(w, "failed to fetch branch time series: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(metrics.BranchTimeSeries)
	}
}
