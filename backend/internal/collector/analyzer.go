// Package collector предоставляет функциональность для анализа данных.
package collector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// MLClient представляет клиент для взаимодействия с ML сервисом.
type MLClient struct {
	mlURL      string
	httpClient *http.Client
}

// NewMLClient создаёт новый экземпляр клиента ML сервиса.
func NewMLClient(mlURL string) *MLClient {
	return &MLClient{
		mlURL: mlURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// MLRequest представляет запрос к ML сервису.
type MLRequest struct {
	PostID           int    `json:"post_id"`
	Content          string `json:"content"`
	AccountID        int    `json:"account_id"`
	Username         string `json:"username"`
	PublishedAt      string `json:"published_at"`
	FollowersCount   int    `json:"followers_count"`
	AccountCreatedAt string `json:"account_created_at,omitempty"`
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

func (c *MLClient) AnalyzeText(postID int, content string, accountID int,
	username string, publishedAt time.Time,
	followersCount int, accountCreatedAt time.Time) (*MLResponse, error) {

	req := MLRequest{
		PostID:         postID,
		Content:        content,
		AccountID:      accountID,
		Username:       username,
		PublishedAt:    publishedAt.Format(time.RFC3339),
		FollowersCount: followersCount,
	}

	if !accountCreatedAt.IsZero() {
		req.AccountCreatedAt = accountCreatedAt.Format(time.RFC3339)
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(
		c.mlURL+"/analyze/advanced",
		"application/json",
		bytes.NewBuffer(reqBody),
	)
	if err != nil {
		return nil, fmt.Errorf("ml service request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ml service status: %d", resp.StatusCode)
	}

	var mlResp MLResponse
	if err := json.NewDecoder(resp.Body).Decode(&mlResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &mlResp, nil
}

// GetEscalationPriority определяет приоритет эскалации на основе оценки.
func GetEscalationPriority(manipulationScore, confidenceScore float64) int {
	if manipulationScore > 0.7 && confidenceScore > 0.7 {
		return 1
	}
	if manipulationScore > 0.4 {
		return 2
	}
	return 3
}
