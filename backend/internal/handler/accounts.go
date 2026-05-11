package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

type accountsRepository interface {
	ListAccounts(repository.AccountFilter) (repository.AccountsListResponse, error)
}

// AccountsHandler exposes account-level analytics.
type AccountsHandler struct {
	repo accountsRepository
}

func NewAccountsHandler(repo accountsRepository) *AccountsHandler {
	return &AccountsHandler{repo: repo}
}

func (h *AccountsHandler) List() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page := parseIntQuery(r, "page", 1)
		limit := parseIntQuery(r, "limit", 50)
		if page <= 0 {
			page = 1
		}
		if limit <= 0 || limit > 200 {
			limit = 50
		}
		filter := repository.AccountFilter{
			DatasetName:  r.URL.Query().Get("dataset_name"),
			DatasetSplit: r.URL.Query().Get("dataset_split"),
			Search:       r.URL.Query().Get("search"),
			OnlyVerified: parseBoolQuery(r.URL.Query().Get("verified")),
			OnlyBots:     parseBoolQuery(r.URL.Query().Get("bots")),
			Sort:         r.URL.Query().Get("sort"),
			Limit:        limit,
			Offset:       (page - 1) * limit,
		}
		result, err := h.repo.ListAccounts(filter)
		if err != nil {
			http.Error(w, "failed to fetch accounts", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	}
}

func parseIntQuery(r *http.Request, key string, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func parseBoolQuery(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
