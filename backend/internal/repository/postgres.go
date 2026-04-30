// Package repository предоставляет функции для работы с базой данных PostgreSQL.
package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/collector/mastodon"
)

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

// UnanalyzedPost представляет пост без анализа.
type UnanalyzedPost struct {
	ID          int64
	Content     string
	AccountID   int64     // ← Должно быть это поле
	PublishedAt time.Time // ← И это
}

// IngestionContext хранит канонические метаданные происхождения записи.
type IngestionContext struct {
	SourceType      string
	RawPayloadRef   string
	RawPayloadHash  string
	DatasetName     string
	DatasetSplit    string
	DatasetRecordID string
	IngestionRunID  *int64
}

// DatasetPost представляет канонический пост из dataset/replay источника.
type DatasetPost struct {
	SourceID          int
	ExternalID        string
	AccountExternalID string
	Username          string
	DisplayName       string
	Content           string
	Language          string
	PublishedAt       time.Time
	PostURL           string
	LikesCount        int
	RepostsCount      int
	RepliesCount      int
	RawPayloadRef     string
	RawPayloadHash    string
	DatasetName       string
	DatasetSplit      string
	DatasetRecordID   string
	IngestionRunID    *int64
	CaseID            *int64
	IsCaseRoot        bool
	ReplyToExternalID string
	FollowersCount    int
	FollowingCount    int
	IsVerified        bool
	Metadata          map[string]interface{}
	Tags              []string
	Links             []DatasetLink
}

// DatasetLink представляет ссылку, извлечённую из dataset-поста.
type DatasetLink struct {
	URL         string
	Domain      string
	ExpandedURL string
	Title       string
	Description string
	ImageURL    string
}

// CaseRecord представляет канонический case для dataset/live pipeline.
type CaseRecord struct {
	SourceType         string
	SourceName         string
	DatasetName        string
	DatasetSplit       string
	ExternalCaseID     string
	CaseType           string
	Label              string
	Title              string
	Description        string
	EventName          string
	RootPostExternalID string
	Status             string
	OpenedAt           *time.Time
	ClosedAt           *time.Time
	FirstEventAt       *time.Time
	LastEventAt        *time.Time
	Metadata           map[string]interface{}
}

// IngestionRun представляет агрегированную информацию о запуске ingest/replay.
type IngestionRun struct {
	ID            int64      `json:"id"`
	SourceType    string     `json:"source_type"`
	SourceName    string     `json:"source_name"`
	DatasetName   string     `json:"dataset_name,omitempty"`
	DatasetSplit  string     `json:"dataset_split,omitempty"`
	Status        string     `json:"status"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	Notes         string     `json:"notes,omitempty"`
	PostsIngested int        `json:"posts_ingested"`
}

// AnalysisPostFilter задает фильтр batch-анализа постов.
type AnalysisPostFilter struct {
	SourceType     string
	DatasetName    string
	DatasetSplit   string
	IngestionRunID *int64
	OnlyUnanalyzed bool
	Limit          int
}

// AnalysisSummary агрегирует результаты анализа по выбранному срезу данных.
type AnalysisSummary struct {
	SourceType     string  `json:"source_type,omitempty"`
	DatasetName    string  `json:"dataset_name,omitempty"`
	DatasetSplit   string  `json:"dataset_split,omitempty"`
	IngestionRunID *int64  `json:"ingestion_run_id,omitempty"`
	TotalAnalyzed  int     `json:"total_analyzed"`
	HighRisk       int     `json:"high_risk"`
	MediumRisk     int     `json:"medium_risk"`
	LowRisk        int     `json:"low_risk"`
	AverageScore   float64 `json:"average_score"`
}

// CaseFilter задает фильтр для case-level выборок.
type CaseFilter struct {
	SourceName            string
	DatasetName           string
	DatasetSplit          string
	Label                 string
	RiskLevel             string
	ScorerKey             string
	OnlyUnscored          bool
	MissingModelScoreKeys []string
	Limit                 int
}

// CasePostData представляет посты, входящие в case.
type CasePostData struct {
	ID               int64
	ExternalID       string
	AccountID        int64
	Username         string
	PublishedAt      time.Time
	Content          string
	IsCaseRoot       bool
	ReplyToPostID    *int64
	LikesCount       int
	RepostsCount     int
	RepliesCount     int
	FollowersCount   int
	FollowingCount   int
	PostsCount       int
	IsVerified       bool
	AccountCreatedAt *time.Time
	AccountURL       string
	Tags             []string
	Links            []string
}

// CaseForScoring представляет case и все его посты для расчета признаков.
type CaseForScoring struct {
	ID                 int64
	SourceName         string
	DatasetName        string
	DatasetSplit       string
	ExternalCaseID     string
	CaseType           string
	Label              string
	Title              string
	EventName          string
	RootPostExternalID string
	FirstEventAt       *time.Time
	LastEventAt        *time.Time
	Posts              []CasePostData
}

// CaseFeaturesRecord хранит вычисленные case-level признаки.
type CaseFeaturesRecord struct {
	CaseID               int64
	FeatureVersion       string
	EventCount           int
	UniqueAccountCount   int
	UniqueURLCount       int
	UniqueHashtagCount   int
	TemporalFeatures     map[string]interface{}
	CoordinationFeatures map[string]interface{}
	ContentFeatures      map[string]interface{}
	FeaturePayload       map[string]interface{}
}

// CaseScoreRecord хранит итоговый скор case-level анализа.
type CaseScoreRecord struct {
	CaseID            int64
	ScoreVersion      string
	TemporalScore     float64
	CoordinationScore float64
	ContentScore      float64
	RiskScore         float64
	RiskLevel         string
	Evidence          []string
	PipelineHash      string
}

// CaseModelScoreRecord хранит результат конкретной модели/скорера для case.
type CaseModelScoreRecord struct {
	CaseID            int64
	ScorerKey         string
	ModelVersion      string
	RiskScore         float64
	RiskLevel         string
	ConfidenceScore   *float64
	TemporalScore     *float64
	CoordinationScore *float64
	ContentScore      *float64
	Evidence          []string
	FeaturePayload    map[string]interface{}
	ModelInfo         map[string]interface{}
	PipelineHash      string
	SourceEndpoint    string
}

// CaseListItem представляет компактное представление case для API.
type CaseListItem struct {
	ID                int64      `json:"id"`
	SourceName        string     `json:"source_name"`
	DatasetName       string     `json:"dataset_name"`
	DatasetSplit      string     `json:"dataset_split"`
	ExternalCaseID    string     `json:"external_case_id"`
	CaseType          string     `json:"case_type"`
	Label             string     `json:"label,omitempty"`
	Title             string     `json:"title,omitempty"`
	EventName         string     `json:"event_name,omitempty"`
	Status            string     `json:"status"`
	FirstEventAt      *time.Time `json:"first_event_at,omitempty"`
	LastEventAt       *time.Time `json:"last_event_at,omitempty"`
	PostCount         int        `json:"post_count"`
	ScorerKey         string     `json:"scorer_key,omitempty"`
	ModelVersion      string     `json:"model_version,omitempty"`
	RiskScore         *float64   `json:"risk_score,omitempty"`
	RiskLevel         string     `json:"risk_level,omitempty"`
	TemporalScore     *float64   `json:"temporal_score,omitempty"`
	CoordinationScore *float64   `json:"coordination_score,omitempty"`
	ContentScore      *float64   `json:"content_score,omitempty"`
}

// CasePostItem представляет пост в case details API.
type CasePostItem struct {
	ID               int64      `json:"id"`
	ExternalID       string     `json:"external_id"`
	AccountID        int64      `json:"account_id"`
	Username         string     `json:"username"`
	PublishedAt      time.Time  `json:"published_at"`
	Content          string     `json:"content"`
	IsCaseRoot       bool       `json:"is_case_root"`
	ReplyToPostID    *int64     `json:"reply_to_post_id,omitempty"`
	LikesCount       int        `json:"likes_count"`
	RepostsCount     int        `json:"reposts_count"`
	RepliesCount     int        `json:"replies_count"`
	FollowersCount   int        `json:"followers_count"`
	FollowingCount   int        `json:"following_count"`
	PostsCount       int        `json:"posts_count"`
	IsVerified       bool       `json:"is_verified"`
	AccountCreatedAt *time.Time `json:"account_created_at,omitempty"`
	AccountURL       string     `json:"account_url,omitempty"`
	Tags             []string   `json:"tags,omitempty"`
	Links            []string   `json:"links,omitempty"`
}

// CaseAccountItem агрегирует участие аккаунта в case.
type CaseAccountItem struct {
	ID             int64      `json:"id"`
	ExternalID     string     `json:"external_id"`
	Username       string     `json:"username"`
	DisplayName    string     `json:"display_name,omitempty"`
	AccountURL     string     `json:"account_url,omitempty"`
	FollowersCount int        `json:"followers_count"`
	FollowingCount int        `json:"following_count"`
	PostsCount     int        `json:"posts_count"`
	IsBot          bool       `json:"is_bot"`
	IsVerified     bool       `json:"is_verified"`
	CasePostCount  int        `json:"case_post_count"`
	FirstPostAt    *time.Time `json:"first_post_at,omitempty"`
	LastPostAt     *time.Time `json:"last_post_at,omitempty"`
	HasRootPost    bool       `json:"has_root_post"`
}

// ArtifactCount хранит частотность shared artifacts внутри case.
type ArtifactCount struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// CaseArtifactsSummary агрегирует ссылки, домены и теги case.
type CaseArtifactsSummary struct {
	Tags    []ArtifactCount `json:"tags,omitempty"`
	URLs    []ArtifactCount `json:"urls,omitempty"`
	Domains []ArtifactCount `json:"domains,omitempty"`
}

// CaseFeaturesSnapshot представляет сохраненный feature snapshot case.
type CaseFeaturesSnapshot struct {
	FeatureVersion       string                 `json:"feature_version"`
	EventCount           int                    `json:"event_count"`
	UniqueAccountCount   int                    `json:"unique_account_count"`
	UniqueURLCount       int                    `json:"unique_url_count"`
	UniqueHashtagCount   int                    `json:"unique_hashtag_count"`
	TemporalFeatures     map[string]interface{} `json:"temporal_features"`
	CoordinationFeatures map[string]interface{} `json:"coordination_features"`
	ContentFeatures      map[string]interface{} `json:"content_features"`
	FeaturePayload       map[string]interface{} `json:"feature_payload"`
	ComputedAt           time.Time              `json:"computed_at"`
}

// CaseDetails собирает данные для frontend case details view.
type CaseDetails struct {
	Case      CaseListItem          `json:"case"`
	RootPost  *CasePostItem         `json:"root_post,omitempty"`
	Posts     []CasePostItem        `json:"posts"`
	Accounts  []CaseAccountItem     `json:"accounts"`
	Artifacts CaseArtifactsSummary  `json:"artifacts"`
	Features  *CaseFeaturesSnapshot `json:"features,omitempty"`
}

// CaseModelScoreItem представляет один score из case_model_scores.
type CaseModelScoreItem struct {
	CaseID            int64                  `json:"case_id"`
	ScorerKey         string                 `json:"scorer_key"`
	ModelVersion      string                 `json:"model_version"`
	RiskScore         float64                `json:"risk_score"`
	RiskLevel         string                 `json:"risk_level"`
	ConfidenceScore   *float64               `json:"confidence_score,omitempty"`
	TemporalScore     *float64               `json:"temporal_score,omitempty"`
	CoordinationScore *float64               `json:"coordination_score,omitempty"`
	ContentScore      *float64               `json:"content_score,omitempty"`
	Evidence          []string               `json:"evidence,omitempty"`
	FeaturePayload    map[string]interface{} `json:"feature_payload,omitempty"`
	ModelInfo         map[string]interface{} `json:"model_info,omitempty"`
	PipelineHash      string                 `json:"pipeline_hash,omitempty"`
	SourceEndpoint    string                 `json:"source_endpoint,omitempty"`
	ComputedAt        time.Time              `json:"computed_at"`
}

// ModelComparisonFilter задает срез для сравнения case_model_scores.
type ModelComparisonFilter struct {
	SourceName     string
	DatasetName    string
	DatasetSplit   string
	ScorerKeys     []string
	PositiveLabels []string
	TopK           int
}

// ModelMetrics хранит метрики качества scorer-а.
type ModelMetrics struct {
	BestThreshold *float64 `json:"best_threshold,omitempty"`
	PrecisionAt10 *float64 `json:"precision_at_10,omitempty"`
	PrecisionAt20 *float64 `json:"precision_at_20,omitempty"`
	Precision     *float64 `json:"precision,omitempty"`
	Recall        *float64 `json:"recall,omitempty"`
	F1            *float64 `json:"f1,omitempty"`
	ROCAUC        *float64 `json:"roc_auc,omitempty"`
	PRAUC         *float64 `json:"pr_auc,omitempty"`
}

// ModelComparisonModel хранит сравнение одного scorer-а.
type ModelComparisonModel struct {
	ScorerKey    string       `json:"scorer_key"`
	ModelVersion string       `json:"model_version,omitempty"`
	CaseCount    int          `json:"case_count"`
	Metrics      ModelMetrics `json:"metrics"`
}

// ModelComparisonTopCase хранит top case по выбранному ranking scorer-у.
type ModelComparisonTopCase struct {
	CaseID          int64              `json:"case_id"`
	ExternalCaseID  string             `json:"external_case_id,omitempty"`
	EventName       string             `json:"event_name,omitempty"`
	Label           string             `json:"label,omitempty"`
	RankingScore    float64            `json:"ranking_score"`
	ComponentScores map[string]float64 `json:"component_scores"`
}

// ModelComparisonSummary хранит runtime comparison по case_model_scores.
type ModelComparisonSummary struct {
	Dataset struct {
		SourceName   string `json:"source_name,omitempty"`
		DatasetName  string `json:"dataset_name,omitempty"`
		DatasetSplit string `json:"dataset_split,omitempty"`
	} `json:"dataset"`
	PositiveLabels []string                        `json:"positive_labels"`
	CaseCount      int                             `json:"case_count"`
	PositiveCases  int                             `json:"positive_cases"`
	NegativeCases  int                             `json:"negative_cases"`
	Scorers        []string                        `json:"scorers"`
	Models         map[string]ModelComparisonModel `json:"models"`
	TopCases       []ModelComparisonTopCase        `json:"top_cases"`
	GeneratedAt    time.Time                       `json:"generated_at"`
}

// ScoredCaseItem представляет case со score и метаданными для evaluation/export.
type ScoredCaseItem struct {
	ID                int64      `json:"id"`
	SourceName        string     `json:"source_name"`
	DatasetName       string     `json:"dataset_name"`
	DatasetSplit      string     `json:"dataset_split"`
	ExternalCaseID    string     `json:"external_case_id"`
	CaseType          string     `json:"case_type"`
	Label             string     `json:"label,omitempty"`
	Title             string     `json:"title,omitempty"`
	EventName         string     `json:"event_name,omitempty"`
	Status            string     `json:"status"`
	FirstEventAt      *time.Time `json:"first_event_at,omitempty"`
	LastEventAt       *time.Time `json:"last_event_at,omitempty"`
	PostCount         int        `json:"post_count"`
	FeatureVersion    string     `json:"feature_version,omitempty"`
	ScoreVersion      string     `json:"score_version,omitempty"`
	PipelineHash      string     `json:"pipeline_hash,omitempty"`
	RiskScore         float64    `json:"risk_score"`
	RiskLevel         string     `json:"risk_level,omitempty"`
	TemporalScore     float64    `json:"temporal_score"`
	CoordinationScore float64    `json:"coordination_score"`
	ContentScore      float64    `json:"content_score"`
	Evidence          []string   `json:"evidence,omitempty"`
}

// EvaluationRunRecord хранит агрегированные метрики case-level evaluation.
type EvaluationRunRecord struct {
	SourceName    string
	DatasetName   string
	DatasetSplit  string
	PipelineHash  string
	ScorerVersion string
	CaseCount     int
	PrecisionAt10 *float64
	PrecisionAt20 *float64
	Recall        *float64
	F1            *float64
	ROCAUC        *float64
	PRAUC         *float64
	Metrics       map[string]interface{}
	Notes         string
}

// AuditEventRecord хранит один persistent audit event для API/pipeline действий.
type AuditEventRecord struct {
	ActorType    string
	ActorID      string
	Action       string
	EntityType   string
	EntityID     string
	Status       string
	RequestID    string
	SourceType   string
	SourceName   string
	DatasetName  string
	DatasetSplit string
	Payload      map[string]interface{}
	ErrorMessage string
}

// PostgresDB представляет подключение к PostgreSQL.
type PostgresDB struct {
	db *sql.DB
}

// NewPostgresDB создаёт новое подключение к базе данных.
func NewPostgresDB(connStr string) (*PostgresDB, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("открытие БД: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("проверка соединения: %w", err)
	}

	log.Println("✅ Подключено к PostgreSQL")
	return &PostgresDB{db: db}, nil
}

// Close закрывает соединение с базой данных.
func (p *PostgresDB) Close() error {
	return p.db.Close()
}

// Ping проверяет доступность БД для health-check endpoint-ов.
func (p *PostgresDB) Ping() error {
	return p.db.Ping()
}

// EnsureDataSource возвращает id источника данных, создавая его при необходимости.
func (p *PostgresDB) EnsureDataSource(name, apiEndpoint string) (int, error) {
	if strings.TrimSpace(name) == "" {
		name = "dataset"
	}

	var id int
	err := p.db.QueryRow(`
		INSERT INTO data_sources (name, api_endpoint, is_active)
		VALUES ($1, $2, TRUE)
		ON CONFLICT (name) DO UPDATE SET
			api_endpoint = COALESCE(NULLIF(EXCLUDED.api_endpoint, ''), data_sources.api_endpoint),
			is_active = TRUE
		RETURNING id
	`, name, apiEndpoint).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("ensure data source: %w", err)
	}

	return id, nil
}

// UpsertCase создает или обновляет case-level сущность.
func (p *PostgresDB) UpsertCase(rec CaseRecord) (int64, error) {
	if strings.TrimSpace(rec.SourceType) == "" {
		rec.SourceType = "dataset"
	}
	if strings.TrimSpace(rec.SourceName) == "" {
		rec.SourceName = "dataset"
	}
	if strings.TrimSpace(rec.ExternalCaseID) == "" {
		return 0, fmt.Errorf("external case id is required")
	}
	if strings.TrimSpace(rec.CaseType) == "" {
		rec.CaseType = "thread"
	}
	if strings.TrimSpace(rec.Status) == "" {
		rec.Status = "closed"
	}

	openedAt := time.Now().UTC()
	if rec.OpenedAt != nil && !rec.OpenedAt.IsZero() {
		openedAt = rec.OpenedAt.UTC()
	} else if rec.FirstEventAt != nil && !rec.FirstEventAt.IsZero() {
		openedAt = rec.FirstEventAt.UTC()
	}

	metadataJSON, err := marshalJSONObject(rec.Metadata)
	if err != nil {
		return 0, fmt.Errorf("marshal case metadata: %w", err)
	}

	var id int64
	err = p.db.QueryRow(`
		INSERT INTO cases (
			source_type, source_name, dataset_name, dataset_split,
			external_case_id, case_type, label, title, description,
			event_name, root_post_external_id, status, opened_at,
			closed_at, first_event_at, last_event_at, metadata
		)
		VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8, $9,
			$10, $11, $12, $13,
			$14, $15, $16, $17
		)
		ON CONFLICT (source_type, source_name, dataset_name, dataset_split, external_case_id)
		DO UPDATE SET
			case_type = EXCLUDED.case_type,
			label = COALESCE(NULLIF(EXCLUDED.label, ''), cases.label),
			title = COALESCE(NULLIF(EXCLUDED.title, ''), cases.title),
			description = COALESCE(NULLIF(EXCLUDED.description, ''), cases.description),
			event_name = COALESCE(NULLIF(EXCLUDED.event_name, ''), cases.event_name),
			root_post_external_id = COALESCE(NULLIF(EXCLUDED.root_post_external_id, ''), cases.root_post_external_id),
			status = EXCLUDED.status,
			closed_at = COALESCE(EXCLUDED.closed_at, cases.closed_at),
			first_event_at = CASE
				WHEN cases.first_event_at IS NULL THEN EXCLUDED.first_event_at
				WHEN EXCLUDED.first_event_at IS NULL THEN cases.first_event_at
				ELSE LEAST(cases.first_event_at, EXCLUDED.first_event_at)
			END,
			last_event_at = CASE
				WHEN cases.last_event_at IS NULL THEN EXCLUDED.last_event_at
				WHEN EXCLUDED.last_event_at IS NULL THEN cases.last_event_at
				ELSE GREATEST(cases.last_event_at, EXCLUDED.last_event_at)
			END,
			metadata = CASE
				WHEN EXCLUDED.metadata = '{}'::jsonb THEN cases.metadata
				ELSE cases.metadata || EXCLUDED.metadata
			END,
			updated_at = NOW()
		RETURNING id
	`,
		rec.SourceType,
		rec.SourceName,
		rec.DatasetName,
		rec.DatasetSplit,
		rec.ExternalCaseID,
		rec.CaseType,
		rec.Label,
		rec.Title,
		rec.Description,
		rec.EventName,
		rec.RootPostExternalID,
		rec.Status,
		openedAt,
		rec.ClosedAt,
		rec.FirstEventAt,
		rec.LastEventAt,
		metadataJSON,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert case: %w", err)
	}

	return id, nil
}

// SaveAccount сохраняет аккаунт в базу данных.
func (p *PostgresDB) SaveAccount(sourceID int, acc *mastodon.Account) (int64, error) {
	var id int64

	metadata, _ := json.Marshal(map[string]interface{}{
		"acct":   acc.Acct,
		"url":    acc.URL,
		"avatar": acc.Avatar,
		"note":   acc.Note,
		"fields": acc.Fields,
	})

	query := `
		INSERT INTO accounts (source_id, external_id, username, display_name, 
		                      followers_count, following_count, posts_count, 
		                      created_at, collected_at, metadata, is_bot)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), $9, $10)
		ON CONFLICT (source_id, external_id) 
		DO UPDATE SET 
			followers_count = EXCLUDED.followers_count,
			following_count = EXCLUDED.following_count,
			posts_count = EXCLUDED.posts_count,
			collected_at = NOW()
		RETURNING id
	`

	err := p.db.QueryRow(query,
		sourceID,
		acc.ID,
		acc.Username,
		acc.DisplayName,
		acc.FollowersCount,
		acc.FollowingCount,
		acc.StatusesCount,
		acc.CreatedAt,
		metadata,
		acc.Bot,
	).Scan(&id)

	return id, err
}

// SaveDatasetPost сохраняет dataset-запись в канонические таблицы accounts/posts.
func (p *PostgresDB) SaveDatasetPost(post DatasetPost) error {
	if post.SourceID == 0 {
		post.SourceID = 1
	}

	if post.ExternalID == "" {
		post.ExternalID = fmt.Sprintf("dataset-%d", time.Now().UnixNano())
	}

	if post.Username == "" {
		post.Username = "dataset_actor"
	}

	if post.PublishedAt.IsZero() {
		post.PublishedAt = time.Now().UTC()
	}

	accountExternalID := strings.TrimSpace(post.AccountExternalID)
	if accountExternalID == "" {
		if post.Username != "" {
			accountExternalID = fmt.Sprintf("dataset:%s", post.Username)
		} else {
			accountExternalID = fmt.Sprintf("dataset-post:%s", post.ExternalID)
		}
	}
	if post.Username == "" {
		post.Username = accountExternalID
	}

	accountMetadata, err := marshalJSONObject(map[string]interface{}{
		"ingest": "dataset",
	})
	if err != nil {
		return fmt.Errorf("marshal dataset account metadata: %w", err)
	}

	var accountID int64
	err = p.db.QueryRow(`
		INSERT INTO accounts (
			source_id, external_id, username, display_name,
			created_at, collected_at, metadata,
			followers_count, following_count, is_verified
		)
		VALUES ($1, $2, $3, $4, NOW(), NOW(), $5, $6, $7, $8)
		ON CONFLICT (source_id, external_id)
		DO UPDATE SET
			username = EXCLUDED.username,
			display_name = EXCLUDED.display_name,
			followers_count = EXCLUDED.followers_count,
			following_count = EXCLUDED.following_count,
			is_verified = EXCLUDED.is_verified,
			last_seen_at = NOW()
		RETURNING id
	`,
		post.SourceID,
		accountExternalID,
		post.Username,
		post.DisplayName,
		accountMetadata,
		post.FollowersCount,
		post.FollowingCount,
		post.IsVerified,
	).Scan(&accountID)
	if err != nil {
		return fmt.Errorf("save dataset account: %w", err)
	}

	postMetadata := map[string]interface{}{
		"ingest": "dataset",
	}
	for k, v := range post.Metadata {
		postMetadata[k] = v
	}
	if strings.TrimSpace(post.ReplyToExternalID) != "" {
		postMetadata["reply_to_external_id"] = post.ReplyToExternalID
	}

	postMetadataJSON, err := marshalJSONObject(postMetadata)
	if err != nil {
		return fmt.Errorf("marshal dataset post metadata: %w", err)
	}

	var postID int64
	err = p.db.QueryRow(`
		INSERT INTO posts (
			source_id, external_id, account_id, content, language,
			published_at, collected_at, post_url, likes_count,
			reposts_count, replies_count, metadata, source_type,
			raw_payload_ref, raw_payload_hash, dataset_name,
			dataset_split, dataset_record_id, ingestion_run_id,
			case_id, is_case_root
		)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), $7, $8, $9, $10, $11, 'dataset', $12, $13, $14, $15, $16, $17, $18, $19)
		ON CONFLICT (source_id, external_id)
		DO UPDATE SET
			account_id = EXCLUDED.account_id,
			content = EXCLUDED.content,
			language = EXCLUDED.language,
			published_at = EXCLUDED.published_at,
			post_url = EXCLUDED.post_url,
			likes_count = EXCLUDED.likes_count,
			reposts_count = EXCLUDED.reposts_count,
			replies_count = EXCLUDED.replies_count,
			metadata = EXCLUDED.metadata,
			source_type = EXCLUDED.source_type,
			raw_payload_ref = EXCLUDED.raw_payload_ref,
			raw_payload_hash = EXCLUDED.raw_payload_hash,
			dataset_name = EXCLUDED.dataset_name,
			dataset_split = EXCLUDED.dataset_split,
			dataset_record_id = EXCLUDED.dataset_record_id,
			ingestion_run_id = EXCLUDED.ingestion_run_id,
			case_id = COALESCE(EXCLUDED.case_id, posts.case_id),
			is_case_root = posts.is_case_root OR EXCLUDED.is_case_root
		RETURNING id
	`,
		post.SourceID,
		post.ExternalID,
		accountID,
		post.Content,
		post.Language,
		post.PublishedAt,
		post.PostURL,
		post.LikesCount,
		post.RepostsCount,
		post.RepliesCount,
		postMetadataJSON,
		post.RawPayloadRef,
		post.RawPayloadHash,
		post.DatasetName,
		post.DatasetSplit,
		post.DatasetRecordID,
		post.IngestionRunID,
		post.CaseID,
		post.IsCaseRoot,
	).Scan(&postID)
	if err != nil {
		return fmt.Errorf("save dataset post: %w", err)
	}

	if strings.TrimSpace(post.ReplyToExternalID) != "" {
		var parentPostID int64
		err = p.db.QueryRow(`
			SELECT id
			FROM posts
			WHERE source_id = $1 AND external_id = $2
		`, post.SourceID, post.ReplyToExternalID).Scan(&parentPostID)
		if err == nil {
			if _, err := p.db.Exec(`
				UPDATE posts
				SET reply_to_post_id = $2
				WHERE id = $1
			`, postID, parentPostID); err != nil {
				return fmt.Errorf("link dataset reply: %w", err)
			}
		} else if err != sql.ErrNoRows {
			return fmt.Errorf("lookup dataset reply parent: %w", err)
		}
	}

	if _, err := p.db.Exec(`DELETE FROM post_tags WHERE post_id = $1`, postID); err != nil {
		return fmt.Errorf("clear dataset post tags: %w", err)
	}
	for _, tag := range dedupeStrings(post.Tags) {
		if strings.TrimSpace(tag) == "" {
			continue
		}
		if _, err := p.db.Exec(`
			INSERT INTO post_tags (post_id, tag_name)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, postID, tag); err != nil {
			return fmt.Errorf("save dataset post tag: %w", err)
		}
	}

	if _, err := p.db.Exec(`DELETE FROM post_links WHERE post_id = $1`, postID); err != nil {
		return fmt.Errorf("clear dataset post links: %w", err)
	}
	for _, link := range post.Links {
		if strings.TrimSpace(link.URL) == "" {
			continue
		}
		if _, err := p.db.Exec(`
			INSERT INTO post_links (post_id, url, domain, expanded_url, title, description, image_url)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, postID, link.URL, link.Domain, link.ExpandedURL, link.Title, link.Description, link.ImageURL); err != nil {
			return fmt.Errorf("save dataset post link: %w", err)
		}
	}

	return nil
}

// StartIngestionRun создает запись запуска ingestion/replay и возвращает run id.
func (p *PostgresDB) StartIngestionRun(sourceType, sourceName, datasetName, datasetSplit string) (int64, error) {
	var id int64
	err := p.db.QueryRow(`
		INSERT INTO ingestion_runs (source_type, source_name, dataset_name, dataset_split, status)
		VALUES ($1, $2, $3, $4, 'running')
		RETURNING id
	`, sourceType, sourceName, datasetName, datasetSplit).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("start ingestion run: %w", err)
	}
	return id, nil
}

// FinishIngestionRun закрывает ingestion/replay запуск.
func (p *PostgresDB) FinishIngestionRun(runID int64, status, notes string) error {
	if status == "" {
		status = "completed"
	}
	_, err := p.db.Exec(`
		UPDATE ingestion_runs
		SET status = $2, notes = $3, finished_at = NOW()
		WHERE id = $1
	`, runID, status, notes)
	if err != nil {
		return fmt.Errorf("finish ingestion run: %w", err)
	}
	return nil
}

// SaveAuditEvent сохраняет persistent audit event.
func (p *PostgresDB) SaveAuditEvent(event AuditEventRecord) (int64, error) {
	if strings.TrimSpace(event.ActorType) == "" {
		event.ActorType = "system"
	}
	if strings.TrimSpace(event.Status) == "" {
		event.Status = "succeeded"
	}
	if strings.TrimSpace(event.Action) == "" {
		return 0, fmt.Errorf("audit event action is required")
	}
	if event.Payload == nil {
		event.Payload = map[string]interface{}{}
	}

	payloadJSON, err := json.Marshal(event.Payload)
	if err != nil {
		return 0, fmt.Errorf("marshal audit payload: %w", err)
	}

	var id int64
	err = p.db.QueryRow(`
		INSERT INTO audit_events (
			actor_type,
			actor_id,
			action,
			entity_type,
			entity_id,
			status,
			request_id,
			source_type,
			source_name,
			dataset_name,
			dataset_split,
			payload,
			error_message
		)
		VALUES (
			$1, NULLIF($2, ''), $3, NULLIF($4, ''), NULLIF($5, ''), $6,
			NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''),
			NULLIF($11, ''), $12, NULLIF($13, '')
		)
		RETURNING id
	`,
		event.ActorType,
		event.ActorID,
		event.Action,
		event.EntityType,
		event.EntityID,
		event.Status,
		event.RequestID,
		event.SourceType,
		event.SourceName,
		event.DatasetName,
		event.DatasetSplit,
		payloadJSON,
		event.ErrorMessage,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("save audit event: %w", err)
	}

	return id, nil
}

// GetIngestionRuns возвращает последние ingestion/replay запуски с количеством постов.
func (p *PostgresDB) GetIngestionRuns(limit int) ([]IngestionRun, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := p.db.Query(`
		SELECT
			ir.id,
			ir.source_type,
			ir.source_name,
			COALESCE(ir.dataset_name, '') AS dataset_name,
			COALESCE(ir.dataset_split, '') AS dataset_split,
			ir.status,
			ir.started_at,
			ir.finished_at,
			COALESCE(ir.notes, '') AS notes,
			COUNT(p.id) AS posts_ingested
		FROM ingestion_runs ir
		LEFT JOIN posts p ON p.ingestion_run_id = ir.id
		GROUP BY ir.id
		ORDER BY ir.started_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("get ingestion runs: %w", err)
	}
	defer rows.Close()

	out := make([]IngestionRun, 0, limit)
	for rows.Next() {
		var r IngestionRun
		if err := rows.Scan(
			&r.ID,
			&r.SourceType,
			&r.SourceName,
			&r.DatasetName,
			&r.DatasetSplit,
			&r.Status,
			&r.StartedAt,
			&r.FinishedAt,
			&r.Notes,
			&r.PostsIngested,
		); err != nil {
			return nil, fmt.Errorf("scan ingestion run: %w", err)
		}
		out = append(out, r)
	}

	return out, rows.Err()
}

// GetPostsForAnalysis возвращает посты для batch-анализа с учетом dataset/live фильтра.
func (p *PostgresDB) GetPostsForAnalysis(filter AnalysisPostFilter) ([]PostFullData, error) {
	if filter.Limit <= 0 {
		filter.Limit = 100
	}
	if filter.Limit > 1000 {
		filter.Limit = 1000
	}
	if filter.SourceType == "" {
		filter.SourceType = "dataset"
	}

	conditions := make([]string, 0, 5)
	args := make([]interface{}, 0, 6)

	if filter.SourceType != "all" {
		args = append(args, filter.SourceType)
		conditions = append(conditions, fmt.Sprintf("p.source_type = $%d", len(args)))
	}
	if filter.DatasetName != "" {
		args = append(args, filter.DatasetName)
		conditions = append(conditions, fmt.Sprintf("p.dataset_name = $%d", len(args)))
	}
	if filter.DatasetSplit != "" {
		args = append(args, filter.DatasetSplit)
		conditions = append(conditions, fmt.Sprintf("p.dataset_split = $%d", len(args)))
	}
	if filter.IngestionRunID != nil {
		args = append(args, *filter.IngestionRunID)
		conditions = append(conditions, fmt.Sprintf("p.ingestion_run_id = $%d", len(args)))
	}
	if filter.OnlyUnanalyzed {
		conditions = append(conditions, "NOT EXISTS (SELECT 1 FROM analysis_results ar WHERE ar.post_id = p.id)")
	}
	if len(conditions) == 0 {
		conditions = append(conditions, "1=1")
	}

	args = append(args, filter.Limit)
	query := fmt.Sprintf(`
		SELECT
			p.id,
			p.content,
			p.account_id,
			p.published_at,
			a.username,
			COALESCE(a.followers_count, 0) AS followers_count,
			COALESCE(a.created_at, p.published_at) AS account_created_at
		FROM posts p
		JOIN accounts a ON p.account_id = a.id
		WHERE %s
		ORDER BY p.published_at ASC, p.id ASC
		LIMIT $%d
	`, strings.Join(conditions, " AND "), len(args))

	rows, err := p.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("get posts for analysis: %w", err)
	}
	defer rows.Close()

	posts := make([]PostFullData, 0, filter.Limit)
	for rows.Next() {
		var data PostFullData
		if err := rows.Scan(
			&data.ID,
			&data.Content,
			&data.AccountID,
			&data.PublishedAt,
			&data.Username,
			&data.FollowersCount,
			&data.AccountCreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan analysis post: %w", err)
		}
		posts = append(posts, data)
	}

	return posts, rows.Err()
}

// GetAnalysisSummary возвращает агрегированную статистику по результатам анализа.
func (p *PostgresDB) GetAnalysisSummary(filter AnalysisPostFilter) (AnalysisSummary, error) {
	if filter.SourceType == "" {
		filter.SourceType = "dataset"
	}

	summary := AnalysisSummary{
		SourceType:     filter.SourceType,
		DatasetName:    filter.DatasetName,
		DatasetSplit:   filter.DatasetSplit,
		IngestionRunID: filter.IngestionRunID,
	}

	conditions := make([]string, 0, 4)
	args := make([]interface{}, 0, 4)

	if filter.SourceType != "all" {
		args = append(args, filter.SourceType)
		conditions = append(conditions, fmt.Sprintf("p.source_type = $%d", len(args)))
	}
	if filter.DatasetName != "" {
		args = append(args, filter.DatasetName)
		conditions = append(conditions, fmt.Sprintf("p.dataset_name = $%d", len(args)))
	}
	if filter.DatasetSplit != "" {
		args = append(args, filter.DatasetSplit)
		conditions = append(conditions, fmt.Sprintf("p.dataset_split = $%d", len(args)))
	}
	if filter.IngestionRunID != nil {
		args = append(args, *filter.IngestionRunID)
		conditions = append(conditions, fmt.Sprintf("p.ingestion_run_id = $%d", len(args)))
	}
	if len(conditions) == 0 {
		conditions = append(conditions, "1=1")
	}

	query := fmt.Sprintf(`
		SELECT
			COUNT(*) AS total_analyzed,
			COUNT(*) FILTER (WHERE ar.escalation_priority = 1) AS high_risk,
			COUNT(*) FILTER (WHERE ar.escalation_priority = 2) AS medium_risk,
			COUNT(*) FILTER (WHERE ar.escalation_priority = 3) AS low_risk,
			COALESCE(AVG(ar.manipulation_score), 0) AS average_score
		FROM analysis_results ar
		JOIN posts p ON p.id = ar.post_id
		WHERE %s
	`, strings.Join(conditions, " AND "))

	err := p.db.QueryRow(query, args...).Scan(
		&summary.TotalAnalyzed,
		&summary.HighRisk,
		&summary.MediumRisk,
		&summary.LowRisk,
		&summary.AverageScore,
	)
	if err != nil {
		return summary, fmt.Errorf("get analysis summary: %w", err)
	}

	return summary, nil
}

// ListCases возвращает case-level сводку для API/отладки.
func (p *PostgresDB) ListCases(filter CaseFilter) ([]CaseListItem, error) {
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 500 {
		filter.Limit = 500
	}

	conditions := make([]string, 0, 5)
	args := make([]interface{}, 0, 6)
	scorerKey := normalizeScorerKey(filter.ScorerKey)
	useModelScore := useCaseModelScore(scorerKey)

	if filter.SourceName != "" {
		args = append(args, filter.SourceName)
		conditions = append(conditions, fmt.Sprintf("c.source_name = $%d", len(args)))
	}
	if filter.DatasetName != "" {
		args = append(args, filter.DatasetName)
		conditions = append(conditions, fmt.Sprintf("c.dataset_name = $%d", len(args)))
	}
	if filter.DatasetSplit != "" {
		args = append(args, filter.DatasetSplit)
		conditions = append(conditions, fmt.Sprintf("c.dataset_split = $%d", len(args)))
	}
	if filter.Label != "" {
		args = append(args, filter.Label)
		conditions = append(conditions, fmt.Sprintf("c.label = $%d", len(args)))
	}
	if filter.RiskLevel != "" {
		args = append(args, filter.RiskLevel)
		if useModelScore {
			conditions = append(conditions, fmt.Sprintf("cms.risk_level = $%d", len(args)))
		} else {
			conditions = append(conditions, fmt.Sprintf("cs.risk_level = $%d", len(args)))
		}
	}
	scoreJoin := "LEFT JOIN case_scores cs ON cs.case_id = c.id"
	scoreGroupBy := "c.id, cs.case_id"
	scoreSelect := `
			'case_scores' AS scorer_key,
			COALESCE(cs.score_version, '') AS model_version,
			cs.risk_score,
			COALESCE(cs.risk_level, '') AS risk_level,
			cs.temporal_score,
			cs.coordination_score,
			cs.content_score`
	scoreOrder := "cs.risk_score DESC NULLS LAST"
	if useModelScore {
		args = append(args, scorerKey)
		scoreJoin = fmt.Sprintf("LEFT JOIN case_model_scores cms ON cms.case_id = c.id AND cms.scorer_key = $%d", len(args))
		scoreGroupBy = "c.id, cms.id"
		scoreSelect = `
			COALESCE(cms.scorer_key, '') AS scorer_key,
			COALESCE(cms.model_version, '') AS model_version,
			cms.risk_score,
			COALESCE(cms.risk_level, '') AS risk_level,
			cms.temporal_score,
			cms.coordination_score,
			cms.content_score`
		scoreOrder = "cms.risk_score DESC NULLS LAST"
	}
	if filter.OnlyUnscored {
		if useModelScore {
			conditions = append(conditions, "cms.case_id IS NULL")
		} else {
			conditions = append(conditions, "cs.case_id IS NULL")
		}
	}
	if len(conditions) == 0 {
		conditions = append(conditions, "1=1")
	}

	args = append(args, filter.Limit)
	query := fmt.Sprintf(`
		SELECT
			c.id,
			c.source_name,
			c.dataset_name,
			c.dataset_split,
			c.external_case_id,
			c.case_type,
			COALESCE(c.label, '') AS label,
			COALESCE(c.title, '') AS title,
			COALESCE(c.event_name, '') AS event_name,
			c.status,
			c.first_event_at,
			c.last_event_at,
			COUNT(p.id) AS post_count,
			%s
		FROM cases c
		LEFT JOIN posts p ON p.case_id = c.id
		%s
		WHERE %s
		GROUP BY %s
		ORDER BY %s, c.first_event_at DESC NULLS LAST, c.id DESC
		LIMIT $%d
	`, scoreSelect, scoreJoin, strings.Join(conditions, " AND "), scoreGroupBy, scoreOrder, len(args))

	rows, err := p.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list cases: %w", err)
	}
	defer rows.Close()

	items := make([]CaseListItem, 0, filter.Limit)
	for rows.Next() {
		var item CaseListItem
		var riskScore, temporalScore, coordinationScore, contentScore sql.NullFloat64
		if err := rows.Scan(
			&item.ID,
			&item.SourceName,
			&item.DatasetName,
			&item.DatasetSplit,
			&item.ExternalCaseID,
			&item.CaseType,
			&item.Label,
			&item.Title,
			&item.EventName,
			&item.Status,
			&item.FirstEventAt,
			&item.LastEventAt,
			&item.PostCount,
			&item.ScorerKey,
			&item.ModelVersion,
			&riskScore,
			&item.RiskLevel,
			&temporalScore,
			&coordinationScore,
			&contentScore,
		); err != nil {
			return nil, fmt.Errorf("scan case item: %w", err)
		}
		if riskScore.Valid {
			v := riskScore.Float64
			item.RiskScore = &v
		}
		if temporalScore.Valid {
			v := temporalScore.Float64
			item.TemporalScore = &v
		}
		if coordinationScore.Valid {
			v := coordinationScore.Float64
			item.CoordinationScore = &v
		}
		if contentScore.Valid {
			v := contentScore.Float64
			item.ContentScore = &v
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

// GetCaseDetails возвращает полный case-level payload для frontend details view.
func (p *PostgresDB) GetCaseDetails(caseID int64, scorerKey string) (CaseDetails, error) {
	if caseID <= 0 {
		return CaseDetails{}, sql.ErrNoRows
	}

	summary, err := p.getCaseSummary(caseID, scorerKey)
	if err != nil {
		return CaseDetails{}, err
	}

	rawPosts, err := p.getCasePosts(caseID)
	if err != nil {
		return CaseDetails{}, fmt.Errorf("load case posts: %w", err)
	}
	posts := make([]CasePostItem, 0, len(rawPosts))
	var rootPost *CasePostItem
	for _, raw := range rawPosts {
		item := casePostDataToItem(raw)
		if item.IsCaseRoot && rootPost == nil {
			copyItem := item
			rootPost = &copyItem
		}
		posts = append(posts, item)
	}
	if rootPost == nil && len(posts) > 0 {
		copyItem := posts[0]
		rootPost = &copyItem
	}

	accounts, err := p.getCaseAccounts(caseID)
	if err != nil {
		return CaseDetails{}, fmt.Errorf("load case accounts: %w", err)
	}
	artifacts, err := p.getCaseArtifacts(caseID)
	if err != nil {
		return CaseDetails{}, fmt.Errorf("load case artifacts: %w", err)
	}
	features, err := p.getCaseFeaturesSnapshot(caseID)
	if err != nil {
		return CaseDetails{}, fmt.Errorf("load case features: %w", err)
	}

	return CaseDetails{
		Case:      summary,
		RootPost:  rootPost,
		Posts:     posts,
		Accounts:  accounts,
		Artifacts: artifacts,
		Features:  features,
	}, nil
}

func (p *PostgresDB) getCaseSummary(caseID int64, scorerKey string) (CaseListItem, error) {
	scorerKey = normalizeScorerKey(scorerKey)
	useModelScore := useCaseModelScore(scorerKey)
	args := []interface{}{caseID}

	scoreJoin := "LEFT JOIN case_scores cs ON cs.case_id = c.id"
	scoreGroupBy := "c.id, cs.case_id"
	scoreSelect := `
			'case_scores' AS scorer_key,
			COALESCE(cs.score_version, '') AS model_version,
			cs.risk_score,
			COALESCE(cs.risk_level, '') AS risk_level,
			cs.temporal_score,
			cs.coordination_score,
			cs.content_score`
	if useModelScore {
		args = append(args, scorerKey)
		scoreJoin = "LEFT JOIN case_model_scores cms ON cms.case_id = c.id AND cms.scorer_key = $2"
		scoreGroupBy = "c.id, cms.id"
		scoreSelect = `
			COALESCE(cms.scorer_key, '') AS scorer_key,
			COALESCE(cms.model_version, '') AS model_version,
			cms.risk_score,
			COALESCE(cms.risk_level, '') AS risk_level,
			cms.temporal_score,
			cms.coordination_score,
			cms.content_score`
	}

	query := fmt.Sprintf(`
		SELECT
			c.id,
			c.source_name,
			c.dataset_name,
			c.dataset_split,
			c.external_case_id,
			c.case_type,
			COALESCE(c.label, '') AS label,
			COALESCE(c.title, '') AS title,
			COALESCE(c.event_name, '') AS event_name,
			c.status,
			c.first_event_at,
			c.last_event_at,
			COUNT(p.id) AS post_count,
			%s
		FROM cases c
		LEFT JOIN posts p ON p.case_id = c.id
		%s
		WHERE c.id = $1
		GROUP BY %s
	`, scoreSelect, scoreJoin, scoreGroupBy)

	var item CaseListItem
	var riskScore, temporalScore, coordinationScore, contentScore sql.NullFloat64
	err := p.db.QueryRow(query, args...).Scan(
		&item.ID,
		&item.SourceName,
		&item.DatasetName,
		&item.DatasetSplit,
		&item.ExternalCaseID,
		&item.CaseType,
		&item.Label,
		&item.Title,
		&item.EventName,
		&item.Status,
		&item.FirstEventAt,
		&item.LastEventAt,
		&item.PostCount,
		&item.ScorerKey,
		&item.ModelVersion,
		&riskScore,
		&item.RiskLevel,
		&temporalScore,
		&coordinationScore,
		&contentScore,
	)
	if err != nil {
		return CaseListItem{}, err
	}
	if riskScore.Valid {
		v := riskScore.Float64
		item.RiskScore = &v
	}
	if temporalScore.Valid {
		v := temporalScore.Float64
		item.TemporalScore = &v
	}
	if coordinationScore.Valid {
		v := coordinationScore.Float64
		item.CoordinationScore = &v
	}
	if contentScore.Valid {
		v := contentScore.Float64
		item.ContentScore = &v
	}
	return item, nil
}

func (p *PostgresDB) getCaseAccounts(caseID int64) ([]CaseAccountItem, error) {
	rows, err := p.db.Query(`
		SELECT
			a.id,
			a.external_id,
			a.username,
			COALESCE(a.display_name, '') AS display_name,
			COALESCE(a.account_url, '') AS account_url,
			COALESCE(a.followers_count, 0) AS followers_count,
			COALESCE(a.following_count, 0) AS following_count,
			COALESCE(a.posts_count, 0) AS posts_count,
			COALESCE(a.is_bot, FALSE) AS is_bot,
			COALESCE(a.is_verified, FALSE) AS is_verified,
			COUNT(p.id) AS case_post_count,
			MIN(p.published_at) AS first_post_at,
			MAX(p.published_at) AS last_post_at,
			BOOL_OR(p.is_case_root) AS has_root_post
		FROM posts p
		JOIN accounts a ON a.id = p.account_id
		WHERE p.case_id = $1
		GROUP BY a.id
		ORDER BY COUNT(p.id) DESC, a.id ASC
	`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]CaseAccountItem, 0, 16)
	for rows.Next() {
		var item CaseAccountItem
		var firstPostAt, lastPostAt sql.NullTime
		if err := rows.Scan(
			&item.ID,
			&item.ExternalID,
			&item.Username,
			&item.DisplayName,
			&item.AccountURL,
			&item.FollowersCount,
			&item.FollowingCount,
			&item.PostsCount,
			&item.IsBot,
			&item.IsVerified,
			&item.CasePostCount,
			&firstPostAt,
			&lastPostAt,
			&item.HasRootPost,
		); err != nil {
			return nil, err
		}
		if firstPostAt.Valid {
			v := firstPostAt.Time
			item.FirstPostAt = &v
		}
		if lastPostAt.Valid {
			v := lastPostAt.Time
			item.LastPostAt = &v
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (p *PostgresDB) getCaseArtifacts(caseID int64) (CaseArtifactsSummary, error) {
	tags, err := p.getCaseTagCounts(caseID)
	if err != nil {
		return CaseArtifactsSummary{}, err
	}
	urls, err := p.getCaseURLCounts(caseID)
	if err != nil {
		return CaseArtifactsSummary{}, err
	}
	domains, err := p.getCaseDomainCounts(caseID)
	if err != nil {
		return CaseArtifactsSummary{}, err
	}
	return CaseArtifactsSummary{Tags: tags, URLs: urls, Domains: domains}, nil
}

func (p *PostgresDB) getCaseTagCounts(caseID int64) ([]ArtifactCount, error) {
	return p.queryArtifactCounts(`
		SELECT pt.tag_name AS value, COUNT(*) AS count
		FROM post_tags pt
		JOIN posts p ON p.id = pt.post_id
		WHERE p.case_id = $1
		GROUP BY pt.tag_name
		ORDER BY COUNT(*) DESC, pt.tag_name ASC
		LIMIT 20
	`, caseID)
}

func (p *PostgresDB) getCaseURLCounts(caseID int64) ([]ArtifactCount, error) {
	return p.queryArtifactCounts(`
		SELECT COALESCE(pl.expanded_url, pl.url) AS value, COUNT(*) AS count
		FROM post_links pl
		JOIN posts p ON p.id = pl.post_id
		WHERE p.case_id = $1
		GROUP BY COALESCE(pl.expanded_url, pl.url)
		ORDER BY COUNT(*) DESC, COALESCE(pl.expanded_url, pl.url) ASC
		LIMIT 20
	`, caseID)
}

func (p *PostgresDB) getCaseDomainCounts(caseID int64) ([]ArtifactCount, error) {
	return p.queryArtifactCounts(`
		SELECT COALESCE(NULLIF(pl.domain, ''), COALESCE(pl.expanded_url, pl.url)) AS value, COUNT(*) AS count
		FROM post_links pl
		JOIN posts p ON p.id = pl.post_id
		WHERE p.case_id = $1
		GROUP BY COALESCE(NULLIF(pl.domain, ''), COALESCE(pl.expanded_url, pl.url))
		ORDER BY COUNT(*) DESC, COALESCE(NULLIF(pl.domain, ''), COALESCE(pl.expanded_url, pl.url)) ASC
		LIMIT 20
	`, caseID)
}

func (p *PostgresDB) queryArtifactCounts(query string, caseID int64) ([]ArtifactCount, error) {
	rows, err := p.db.Query(query, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ArtifactCount, 0, 20)
	for rows.Next() {
		var item ArtifactCount
		if err := rows.Scan(&item.Value, &item.Count); err != nil {
			return nil, err
		}
		if strings.TrimSpace(item.Value) != "" {
			items = append(items, item)
		}
	}
	return items, rows.Err()
}

func (p *PostgresDB) getCaseFeaturesSnapshot(caseID int64) (*CaseFeaturesSnapshot, error) {
	var item CaseFeaturesSnapshot
	var temporalRaw, coordinationRaw, contentRaw, payloadRaw []byte
	err := p.db.QueryRow(`
		SELECT
			feature_version,
			event_count,
			unique_account_count,
			unique_url_count,
			unique_hashtag_count,
			temporal_features,
			coordination_features,
			content_features,
			feature_payload,
			computed_at
		FROM case_features
		WHERE case_id = $1
	`, caseID).Scan(
		&item.FeatureVersion,
		&item.EventCount,
		&item.UniqueAccountCount,
		&item.UniqueURLCount,
		&item.UniqueHashtagCount,
		&temporalRaw,
		&coordinationRaw,
		&contentRaw,
		&payloadRaw,
		&item.ComputedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if item.TemporalFeatures, err = decodeJSONMap(temporalRaw); err != nil {
		return nil, fmt.Errorf("decode temporal features: %w", err)
	}
	if item.CoordinationFeatures, err = decodeJSONMap(coordinationRaw); err != nil {
		return nil, fmt.Errorf("decode coordination features: %w", err)
	}
	if item.ContentFeatures, err = decodeJSONMap(contentRaw); err != nil {
		return nil, fmt.Errorf("decode content features: %w", err)
	}
	if item.FeaturePayload, err = decodeJSONMap(payloadRaw); err != nil {
		return nil, fmt.Errorf("decode feature payload: %w", err)
	}
	return &item, nil
}

// ListCaseModelScores возвращает все сохраненные model/scorer outputs для case.
func (p *PostgresDB) ListCaseModelScores(caseID int64) ([]CaseModelScoreItem, error) {
	rows, err := p.db.Query(`
		SELECT
			case_id,
			scorer_key,
			model_version,
			risk_score,
			risk_level,
			confidence_score,
			temporal_score,
			coordination_score,
			content_score,
			evidence,
			feature_payload,
			model_info,
			COALESCE(pipeline_hash, '') AS pipeline_hash,
			COALESCE(source_endpoint, '') AS source_endpoint,
			computed_at
		FROM case_model_scores
		WHERE case_id = $1
		ORDER BY risk_score DESC, scorer_key ASC
	`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]CaseModelScoreItem, 0, 4)
	for rows.Next() {
		var item CaseModelScoreItem
		var confidenceScore, temporalScore, coordinationScore, contentScore sql.NullFloat64
		var evidenceRaw, payloadRaw, modelInfoRaw []byte
		if err := rows.Scan(
			&item.CaseID,
			&item.ScorerKey,
			&item.ModelVersion,
			&item.RiskScore,
			&item.RiskLevel,
			&confidenceScore,
			&temporalScore,
			&coordinationScore,
			&contentScore,
			&evidenceRaw,
			&payloadRaw,
			&modelInfoRaw,
			&item.PipelineHash,
			&item.SourceEndpoint,
			&item.ComputedAt,
		); err != nil {
			return nil, err
		}
		if confidenceScore.Valid {
			v := confidenceScore.Float64
			item.ConfidenceScore = &v
		}
		if temporalScore.Valid {
			v := temporalScore.Float64
			item.TemporalScore = &v
		}
		if coordinationScore.Valid {
			v := coordinationScore.Float64
			item.CoordinationScore = &v
		}
		if contentScore.Valid {
			v := contentScore.Float64
			item.ContentScore = &v
		}
		if len(evidenceRaw) > 0 {
			if err := json.Unmarshal(evidenceRaw, &item.Evidence); err != nil {
				return nil, fmt.Errorf("decode case model evidence: %w", err)
			}
		}
		var err error
		if item.FeaturePayload, err = decodeJSONMap(payloadRaw); err != nil {
			return nil, fmt.Errorf("decode case model feature payload: %w", err)
		}
		if item.ModelInfo, err = decodeJSONMap(modelInfoRaw); err != nil {
			return nil, fmt.Errorf("decode case model info: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ListScoredCases возвращает cases с уже рассчитанными score для evaluation/export.
func (p *PostgresDB) ListScoredCases(filter CaseFilter) ([]ScoredCaseItem, error) {
	if filter.Limit <= 0 {
		filter.Limit = 100
	}
	if filter.Limit > 10000 {
		filter.Limit = 10000
	}

	conditions := make([]string, 0, 5)
	args := make([]interface{}, 0, 6)

	if filter.SourceName != "" {
		args = append(args, filter.SourceName)
		conditions = append(conditions, fmt.Sprintf("c.source_name = $%d", len(args)))
	}
	if filter.DatasetName != "" {
		args = append(args, filter.DatasetName)
		conditions = append(conditions, fmt.Sprintf("c.dataset_name = $%d", len(args)))
	}
	if filter.DatasetSplit != "" {
		args = append(args, filter.DatasetSplit)
		conditions = append(conditions, fmt.Sprintf("c.dataset_split = $%d", len(args)))
	}
	if filter.Label != "" {
		args = append(args, filter.Label)
		conditions = append(conditions, fmt.Sprintf("c.label = $%d", len(args)))
	}
	if len(conditions) == 0 {
		conditions = append(conditions, "1=1")
	}

	args = append(args, filter.Limit)
	query := fmt.Sprintf(`
		SELECT
			c.id,
			c.source_name,
			c.dataset_name,
			c.dataset_split,
			c.external_case_id,
			c.case_type,
			COALESCE(c.label, '') AS label,
			COALESCE(c.title, '') AS title,
			COALESCE(c.event_name, '') AS event_name,
			c.status,
			c.first_event_at,
			c.last_event_at,
			COUNT(p.id) AS post_count,
			COALESCE(cf.feature_version, '') AS feature_version,
			cs.score_version,
			COALESCE(cs.pipeline_hash, '') AS pipeline_hash,
			cs.risk_score,
			COALESCE(cs.risk_level, '') AS risk_level,
			COALESCE(cs.temporal_score, 0) AS temporal_score,
			COALESCE(cs.coordination_score, 0) AS coordination_score,
			COALESCE(cs.content_score, 0) AS content_score,
			COALESCE(cs.evidence, '[]'::jsonb) AS evidence
		FROM cases c
		JOIN case_scores cs ON cs.case_id = c.id
		LEFT JOIN case_features cf ON cf.case_id = c.id
		LEFT JOIN posts p ON p.case_id = c.id
		WHERE %s
		GROUP BY c.id, cs.case_id, cf.case_id
		ORDER BY cs.risk_score DESC, c.first_event_at DESC NULLS LAST, c.id DESC
		LIMIT $%d
	`, strings.Join(conditions, " AND "), len(args))

	rows, err := p.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list scored cases: %w", err)
	}
	defer rows.Close()

	items := make([]ScoredCaseItem, 0, filter.Limit)
	for rows.Next() {
		var item ScoredCaseItem
		var evidenceRaw []byte
		if err := rows.Scan(
			&item.ID,
			&item.SourceName,
			&item.DatasetName,
			&item.DatasetSplit,
			&item.ExternalCaseID,
			&item.CaseType,
			&item.Label,
			&item.Title,
			&item.EventName,
			&item.Status,
			&item.FirstEventAt,
			&item.LastEventAt,
			&item.PostCount,
			&item.FeatureVersion,
			&item.ScoreVersion,
			&item.PipelineHash,
			&item.RiskScore,
			&item.RiskLevel,
			&item.TemporalScore,
			&item.CoordinationScore,
			&item.ContentScore,
			&evidenceRaw,
		); err != nil {
			return nil, fmt.Errorf("scan scored case item: %w", err)
		}
		if len(evidenceRaw) > 0 {
			if err := json.Unmarshal(evidenceRaw, &item.Evidence); err != nil {
				return nil, fmt.Errorf("decode case evidence: %w", err)
			}
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

// GetCasesForScoring возвращает cases с полным набором постов для расчета признаков.
func (p *PostgresDB) GetCasesForScoring(filter CaseFilter) ([]CaseForScoring, error) {
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 500 {
		filter.Limit = 500
	}

	conditions := make([]string, 0, 5)
	args := make([]interface{}, 0, 6)

	if filter.SourceName != "" {
		args = append(args, filter.SourceName)
		conditions = append(conditions, fmt.Sprintf("c.source_name = $%d", len(args)))
	}
	if filter.DatasetName != "" {
		args = append(args, filter.DatasetName)
		conditions = append(conditions, fmt.Sprintf("c.dataset_name = $%d", len(args)))
	}
	if filter.DatasetSplit != "" {
		args = append(args, filter.DatasetSplit)
		conditions = append(conditions, fmt.Sprintf("c.dataset_split = $%d", len(args)))
	}
	if filter.Label != "" {
		args = append(args, filter.Label)
		conditions = append(conditions, fmt.Sprintf("c.label = $%d", len(args)))
	}
	if filter.OnlyUnscored {
		if len(filter.MissingModelScoreKeys) > 0 {
			missingConditions := make([]string, 0, len(filter.MissingModelScoreKeys))
			for _, key := range filter.MissingModelScoreKeys {
				key = strings.TrimSpace(key)
				if key == "" {
					continue
				}
				args = append(args, key)
				missingConditions = append(
					missingConditions,
					fmt.Sprintf("NOT EXISTS (SELECT 1 FROM case_model_scores cms WHERE cms.case_id = c.id AND cms.scorer_key = $%d)", len(args)),
				)
			}
			if len(missingConditions) > 0 {
				conditions = append(conditions, "("+strings.Join(missingConditions, " OR ")+")")
			}
		} else {
			conditions = append(conditions, "NOT EXISTS (SELECT 1 FROM case_scores cs WHERE cs.case_id = c.id)")
		}
	}
	if len(conditions) == 0 {
		conditions = append(conditions, "1=1")
	}

	args = append(args, filter.Limit)
	query := fmt.Sprintf(`
		SELECT
			c.id,
			c.source_name,
			c.dataset_name,
			c.dataset_split,
			c.external_case_id,
			c.case_type,
			COALESCE(c.label, '') AS label,
			COALESCE(c.title, '') AS title,
			COALESCE(c.event_name, '') AS event_name,
			COALESCE(c.root_post_external_id, '') AS root_post_external_id,
			c.first_event_at,
			c.last_event_at
		FROM cases c
		WHERE %s
		ORDER BY c.first_event_at ASC NULLS LAST, c.id ASC
		LIMIT $%d
	`, strings.Join(conditions, " AND "), len(args))

	rows, err := p.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("get cases for scoring: %w", err)
	}
	defer rows.Close()

	cases := make([]CaseForScoring, 0, filter.Limit)
	for rows.Next() {
		var item CaseForScoring
		if err := rows.Scan(
			&item.ID,
			&item.SourceName,
			&item.DatasetName,
			&item.DatasetSplit,
			&item.ExternalCaseID,
			&item.CaseType,
			&item.Label,
			&item.Title,
			&item.EventName,
			&item.RootPostExternalID,
			&item.FirstEventAt,
			&item.LastEventAt,
		); err != nil {
			return nil, fmt.Errorf("scan case for scoring: %w", err)
		}

		posts, err := p.getCasePosts(item.ID)
		if err != nil {
			return nil, fmt.Errorf("load case posts %d: %w", item.ID, err)
		}
		item.Posts = posts
		cases = append(cases, item)
	}

	return cases, rows.Err()
}

func (p *PostgresDB) getCasePosts(caseID int64) ([]CasePostData, error) {
	rows, err := p.db.Query(`
		SELECT
			p.id,
			p.external_id,
			p.account_id,
			a.username,
			p.published_at,
			p.content,
			p.is_case_root,
			p.reply_to_post_id,
			COALESCE(p.likes_count, 0),
			COALESCE(p.reposts_count, 0),
			COALESCE(p.replies_count, 0),
			COALESCE(a.followers_count, 0),
			COALESCE(a.following_count, 0),
			COALESCE(a.posts_count, 0),
			COALESCE(a.is_verified, FALSE),
			a.created_at,
			COALESCE(a.account_url, '')
		FROM posts p
		JOIN accounts a ON a.id = p.account_id
		WHERE p.case_id = $1
		ORDER BY p.published_at ASC, p.id ASC
	`, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	posts := make([]CasePostData, 0, 32)
	postIndex := make(map[int64]int)
	for rows.Next() {
		var item CasePostData
		var replyTo sql.NullInt64
		var accountCreatedAt sql.NullTime
		if err := rows.Scan(
			&item.ID,
			&item.ExternalID,
			&item.AccountID,
			&item.Username,
			&item.PublishedAt,
			&item.Content,
			&item.IsCaseRoot,
			&replyTo,
			&item.LikesCount,
			&item.RepostsCount,
			&item.RepliesCount,
			&item.FollowersCount,
			&item.FollowingCount,
			&item.PostsCount,
			&item.IsVerified,
			&accountCreatedAt,
			&item.AccountURL,
		); err != nil {
			return nil, err
		}
		if replyTo.Valid {
			value := replyTo.Int64
			item.ReplyToPostID = &value
		}
		if accountCreatedAt.Valid {
			value := accountCreatedAt.Time
			item.AccountCreatedAt = &value
		}
		postIndex[item.ID] = len(posts)
		posts = append(posts, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	tagRows, err := p.db.Query(`
		SELECT post_id, tag_name
		FROM post_tags
		WHERE post_id IN (SELECT id FROM posts WHERE case_id = $1)
	`, caseID)
	if err != nil {
		return nil, err
	}
	defer tagRows.Close()
	for tagRows.Next() {
		var postID int64
		var tag string
		if err := tagRows.Scan(&postID, &tag); err != nil {
			return nil, err
		}
		if idx, ok := postIndex[postID]; ok {
			posts[idx].Tags = append(posts[idx].Tags, tag)
		}
	}
	if err := tagRows.Err(); err != nil {
		return nil, err
	}

	linkRows, err := p.db.Query(`
		SELECT post_id, COALESCE(expanded_url, url) AS resolved_url
		FROM post_links
		WHERE post_id IN (SELECT id FROM posts WHERE case_id = $1)
	`, caseID)
	if err != nil {
		return nil, err
	}
	defer linkRows.Close()
	for linkRows.Next() {
		var postID int64
		var resolvedURL string
		if err := linkRows.Scan(&postID, &resolvedURL); err != nil {
			return nil, err
		}
		if idx, ok := postIndex[postID]; ok {
			posts[idx].Links = append(posts[idx].Links, resolvedURL)
		}
	}
	if err := linkRows.Err(); err != nil {
		return nil, err
	}

	return posts, nil
}

// SaveCaseFeatures сохраняет рассчитанные признаки кейса.
func (p *PostgresDB) SaveCaseFeatures(rec CaseFeaturesRecord) error {
	temporalJSON, err := marshalJSONObject(rec.TemporalFeatures)
	if err != nil {
		return fmt.Errorf("marshal temporal features: %w", err)
	}
	coordinationJSON, err := marshalJSONObject(rec.CoordinationFeatures)
	if err != nil {
		return fmt.Errorf("marshal coordination features: %w", err)
	}
	contentJSON, err := marshalJSONObject(rec.ContentFeatures)
	if err != nil {
		return fmt.Errorf("marshal content features: %w", err)
	}
	payloadJSON, err := marshalJSONObject(rec.FeaturePayload)
	if err != nil {
		return fmt.Errorf("marshal feature payload: %w", err)
	}

	_, err = p.db.Exec(`
		INSERT INTO case_features (
			case_id, feature_version, event_count, unique_account_count,
			unique_url_count, unique_hashtag_count, temporal_features,
			coordination_features, content_features, feature_payload, computed_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		ON CONFLICT (case_id) DO UPDATE SET
			feature_version = EXCLUDED.feature_version,
			event_count = EXCLUDED.event_count,
			unique_account_count = EXCLUDED.unique_account_count,
			unique_url_count = EXCLUDED.unique_url_count,
			unique_hashtag_count = EXCLUDED.unique_hashtag_count,
			temporal_features = EXCLUDED.temporal_features,
			coordination_features = EXCLUDED.coordination_features,
			content_features = EXCLUDED.content_features,
			feature_payload = EXCLUDED.feature_payload,
			computed_at = NOW()
	`,
		rec.CaseID,
		rec.FeatureVersion,
		rec.EventCount,
		rec.UniqueAccountCount,
		rec.UniqueURLCount,
		rec.UniqueHashtagCount,
		temporalJSON,
		coordinationJSON,
		contentJSON,
		payloadJSON,
	)
	if err != nil {
		return fmt.Errorf("save case features: %w", err)
	}
	return nil
}

// SaveCaseScore сохраняет итоговый скор кейса.
func (p *PostgresDB) SaveCaseScore(rec CaseScoreRecord) error {
	evidenceJSON, err := json.Marshal(rec.Evidence)
	if err != nil {
		return fmt.Errorf("marshal case evidence: %w", err)
	}

	_, err = p.db.Exec(`
		INSERT INTO case_scores (
			case_id, score_version, temporal_score, coordination_score,
			content_score, risk_score, risk_level, evidence,
			pipeline_hash, computed_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, NOW())
		ON CONFLICT (case_id) DO UPDATE SET
			score_version = EXCLUDED.score_version,
			temporal_score = EXCLUDED.temporal_score,
			coordination_score = EXCLUDED.coordination_score,
			content_score = EXCLUDED.content_score,
			risk_score = EXCLUDED.risk_score,
			risk_level = EXCLUDED.risk_level,
			evidence = EXCLUDED.evidence,
			pipeline_hash = EXCLUDED.pipeline_hash,
			computed_at = NOW()
	`,
		rec.CaseID,
		rec.ScoreVersion,
		rec.TemporalScore,
		rec.CoordinationScore,
		rec.ContentScore,
		rec.RiskScore,
		rec.RiskLevel,
		string(evidenceJSON),
		rec.PipelineHash,
	)
	if err != nil {
		return fmt.Errorf("save case score: %w", err)
	}
	return nil
}

// SaveCaseModelScore сохраняет результат конкретного model/scorer для case.
func (p *PostgresDB) SaveCaseModelScore(rec CaseModelScoreRecord) error {
	evidenceJSON, err := json.Marshal(rec.Evidence)
	if err != nil {
		return fmt.Errorf("marshal case model evidence: %w", err)
	}
	payloadJSON, err := marshalJSONObject(rec.FeaturePayload)
	if err != nil {
		return fmt.Errorf("marshal case model feature payload: %w", err)
	}
	modelInfoJSON, err := marshalJSONObject(rec.ModelInfo)
	if err != nil {
		return fmt.Errorf("marshal case model info: %w", err)
	}

	_, err = p.db.Exec(`
		INSERT INTO case_model_scores (
			case_id, scorer_key, model_version, risk_score, risk_level,
			confidence_score, temporal_score, coordination_score, content_score,
			evidence, feature_payload, model_info, pipeline_hash, source_endpoint,
			computed_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11::jsonb, $12::jsonb, $13, $14, NOW())
		ON CONFLICT (case_id, scorer_key) DO UPDATE SET
			model_version = EXCLUDED.model_version,
			risk_score = EXCLUDED.risk_score,
			risk_level = EXCLUDED.risk_level,
			confidence_score = EXCLUDED.confidence_score,
			temporal_score = EXCLUDED.temporal_score,
			coordination_score = EXCLUDED.coordination_score,
			content_score = EXCLUDED.content_score,
			evidence = EXCLUDED.evidence,
			feature_payload = EXCLUDED.feature_payload,
			model_info = EXCLUDED.model_info,
			pipeline_hash = EXCLUDED.pipeline_hash,
			source_endpoint = EXCLUDED.source_endpoint,
			computed_at = NOW()
	`,
		rec.CaseID,
		rec.ScorerKey,
		rec.ModelVersion,
		rec.RiskScore,
		rec.RiskLevel,
		rec.ConfidenceScore,
		rec.TemporalScore,
		rec.CoordinationScore,
		rec.ContentScore,
		string(evidenceJSON),
		string(payloadJSON),
		string(modelInfoJSON),
		rec.PipelineHash,
		rec.SourceEndpoint,
	)
	if err != nil {
		return fmt.Errorf("save case model score: %w", err)
	}
	return nil
}

// SaveEvaluationRun сохраняет агрегированный результат evaluation в evaluation_runs.
func (p *PostgresDB) SaveEvaluationRun(rec EvaluationRunRecord) (int64, error) {
	metricsJSON, err := marshalJSONObject(rec.Metrics)
	if err != nil {
		return 0, fmt.Errorf("marshal evaluation metrics: %w", err)
	}

	var id int64
	err = p.db.QueryRow(`
		INSERT INTO evaluation_runs (
			source_name,
			dataset_name,
			dataset_split,
			pipeline_hash,
			scorer_version,
			case_count,
			precision_at_10,
			precision_at_20,
			recall,
			f1,
			roc_auc,
			pr_auc,
			metrics,
			notes,
			evaluated_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12,
			$13, $14, NOW()
		)
		RETURNING id
	`,
		rec.SourceName,
		rec.DatasetName,
		rec.DatasetSplit,
		rec.PipelineHash,
		rec.ScorerVersion,
		rec.CaseCount,
		rec.PrecisionAt10,
		rec.PrecisionAt20,
		rec.Recall,
		rec.F1,
		rec.ROCAUC,
		rec.PRAUC,
		metricsJSON,
		rec.Notes,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("save evaluation run: %w", err)
	}

	return id, nil
}

type modelScoreForComparison struct {
	CaseID         int64
	ExternalCaseID string
	EventName      string
	Label          string
	ScorerKey      string
	ModelVersion   string
	RiskScore      float64
}

// GetModelComparison сравнивает выбранные case_model_scores на размеченных cases.
func (p *PostgresDB) GetModelComparison(filter ModelComparisonFilter) (ModelComparisonSummary, error) {
	filter.ScorerKeys = normalizeScorerKeys(filter.ScorerKeys)
	if len(filter.ScorerKeys) == 0 {
		filter.ScorerKeys = []string{
			"case_logreg_oof",
			"case_lightgbm_oof",
			"pheme_transformer_text_oof",
			"case_ensemble_v1",
		}
	}
	filter.PositiveLabels = normalizePositiveLabels(filter.PositiveLabels)
	if len(filter.PositiveLabels) == 0 {
		filter.PositiveLabels = []string{"rumor", "rumour"}
	}
	if filter.TopK <= 0 {
		filter.TopK = 20
	}
	if filter.TopK > 100 {
		filter.TopK = 100
	}

	conditions := []string{"cms.scorer_key = ANY($1)", "COALESCE(c.label, '') <> ''"}
	args := []interface{}{pq.Array(filter.ScorerKeys)}
	if filter.SourceName != "" {
		args = append(args, filter.SourceName)
		conditions = append(conditions, fmt.Sprintf("c.source_name = $%d", len(args)))
	}
	if filter.DatasetName != "" {
		args = append(args, filter.DatasetName)
		conditions = append(conditions, fmt.Sprintf("c.dataset_name = $%d", len(args)))
	}
	if filter.DatasetSplit != "" {
		args = append(args, filter.DatasetSplit)
		conditions = append(conditions, fmt.Sprintf("c.dataset_split = $%d", len(args)))
	}

	query := fmt.Sprintf(`
		SELECT
			c.id,
			c.external_case_id,
			COALESCE(c.event_name, '') AS event_name,
			COALESCE(c.label, '') AS label,
			cms.scorer_key,
			cms.model_version,
			cms.risk_score
		FROM cases c
		JOIN case_model_scores cms ON cms.case_id = c.id
		WHERE %s
		ORDER BY c.id ASC, cms.scorer_key ASC
	`, strings.Join(conditions, " AND "))

	rows, err := p.db.Query(query, args...)
	if err != nil {
		return ModelComparisonSummary{}, fmt.Errorf("load model comparison scores: %w", err)
	}
	defer rows.Close()

	raw := make([]modelScoreForComparison, 0, 1024)
	for rows.Next() {
		var item modelScoreForComparison
		if err := rows.Scan(
			&item.CaseID,
			&item.ExternalCaseID,
			&item.EventName,
			&item.Label,
			&item.ScorerKey,
			&item.ModelVersion,
			&item.RiskScore,
		); err != nil {
			return ModelComparisonSummary{}, fmt.Errorf("scan model comparison score: %w", err)
		}
		raw = append(raw, item)
	}
	if err := rows.Err(); err != nil {
		return ModelComparisonSummary{}, err
	}

	summary := ModelComparisonSummary{
		PositiveLabels: filter.PositiveLabels,
		Scorers:        filter.ScorerKeys,
		Models:         make(map[string]ModelComparisonModel, len(filter.ScorerKeys)),
		GeneratedAt:    time.Now().UTC(),
	}
	summary.Dataset.SourceName = filter.SourceName
	summary.Dataset.DatasetName = filter.DatasetName
	summary.Dataset.DatasetSplit = filter.DatasetSplit

	if len(raw) == 0 {
		return summary, nil
	}

	positiveSet := make(map[string]struct{}, len(filter.PositiveLabels))
	for _, label := range filter.PositiveLabels {
		positiveSet[normalizeLabelForMetrics(label)] = struct{}{}
	}

	caseLabels := make(map[int64]string)
	caseExternalIDs := make(map[int64]string)
	caseEvents := make(map[int64]string)
	scoresByScorer := make(map[string][]labeledScore, len(filter.ScorerKeys))
	scoresByCase := make(map[int64]map[string]float64)
	modelVersions := make(map[string]map[string]int)

	for _, item := range raw {
		caseLabels[item.CaseID] = item.Label
		caseExternalIDs[item.CaseID] = item.ExternalCaseID
		caseEvents[item.CaseID] = item.EventName
		if _, ok := scoresByCase[item.CaseID]; !ok {
			scoresByCase[item.CaseID] = make(map[string]float64)
		}
		scoresByCase[item.CaseID][item.ScorerKey] = item.RiskScore
		scoresByScorer[item.ScorerKey] = append(scoresByScorer[item.ScorerKey], labeledScore{
			CaseID: item.CaseID,
			Label:  item.Label,
			Score:  item.RiskScore,
		})
		if _, ok := modelVersions[item.ScorerKey]; !ok {
			modelVersions[item.ScorerKey] = make(map[string]int)
		}
		modelVersions[item.ScorerKey][item.ModelVersion]++
	}

	summary.CaseCount = len(caseLabels)
	for _, label := range caseLabels {
		if isPositiveMetricLabel(label, positiveSet) {
			summary.PositiveCases++
		} else {
			summary.NegativeCases++
		}
	}

	for _, scorerKey := range filter.ScorerKeys {
		scores := scoresByScorer[scorerKey]
		summary.Models[scorerKey] = ModelComparisonModel{
			ScorerKey:    scorerKey,
			ModelVersion: mostCommonString(modelVersions[scorerKey]),
			CaseCount:    len(scores),
			Metrics:      computeModelMetrics(scores, positiveSet),
		}
	}

	rankingKey := filter.ScorerKeys[len(filter.ScorerKeys)-1]
	if containsString(filter.ScorerKeys, "case_ensemble_v1") {
		rankingKey = "case_ensemble_v1"
	}
	rankingItems := make([]ModelComparisonTopCase, 0, len(scoresByCase))
	for caseID, scoreMap := range scoresByCase {
		rankingScore, ok := scoreMap[rankingKey]
		if !ok {
			continue
		}
		rankingItems = append(rankingItems, ModelComparisonTopCase{
			CaseID:          caseID,
			ExternalCaseID:  caseExternalIDs[caseID],
			EventName:       caseEvents[caseID],
			Label:           caseLabels[caseID],
			RankingScore:    rankingScore,
			ComponentScores: scoreMap,
		})
	}
	sort.SliceStable(rankingItems, func(i, j int) bool {
		if rankingItems[i].RankingScore == rankingItems[j].RankingScore {
			return rankingItems[i].CaseID < rankingItems[j].CaseID
		}
		return rankingItems[i].RankingScore > rankingItems[j].RankingScore
	})
	if len(rankingItems) > filter.TopK {
		rankingItems = rankingItems[:filter.TopK]
	}
	summary.TopCases = rankingItems

	return summary, nil
}

func marshalJSONObject(payload map[string]interface{}) ([]byte, error) {
	if len(payload) == 0 {
		return []byte(`{}`), nil
	}
	return json.Marshal(payload)
}

func decodeJSONMap(raw []byte) (map[string]interface{}, error) {
	if len(raw) == 0 {
		return map[string]interface{}{}, nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]interface{}{}
	}
	return out, nil
}

func casePostDataToItem(raw CasePostData) CasePostItem {
	return CasePostItem{
		ID:               raw.ID,
		ExternalID:       raw.ExternalID,
		AccountID:        raw.AccountID,
		Username:         raw.Username,
		PublishedAt:      raw.PublishedAt,
		Content:          raw.Content,
		IsCaseRoot:       raw.IsCaseRoot,
		ReplyToPostID:    raw.ReplyToPostID,
		LikesCount:       raw.LikesCount,
		RepostsCount:     raw.RepostsCount,
		RepliesCount:     raw.RepliesCount,
		FollowersCount:   raw.FollowersCount,
		FollowingCount:   raw.FollowingCount,
		PostsCount:       raw.PostsCount,
		IsVerified:       raw.IsVerified,
		AccountCreatedAt: raw.AccountCreatedAt,
		AccountURL:       raw.AccountURL,
		Tags:             raw.Tags,
		Links:            raw.Links,
	}
}

func normalizeScorerKey(raw string) string {
	return strings.TrimSpace(raw)
}

func useCaseModelScore(scorerKey string) bool {
	scorerKey = normalizeScorerKey(scorerKey)
	if scorerKey == "" {
		return false
	}
	switch scorerKey {
	case "case_scores", "active", "default":
		return false
	default:
		return true
	}
}

func normalizeScorerKeys(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		key := strings.TrimSpace(value)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func normalizePositiveLabels(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		label := normalizeLabelForMetrics(value)
		if label == "" {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	sort.Strings(out)
	return out
}

type labeledScore struct {
	CaseID int64
	Label  string
	Score  float64
}

func computeModelMetrics(items []labeledScore, positiveSet map[string]struct{}) ModelMetrics {
	if len(items) == 0 {
		return ModelMetrics{}
	}
	threshold := bestMetricThreshold(items, positiveSet)
	precision, recall, f1 := thresholdClassificationMetrics(items, positiveSet, threshold)
	return ModelMetrics{
		BestThreshold: &threshold,
		PrecisionAt10: precisionAtKMetric(items, positiveSet, 10),
		PrecisionAt20: precisionAtKMetric(items, positiveSet, 20),
		Precision:     precision,
		Recall:        recall,
		F1:            f1,
		ROCAUC:        rocAUCMetric(items, positiveSet),
		PRAUC:         prAUCMetric(items, positiveSet),
	}
}

func bestMetricThreshold(items []labeledScore, positiveSet map[string]struct{}) float64 {
	bestThreshold := 0.5
	bestF1 := -1.0
	for i := 0; i <= 100; i++ {
		threshold := float64(i) / 100.0
		_, _, f1 := thresholdClassificationMetrics(items, positiveSet, threshold)
		f1Value := 0.0
		if f1 != nil {
			f1Value = *f1
		}
		if f1Value > bestF1 {
			bestF1 = f1Value
			bestThreshold = threshold
		}
	}
	return bestThreshold
}

func thresholdClassificationMetrics(items []labeledScore, positiveSet map[string]struct{}, threshold float64) (*float64, *float64, *float64) {
	if len(items) == 0 {
		return nil, nil, nil
	}
	tp, fp, fn := 0, 0, 0
	for _, item := range items {
		actual := isPositiveMetricLabel(item.Label, positiveSet)
		predicted := item.Score >= threshold
		switch {
		case actual && predicted:
			tp++
		case !actual && predicted:
			fp++
		case actual && !predicted:
			fn++
		}
	}
	precision := 0.0
	if tp+fp > 0 {
		precision = float64(tp) / float64(tp+fp)
	}
	recall := 0.0
	if tp+fn > 0 {
		recall = float64(tp) / float64(tp+fn)
	}
	f1 := 0.0
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}
	return &precision, &recall, &f1
}

func precisionAtKMetric(items []labeledScore, positiveSet map[string]struct{}, k int) *float64 {
	if len(items) == 0 || k <= 0 {
		return nil
	}
	sortedItems := append([]labeledScore(nil), items...)
	sort.SliceStable(sortedItems, func(i, j int) bool {
		if sortedItems[i].Score == sortedItems[j].Score {
			return sortedItems[i].CaseID < sortedItems[j].CaseID
		}
		return sortedItems[i].Score > sortedItems[j].Score
	})
	topK := k
	if topK > len(sortedItems) {
		topK = len(sortedItems)
	}
	positive := 0
	for _, item := range sortedItems[:topK] {
		if isPositiveMetricLabel(item.Label, positiveSet) {
			positive++
		}
	}
	value := float64(positive) / float64(topK)
	return &value
}

func rocAUCMetric(items []labeledScore, positiveSet map[string]struct{}) *float64 {
	if len(items) == 0 {
		return nil
	}
	sortedItems := append([]labeledScore(nil), items...)
	sort.SliceStable(sortedItems, func(i, j int) bool {
		if sortedItems[i].Score == sortedItems[j].Score {
			return sortedItems[i].CaseID < sortedItems[j].CaseID
		}
		return sortedItems[i].Score < sortedItems[j].Score
	})

	pos, neg := 0, 0
	for _, item := range sortedItems {
		if isPositiveMetricLabel(item.Label, positiveSet) {
			pos++
		} else {
			neg++
		}
	}
	if pos == 0 || neg == 0 {
		return nil
	}

	sumPositiveRanks := 0.0
	for i := 0; i < len(sortedItems); {
		j := i + 1
		for j < len(sortedItems) && sortedItems[j].Score == sortedItems[i].Score {
			j++
		}
		averageRank := (float64(i+1) + float64(j)) / 2.0
		for k := i; k < j; k++ {
			if isPositiveMetricLabel(sortedItems[k].Label, positiveSet) {
				sumPositiveRanks += averageRank
			}
		}
		i = j
	}

	value := (sumPositiveRanks - float64(pos*(pos+1))/2.0) / float64(pos*neg)
	return &value
}

func prAUCMetric(items []labeledScore, positiveSet map[string]struct{}) *float64 {
	if len(items) == 0 {
		return nil
	}
	sortedItems := append([]labeledScore(nil), items...)
	sort.SliceStable(sortedItems, func(i, j int) bool {
		if sortedItems[i].Score == sortedItems[j].Score {
			return sortedItems[i].CaseID < sortedItems[j].CaseID
		}
		return sortedItems[i].Score > sortedItems[j].Score
	})
	totalPositive := 0
	for _, item := range sortedItems {
		if isPositiveMetricLabel(item.Label, positiveSet) {
			totalPositive++
		}
	}
	if totalPositive == 0 {
		return nil
	}
	positiveSeen := 0
	precisionSum := 0.0
	for idx, item := range sortedItems {
		if !isPositiveMetricLabel(item.Label, positiveSet) {
			continue
		}
		positiveSeen++
		precisionSum += float64(positiveSeen) / float64(idx+1)
	}
	value := precisionSum / float64(totalPositive)
	return &value
}

func isPositiveMetricLabel(label string, positiveSet map[string]struct{}) bool {
	_, ok := positiveSet[normalizeLabelForMetrics(label)]
	return ok
}

func normalizeLabelForMetrics(raw string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(raw)), "-", "_"), " ", "_")
}

func mostCommonString(counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}
	type pair struct {
		value string
		count int
	}
	pairs := make([]pair, 0, len(counts))
	for value, count := range counts {
		pairs = append(pairs, pair{value: value, count: count})
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		if pairs[i].count == pairs[j].count {
			return pairs[i].value < pairs[j].value
		}
		return pairs[i].count > pairs[j].count
	})
	return pairs[0].value
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

// SavePost сохраняет пост в базу данных.
func (p *PostgresDB) SavePost(sourceID int, status *mastodon.Status, accountID int64) error {
	defaultCtx := IngestionContext{
		SourceType:    "live",
		RawPayloadRef: status.URL,
	}
	return p.SavePostWithIngestion(sourceID, status, accountID, defaultCtx)
}

// SavePostWithIngestion сохраняет пост в базу с metadata о происхождении payload.
func (p *PostgresDB) SavePostWithIngestion(sourceID int, status *mastodon.Status, accountID int64, ctx IngestionContext) error {
	metadata, _ := json.Marshal(map[string]interface{}{
		"mentions": status.Mentions,
		"media":    status.MediaAttachments,
	})

	sourceType := ctx.SourceType
	if sourceType == "" {
		sourceType = "live"
	}

	rawPayloadRef := ctx.RawPayloadRef
	if rawPayloadRef == "" {
		rawPayloadRef = status.URL
	}

	query := `
		INSERT INTO posts (source_id, external_id, account_id, content, language, 
		                   published_at, collected_at, post_url, likes_count, 
		                   reposts_count, replies_count, metadata, source_type,
		                   raw_payload_ref, raw_payload_hash, dataset_name,
		                   dataset_split, dataset_record_id, ingestion_run_id)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		ON CONFLICT (source_id, external_id) DO NOTHING
	`

	_, err := p.db.Exec(query,
		sourceID,
		status.ID,
		accountID,
		status.Content,
		status.Language,
		status.CreatedAt,
		status.URL,
		status.FavouritesCount,
		status.ReblogsCount,
		status.RepliesCount,
		metadata,
		sourceType,
		rawPayloadRef,
		ctx.RawPayloadHash,
		ctx.DatasetName,
		ctx.DatasetSplit,
		ctx.DatasetRecordID,
		ctx.IngestionRunID,
	)

	return err
}

// SaveTrendingTag сохраняет трендовый хэштег в базу данных.
func (p *PostgresDB) SaveTrendingTag(sourceID int, tag *mastodon.TrendingTag) error {
	historyJSON, _ := json.Marshal(tag.History)

	var todayAccounts, todayUses int
	if len(tag.History) > 0 {
		fmt.Sscanf(tag.History[0].Accounts, "%d", &todayAccounts)
		fmt.Sscanf(tag.History[0].Uses, "%d", &todayUses)
	}

	query := `
		INSERT INTO trending_tags (source_id, tag_name, history, today_accounts, today_uses, last_seen_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (source_id, tag_name) 
		DO UPDATE SET 
			history = EXCLUDED.history,
			today_accounts = EXCLUDED.today_accounts,
			today_uses = EXCLUDED.today_uses,
			last_seen_at = NOW()
	`

	_, err := p.db.Exec(query, sourceID, tag.Name, historyJSON, todayAccounts, todayUses)
	return err
}

// SaveTrendingLink сохраняет трендовую ссылку в базу данных.
func (p *PostgresDB) SaveTrendingLink(sourceID int, link *mastodon.TrendingLink) error {
	historyJSON, _ := json.Marshal(link.History)

	var todayAccounts, todayUses int
	if len(link.History) > 0 {
		fmt.Sscanf(link.History[0].Accounts, "%d", &todayAccounts)
		fmt.Sscanf(link.History[0].Uses, "%d", &todayUses)
	}

	query := `
		INSERT INTO trending_links (source_id, url, title, description, provider_name, 
		                            image_url, history, today_accounts, today_uses, last_seen_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (source_id, url) 
		DO UPDATE SET 
			title = EXCLUDED.title,
			description = EXCLUDED.description,
			history = EXCLUDED.history,
			today_accounts = EXCLUDED.today_accounts,
			today_uses = EXCLUDED.today_uses,
			last_seen_at = NOW()
	`

	_, err := p.db.Exec(query,
		sourceID,
		link.URL,
		link.Title,
		link.Description,
		link.ProviderName,
		link.Image,
		historyJSON,
		todayAccounts,
		todayUses,
	)

	return err
}

// GetUnanalyzedPosts возвращает посты, которые ещё не были проанализированы.
func (p *PostgresDB) GetUnanalyzedPosts(limit int) ([]UnanalyzedPost, error) {
	query := `
		SELECT p.id, p.content
		FROM posts p
		LEFT JOIN analysis_results ar ON p.id = ar.post_id
		WHERE ar.id IS NULL
		LIMIT $1
	`

	rows, err := p.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []UnanalyzedPost
	for rows.Next() {
		var post UnanalyzedPost
		if err := rows.Scan(&post.ID, &post.Content); err != nil {
			return nil, err
		}
		posts = append(posts, post)
	}
	return posts, nil
}

// SaveAnalysisResult сохраняет результат анализа в базу данных.
func (p *PostgresDB) SaveAnalysisResult(postID int64, mlResp *MLResponse, escalationPriority int) error {
	// Сохраняем в таблицу analysis_results
	query := `
		INSERT INTO analysis_results (
			post_id, manipulation_score, confidence_score,
			coordination_contribution, temporal_contribution, narrative_contribution,
			escalation_priority, confidence_note, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (post_id) DO UPDATE SET
			manipulation_score = EXCLUDED.manipulation_score,
			confidence_score = EXCLUDED.confidence_score,
			coordination_contribution = EXCLUDED.coordination_contribution,
			temporal_contribution = EXCLUDED.temporal_contribution,
			narrative_contribution = EXCLUDED.narrative_contribution,
			escalation_priority = EXCLUDED.escalation_priority,
			confidence_note = EXCLUDED.confidence_note
	`

	_, err := p.db.Exec(query,
		postID,
		mlResp.ManipulationScore,
		mlResp.ConfidenceScore,
		mlResp.CoordinationContribution,
		mlResp.TemporalContribution,
		mlResp.NarrativeContribution,
		escalationPriority,
		mlResp.ConfidenceNote,
	)
	if err != nil {
		return err
	}

	// Сохраняем evidence в таблицу evidence_cards
	evidenceJSON, err := json.Marshal(mlResp.KeyEvidence)
	if err != nil {
		return err
	}
	evidenceJSONString := string(evidenceJSON)

	// Получаем analysis_result_id
	var analysisID int64
	err = p.db.QueryRow(`
		SELECT id FROM analysis_results WHERE post_id = $1
	`, postID).Scan(&analysisID)
	if err != nil {
		return err
	}

	// radar_data для визуализации
	radarData := map[string]float64{
		"coordination": mlResp.CoordinationContribution,
		"temporal":     mlResp.TemporalContribution,
		"narrative":    mlResp.NarrativeContribution,
	}
	radarJSON, err := json.Marshal(radarData)
	if err != nil {
		return err
	}
	radarJSONString := string(radarJSON)

	_, err = p.db.Exec(`
		INSERT INTO evidence_cards (
			analysis_result_id, radar_data, key_evidence, summary, created_at
		)
		VALUES ($1, $2::jsonb, $3::jsonb, $4, NOW())
		ON CONFLICT (analysis_result_id) DO UPDATE SET
			radar_data = EXCLUDED.radar_data,
			key_evidence = EXCLUDED.key_evidence,
			summary = EXCLUDED.summary
	`, analysisID, radarJSONString, evidenceJSONString, mlResp.ConfidenceNote)

	return err
}

// GetAnalysisResults возвращает результаты анализа
func (p *PostgresDB) GetAnalysisResults(limit int) ([]map[string]interface{}, error) {
	query := `
		SELECT ar.id, ar.post_id, ar.manipulation_score, ar.confidence_score,
		       ar.coordination_contribution, ar.temporal_contribution, 
		       ar.narrative_contribution, ar.escalation_priority, ar.confidence_note,
		       ar.created_at, p.content, p.likes_count, p.reposts_count,
		       a.username, a.display_name
		FROM analysis_results ar
		JOIN posts p ON ar.post_id = p.id
		JOIN accounts a ON p.account_id = a.id
		ORDER BY ar.manipulation_score DESC
		LIMIT $1
	`

	rows, err := p.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id, postID int64
		var manipScore, confScore, coordCont, tempCont, narrCont float64
		var escPriority int
		var confNote, content, username, displayName string
		var likes, reposts int
		var createdAt time.Time

		err := rows.Scan(&id, &postID, &manipScore, &confScore, &coordCont, &tempCont,
			&narrCont, &escPriority, &confNote, &createdAt, &content,
			&likes, &reposts, &username, &displayName)
		if err != nil {
			return nil, err
		}

		result := map[string]interface{}{
			"id":                        id,
			"post_id":                   postID,
			"manipulation_score":        manipScore,
			"confidence_score":          confScore,
			"coordination_contribution": coordCont,
			"temporal_contribution":     tempCont,
			"narrative_contribution":    narrCont,
			"escalation_priority":       escPriority,
			"confidence_note":           confNote,
			"created_at":                createdAt,
			"post_content":              content,
			"likes_count":               likes,
			"reposts_count":             reposts,
			"author_username":           username,
			"author_display_name":       displayName,
		}
		results = append(results, result)
	}
	return results, nil
}

// GetDashboardStats возвращает статистику
func (p *PostgresDB) GetDashboardStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	var totalPosts int
	p.db.QueryRow("SELECT COUNT(*) FROM posts").Scan(&totalPosts)
	stats["total_posts"] = totalPosts

	var high, medium, low int
	p.db.QueryRow(`SELECT COUNT(*) FROM analysis_results WHERE escalation_priority = 1`).Scan(&high)
	p.db.QueryRow(`SELECT COUNT(*) FROM analysis_results WHERE escalation_priority = 2`).Scan(&medium)
	p.db.QueryRow(`SELECT COUNT(*) FROM analysis_results WHERE escalation_priority = 3`).Scan(&low)
	stats["high_priority_count"] = high
	stats["medium_priority_count"] = medium
	stats["low_priority_count"] = low

	var avgManip, avgConf float64
	p.db.QueryRow(`
		SELECT COALESCE(AVG(manipulation_score), 0), COALESCE(AVG(confidence_score), 0)
		FROM analysis_results
	`).Scan(&avgManip, &avgConf)
	stats["avg_manipulation_score"] = avgManip
	stats["avg_confidence"] = avgConf

	return stats, nil
}

// GetTrendingTags возвращает трендовые хэштеги
func (p *PostgresDB) GetTrendingTags(limit int) ([]map[string]interface{}, error) {
	query := `
		SELECT id, tag_name, today_accounts, today_uses, last_seen_at
		FROM trending_tags
		ORDER BY today_uses DESC
		LIMIT $1
	`

	rows, err := p.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []map[string]interface{}
	for rows.Next() {
		var id int64
		var tagName string
		var todayAccounts, todayUses int
		var lastSeen time.Time

		err := rows.Scan(&id, &tagName, &todayAccounts, &todayUses, &lastSeen)
		if err != nil {
			return nil, err
		}

		tag := map[string]interface{}{
			"id":             id,
			"tag_name":       tagName,
			"today_accounts": todayAccounts,
			"today_uses":     todayUses,
			"last_seen_at":   lastSeen,
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

type PostFullData struct {
	ID               int64
	Content          string
	AccountID        int64
	PublishedAt      time.Time
	Username         string
	FollowersCount   int
	AccountCreatedAt time.Time
}

func (p *PostgresDB) GetPostWithAccount(postID int64) (*PostFullData, error) {
	query := `
		SELECT p.id, p.content, p.account_id, p.published_at,
		       a.username, a.followers_count, a.created_at
		FROM posts p
		JOIN accounts a ON p.account_id = a.id
		WHERE p.id = $1
	`

	var data PostFullData
	err := p.db.QueryRow(query, postID).Scan(
		&data.ID, &data.Content, &data.AccountID, &data.PublishedAt,
		&data.Username, &data.FollowersCount, &data.AccountCreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &data, nil
}
