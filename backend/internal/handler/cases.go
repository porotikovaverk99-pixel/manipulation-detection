package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

type caseRepository interface {
	ListCases(repository.CaseFilter) ([]repository.CaseListItem, error)
	GetCasesSummary(repository.CaseFilter) (repository.CasesSummary, error)
	GetCaseDetails(int64, string) (repository.CaseDetails, error)
	ListCaseModelScores(int64) ([]repository.CaseModelScoreItem, error)
	GetModelComparison(repository.ModelComparisonFilter) (repository.ModelComparisonSummary, error)
	RecordCaseDecision(int64, string) (repository.DecisionRecord, error)
}

// CasesHandler отдает case-level сущности и их score summary.
type CasesHandler struct {
	repo caseRepository
}

func NewCasesHandler(repo caseRepository) *CasesHandler {
	return &CasesHandler{repo: repo}
}

func (h *CasesHandler) List() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, limit, err := parsePagination(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		filter := caseFilterFromRequest(r)
		filter.Limit = limit
		filter.Offset = (page - 1) * limit
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
			"page":  page,
			"limit": limit,
		})
	}
}

func (h *CasesHandler) Summary() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, limit, err := parsePagination(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		filter := caseFilterFromRequest(r)
		summary, err := h.repo.GetCasesSummary(filter)
		if err != nil {
			http.Error(w, "failed to fetch cases summary", http.StatusInternalServerError)
			return
		}

		pages := 0
		if summary.TotalCases > 0 {
			pages = (summary.TotalCases + limit - 1) / limit
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"total_cases": summary.TotalCases,
			"high_risk":   summary.HighRisk,
			"medium_risk": summary.MediumRisk,
			"low_risk":    summary.LowRisk,
			"mean_risk":   summary.MeanRisk,
			"limit":       limit,
			"pages":       pages,
		})
	}
}

func (h *CasesHandler) Detail() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseCaseID(r)
		if err != nil {
			http.Error(w, "invalid case id", http.StatusBadRequest)
			return
		}

		item, err := h.repo.GetCaseDetails(id, r.URL.Query().Get("scorer_key"))
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "case not found", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to fetch case details", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(item)
	}
}

func (h *CasesHandler) Scores() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseCaseID(r)
		if err != nil {
			http.Error(w, "invalid case id", http.StatusBadRequest)
			return
		}

		items, err := h.repo.ListCaseModelScores(id)
		if err != nil {
			http.Error(w, "failed to fetch case scores", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"case_id": id,
			"items":   items,
			"count":   len(items),
		})
	}
}

func (h *CasesHandler) Decision() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseCaseID(r)
		if err != nil {
			http.Error(w, "invalid case id", http.StatusBadRequest)
			return
		}
		var payload struct {
			Decision string `json:"decision"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid decision payload", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(payload.Decision) == "" {
			http.Error(w, "decision is required", http.StatusBadRequest)
			return
		}
		record, err := h.repo.RecordCaseDecision(id, payload.Decision)
		if err != nil {
			http.Error(w, "failed to record decision", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(record)
	}
}

func (h *CasesHandler) ModelComparison() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := repository.ModelComparisonFilter{
			SourceName:     r.URL.Query().Get("source_name"),
			DatasetName:    r.URL.Query().Get("dataset_name"),
			DatasetSplit:   r.URL.Query().Get("dataset_split"),
			ScorerKeys:     parseCSVQuery(r.URL.Query().Get("scorer_keys")),
			PositiveLabels: parseCSVQuery(r.URL.Query().Get("positive_labels")),
			TopK:           20,
		}

		if raw := r.URL.Query().Get("top_k"); raw != "" {
			topK, err := strconv.Atoi(raw)
			if err != nil || topK <= 0 || topK > 100 {
				http.Error(w, "invalid top_k", http.StatusBadRequest)
				return
			}
			filter.TopK = topK
		}

		summary, err := h.repo.GetModelComparison(filter)
		if err != nil {
			http.Error(w, "failed to fetch model comparison", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(summary)
	}
}

func parseCaseID(r *http.Request) (int64, error) {
	rawID := chi.URLParam(r, "id")
	return strconv.ParseInt(rawID, 10, 64)
}

func caseFilterFromRequest(r *http.Request) repository.CaseFilter {
	return repository.CaseFilter{
		SourceName:   r.URL.Query().Get("source_name"),
		DatasetName:  r.URL.Query().Get("dataset_name"),
		DatasetSplit: r.URL.Query().Get("dataset_split"),
		EventName:    r.URL.Query().Get("event_name"),
		Status:       r.URL.Query().Get("status"),
		Label:        r.URL.Query().Get("label"),
		RiskLevel:    r.URL.Query().Get("risk_level"),
		ScorerKey:    r.URL.Query().Get("scorer_key"),
		OnlyUnscored: false,
	}
}

func parsePagination(r *http.Request) (int, int, error) {
	page := 1
	limit := 20
	if raw := r.URL.Query().Get("page"); raw != "" {
		parsedPage, err := strconv.Atoi(raw)
		if err != nil || parsedPage <= 0 {
			return 0, 0, errInvalidQuery("invalid page")
		}
		page = parsedPage
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsedLimit, err := strconv.Atoi(raw)
		if err != nil || parsedLimit <= 0 || parsedLimit > 500 {
			return 0, 0, errInvalidQuery("invalid limit")
		}
		limit = parsedLimit
	}
	return page, limit, nil
}

type errInvalidQuery string

func (e errInvalidQuery) Error() string {
	return string(e)
}

func parseCSVQuery(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
