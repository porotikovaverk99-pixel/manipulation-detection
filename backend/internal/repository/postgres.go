// Package repository предоставляет функции для работы с базой данных PostgreSQL.
package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
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

	_, err = p.db.Exec(`
		INSERT INTO evidence_cards (
			analysis_result_id, radar_data, key_evidence, summary, created_at
		)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (analysis_result_id) DO UPDATE SET
			radar_data = EXCLUDED.radar_data,
			key_evidence = EXCLUDED.key_evidence,
			summary = EXCLUDED.summary
	`, analysisID, radarJSON, evidenceJSON, mlResp.ConfidenceNote)

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
