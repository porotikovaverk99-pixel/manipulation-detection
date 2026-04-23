package main

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/collector"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

func main() {
	_ = godotenv.Load()

	connStr := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if connStr == "" {
		connStr = strings.TrimSpace(os.Getenv("DATABASE_URI"))
	}
	if connStr == "" {
		log.Fatal("DATABASE_URL or DATABASE_URI is required")
	}

	filter := repository.CaseFilter{
		SourceName:   getenvDefault("SOURCE_NAME", ""),
		DatasetName:  getenvDefault("DATASET_NAME", ""),
		DatasetSplit: getenvDefault("DATASET_SPLIT", ""),
		Label:        getenvDefault("CASE_LABEL", ""),
		OnlyUnscored: getenvBool("ONLY_UNSCORED", true),
		Limit:        getenvInt("CASE_LIMIT", 20),
	}

	db, err := repository.NewPostgresDB(connStr)
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}
	defer db.Close()

	cases, err := db.GetCasesForScoring(filter)
	if err != nil {
		log.Fatalf("load cases for ML scoring failed: %v", err)
	}
	if len(cases) == 0 {
		log.Printf("no cases selected for ML scoring")
		return
	}

	mlURL := getenvDefault("ML_SERVICE_URL", "http://localhost:8000")
	mlClient := collector.NewMLClient(mlURL)

	processed := 0
	failed := 0
	started := time.Now()

	log.Printf(
		"case ML scorer started: cases=%d dataset=%s split=%s source=%s ml=%s only_unscored=%t",
		len(cases),
		filter.DatasetName,
		filter.DatasetSplit,
		filter.SourceName,
		mlURL,
		filter.OnlyUnscored,
	)

	for _, c := range cases {
		req := buildCaseMLRequest(c)
		resp, err := mlClient.AnalyzeCase(req)
		if err != nil {
			failed++
			log.Printf("case #%d ML scoring failed: %v", c.ID, err)
			continue
		}

		features := repository.CaseFeaturesRecord{
			CaseID:               c.ID,
			FeatureVersion:       resp.FeatureVersion,
			EventCount:           resp.EventCount,
			UniqueAccountCount:   resp.UniqueAccountCount,
			UniqueURLCount:       resp.UniqueURLCount,
			UniqueHashtagCount:   resp.UniqueHashtagCount,
			TemporalFeatures:     copyMap(resp.TemporalFeatures),
			CoordinationFeatures: copyMap(resp.CoordinationFeatures),
			ContentFeatures:      copyMap(resp.ContentFeatures),
			FeaturePayload: mergeMaps(
				copyMap(resp.FeaturePayload),
				map[string]interface{}{
					"confidence_score": resp.ConfidenceScore,
					"model_info":       resp.ModelInfo,
					"scoring_source":   "ml_service",
					"ml_service_url":   mlURL,
				},
			),
		}
		score := repository.CaseScoreRecord{
			CaseID:            c.ID,
			ScoreVersion:      resp.ScoreVersion,
			TemporalScore:     resp.TemporalScore,
			CoordinationScore: resp.CoordinationScore,
			ContentScore:      resp.ContentScore,
			RiskScore:         resp.RiskScore,
			RiskLevel:         resp.RiskLevel,
			Evidence:          append([]string(nil), resp.Evidence...),
			PipelineHash:      resp.PipelineHash,
		}

		if err := db.SaveCaseFeatures(features); err != nil {
			failed++
			log.Printf("case #%d save features failed: %v", c.ID, err)
			continue
		}
		if err := db.SaveCaseScore(score); err != nil {
			failed++
			log.Printf("case #%d save score failed: %v", c.ID, err)
			continue
		}

		processed++
		log.Printf(
			"case #%d ML scored: external_case_id=%s risk=%.3f level=%s confidence=%.3f",
			c.ID,
			c.ExternalCaseID,
			resp.RiskScore,
			resp.RiskLevel,
			resp.ConfidenceScore,
		)
	}

	log.Printf(
		"case ML scorer finished: processed=%d failed=%d duration=%s",
		processed,
		failed,
		time.Since(started).Round(time.Millisecond),
	)
}

func buildCaseMLRequest(c repository.CaseForScoring) collector.CaseMLRequest {
	req := collector.CaseMLRequest{
		CaseID:         c.ID,
		ExternalCaseID: c.ExternalCaseID,
		SourceName:     c.SourceName,
		DatasetName:    c.DatasetName,
		DatasetSplit:   c.DatasetSplit,
		CaseType:       c.CaseType,
		Label:          c.Label,
		Title:          c.Title,
		EventName:      c.EventName,
		Posts:          make([]collector.CaseMLPost, 0, len(c.Posts)),
	}
	if c.FirstEventAt != nil {
		req.FirstEventAt = c.FirstEventAt.UTC().Format(time.RFC3339)
	}
	if c.LastEventAt != nil {
		req.LastEventAt = c.LastEventAt.UTC().Format(time.RFC3339)
	}

	for _, post := range c.Posts {
		reqPost := collector.CaseMLPost{
			PostID:      post.ID,
			ExternalID:  post.ExternalID,
			AccountID:   post.AccountID,
			Username:    post.Username,
			PublishedAt: post.PublishedAt.UTC().Format(time.RFC3339),
			Content:     post.Content,
			IsCaseRoot:  post.IsCaseRoot,
			Tags:        cloneStrings(post.Tags),
			Links:       cloneStrings(post.Links),
		}
		if post.ReplyToPostID != nil {
			replyID := *post.ReplyToPostID
			reqPost.ReplyToPostID = &replyID
		}
		req.Posts = append(req.Posts, reqPost)
	}

	return req
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func copyMap(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return map[string]interface{}{}
	}
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func mergeMaps(base map[string]interface{}, extra map[string]interface{}) map[string]interface{} {
	if len(base) == 0 {
		base = map[string]interface{}{}
	}
	for key, value := range extra {
		base[key] = value
	}
	return base
}

func getenvDefault(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func getenvInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func getenvBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return parsed
}
