// Package collector предоставляет функциональность для анализа данных.
package collector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
	PostID           int64  `json:"post_id"`
	Content          string `json:"content"`
	AccountID        int64  `json:"account_id"`
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

// CaseMLPost представляет один пост внутри case-level ML запроса.
type CaseMLPost struct {
	PostID           int64    `json:"post_id"`
	ExternalID       string   `json:"external_id"`
	AccountID        int64    `json:"account_id"`
	Username         string   `json:"username"`
	PublishedAt      string   `json:"published_at"`
	Content          string   `json:"content"`
	IsCaseRoot       bool     `json:"is_case_root"`
	ReplyToPostID    *int64   `json:"reply_to_post_id,omitempty"`
	LikesCount       int      `json:"likes_count"`
	RepostsCount     int      `json:"reposts_count"`
	RepliesCount     int      `json:"replies_count"`
	FollowersCount   int      `json:"followers_count"`
	FollowingCount   int      `json:"following_count"`
	PostsCount       int      `json:"posts_count"`
	IsVerified       bool     `json:"is_verified"`
	AccountCreatedAt string   `json:"account_created_at,omitempty"`
	AccountURL       string   `json:"account_url,omitempty"`
	Tags             []string `json:"tags"`
	Links            []string `json:"links"`
}

// CaseMLRequest представляет case-level запрос к ML сервису.
type CaseMLRequest struct {
	CaseID         int64        `json:"case_id"`
	ExternalCaseID string       `json:"external_case_id"`
	SourceName     string       `json:"source_name"`
	DatasetName    string       `json:"dataset_name,omitempty"`
	DatasetSplit   string       `json:"dataset_split,omitempty"`
	CaseType       string       `json:"case_type"`
	Label          string       `json:"label,omitempty"`
	Title          string       `json:"title,omitempty"`
	EventName      string       `json:"event_name,omitempty"`
	FirstEventAt   string       `json:"first_event_at,omitempty"`
	LastEventAt    string       `json:"last_event_at,omitempty"`
	Posts          []CaseMLPost `json:"posts"`
}

// CaseMLResponse представляет ответ case-level ML анализа.
type CaseMLResponse struct {
	FeatureVersion       string                 `json:"feature_version"`
	ScoreVersion         string                 `json:"score_version"`
	PipelineHash         string                 `json:"pipeline_hash"`
	RiskScore            float64                `json:"risk_score"`
	RiskLevel            string                 `json:"risk_level"`
	ConfidenceScore      float64                `json:"confidence_score"`
	TemporalScore        float64                `json:"temporal_score"`
	CoordinationScore    float64                `json:"coordination_score"`
	ContentScore         float64                `json:"content_score"`
	EventCount           int                    `json:"event_count"`
	UniqueAccountCount   int                    `json:"unique_account_count"`
	UniqueURLCount       int                    `json:"unique_url_count"`
	UniqueHashtagCount   int                    `json:"unique_hashtag_count"`
	TemporalFeatures     map[string]interface{} `json:"temporal_features"`
	CoordinationFeatures map[string]interface{} `json:"coordination_features"`
	ContentFeatures      map[string]interface{} `json:"content_features"`
	FeaturePayload       map[string]interface{} `json:"feature_payload"`
	Evidence             []string               `json:"evidence"`
	ModelInfo            map[string]interface{} `json:"model_info"`
}

func (c *MLClient) AnalyzeText(postID int64, content string, accountID int64,
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

// AnalyzeCase выполняет case-level анализ через ML сервис.
func (c *MLClient) AnalyzeCase(req CaseMLRequest) (*CaseMLResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal case request: %w", err)
	}

	resp, err := c.httpClient.Post(
		c.mlURL+"/analyze/case",
		"application/json",
		bytes.NewBuffer(reqBody),
	)
	if err != nil {
		return nil, fmt.Errorf("ml case service request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("ml case service status: %d body=%s", resp.StatusCode, bytes.TrimSpace(body))
	}

	var mlResp CaseMLResponse
	if err := json.NewDecoder(resp.Body).Decode(&mlResp); err != nil {
		return nil, fmt.Errorf("decode case response: %w", err)
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
