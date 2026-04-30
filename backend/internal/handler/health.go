package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type healthDB interface {
	Ping() error
}

type HealthHandler struct {
	db         healthDB
	mlURL      string
	httpClient *http.Client
}

type HealthResponse struct {
	Status    string                 `json:"status"`
	Service   string                 `json:"service"`
	CheckedAt time.Time              `json:"checked_at"`
	Checks    map[string]HealthCheck `json:"checks"`
}

type HealthCheck struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func NewHealthHandler(db healthDB, mlURL string) *HealthHandler {
	return &HealthHandler{
		db:    db,
		mlURL: strings.TrimRight(mlURL, "/"),
		httpClient: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
}

func (h *HealthHandler) Health() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		checks := map[string]HealthCheck{
			"database":   h.databaseCheck(),
			"ml_service": h.mlServiceCheck(),
		}

		status := "ok"
		httpStatus := http.StatusOK
		for _, check := range checks {
			if check.Status != "ok" {
				status = "degraded"
				httpStatus = http.StatusServiceUnavailable
				break
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(httpStatus)
		_ = json.NewEncoder(w).Encode(HealthResponse{
			Status:    status,
			Service:   "backend-api",
			CheckedAt: time.Now().UTC(),
			Checks:    checks,
		})
	}
}

func (h *HealthHandler) databaseCheck() HealthCheck {
	if h.db == nil {
		return HealthCheck{Status: "fail", Message: "database is not configured"}
	}
	if err := h.db.Ping(); err != nil {
		return HealthCheck{Status: "fail", Message: err.Error()}
	}
	return HealthCheck{Status: "ok"}
}

func (h *HealthHandler) mlServiceCheck() HealthCheck {
	if h.mlURL == "" {
		return HealthCheck{Status: "fail", Message: "ML service URL is not configured"}
	}
	resp, err := h.httpClient.Get(h.mlURL + "/health")
	if err != nil {
		return HealthCheck{Status: "fail", Message: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return HealthCheck{Status: "fail", Message: resp.Status}
	}
	return HealthCheck{Status: "ok"}
}
