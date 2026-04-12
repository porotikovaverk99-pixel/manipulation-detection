// Package handler предоставляет HTTP обработчики для API.
package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"
)

// AnalyzeHandler обрабатывает запросы на анализ текста.
type AnalyzeHandler struct {
	mlURL      string
	httpClient *http.Client
}

// AnalyzeRequest представляет запрос на анализ текста.
type AnalyzeRequest struct {
	Text   string `json:"text"`
	Source string `json:"source,omitempty"`
}

// MLRequest представляет запрос к ML сервису.
type MLRequest struct {
	Text     string `json:"text"`
	Language string `json:"language"`
}

// MLResponse представляет ответ от ML сервиса.
type MLResponse struct {
	ManipulationScore        float64  `json:"manipulation_score"`
	ConfidenceScore          float64  `json:"confidence_score"`
	CoordinationContribution float64  `json:"coordination_contribution"`
	TemporalContribution     float64  `json:"temporal_contribution"`
	NarrativeContribution    float64  `json:"narrative_contribution"`
	ConfidenceNote           string   `json:"confidence_note"`
	KeyEvidence              []string `json:"key_evidence"`
	Tactics                  []string `json:"tactics"`
}

// AnalyzeResponse представляет ответ API.
type AnalyzeResponse struct {
	ManipulationScore        float64   `json:"manipulation_score"`
	ConfidenceScore          float64   `json:"confidence_score"`
	CoordinationContribution float64   `json:"coordination_contribution"`
	TemporalContribution     float64   `json:"temporal_contribution"`
	NarrativeContribution    float64   `json:"narrative_contribution"`
	EscalationPriority       int       `json:"escalation_priority"`
	ConfidenceNote           string    `json:"confidence_note"`
	KeyEvidence              []string  `json:"key_evidence"`
	Tactics                  []string  `json:"tactics"`
	AnalyzedAt               time.Time `json:"analyzed_at"`
}

// NewAnalyzeHandler создаёт новый обработчик анализа.
func NewAnalyzeHandler(mlURL string) *AnalyzeHandler {
	return &AnalyzeHandler{
		mlURL: mlURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Analyze выполняет анализ текста на манипуляции.
func (h *AnalyzeHandler) Analyze() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req AnalyzeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		if req.Text == "" {
			http.Error(w, "Text is required", http.StatusBadRequest)
			return
		}

		// Вызов ML сервиса
		mlReq := MLRequest{
			Text:     req.Text,
			Language: "ru",
		}
		mlBody, err := json.Marshal(mlReq)
		if err != nil {
			http.Error(w, "Failed to prepare request", http.StatusInternalServerError)
			return
		}

		resp, err := h.httpClient.Post(
			h.mlURL+"/predict",
			"application/json",
			bytes.NewBuffer(mlBody),
		)
		if err != nil {
			http.Error(w, "ML service unavailable: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			http.Error(w, "ML service error", http.StatusInternalServerError)
			return
		}

		var mlResp MLResponse
		if err := json.NewDecoder(resp.Body).Decode(&mlResp); err != nil {
			http.Error(w, "Failed to parse ML response", http.StatusInternalServerError)
			return
		}

		// Определение приоритета эскалации
		escalationPriority := 3
		if mlResp.ManipulationScore > 0.7 && mlResp.ConfidenceScore > 0.7 {
			escalationPriority = 1
		} else if mlResp.ManipulationScore > 0.4 {
			escalationPriority = 2
		}

		response := AnalyzeResponse{
			ManipulationScore:        mlResp.ManipulationScore,
			ConfidenceScore:          mlResp.ConfidenceScore,
			CoordinationContribution: mlResp.CoordinationContribution,
			TemporalContribution:     mlResp.TemporalContribution,
			NarrativeContribution:    mlResp.NarrativeContribution,
			EscalationPriority:       escalationPriority,
			ConfidenceNote:           mlResp.ConfidenceNote,
			KeyEvidence:              mlResp.KeyEvidence,
			Tactics:                  mlResp.Tactics,
			AnalyzedAt:               time.Now(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}
