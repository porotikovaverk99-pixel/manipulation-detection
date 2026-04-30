package main

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/audittrail"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/collector"
	applog "github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/logger"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

const (
	scorerFeature = "feature"
	scorerText    = "text"
)

func main() {
	_ = godotenv.Load()
	logFile, err := applog.ConfigureStandardLog("case_ml_scorer")
	if err != nil {
		log.Printf("configure file logging failed: %v", err)
	}
	if logFile != nil {
		defer logFile.Close()
	}

	connStr := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if connStr == "" {
		connStr = strings.TrimSpace(os.Getenv("DATABASE_URI"))
	}
	if connStr == "" {
		log.Fatal("DATABASE_URL or DATABASE_URI is required")
	}

	scorers := parseScorers(getenvDefault("CASE_SCORERS", scorerFeature))
	modelScoreKeys := make([]string, 0, len(scorers))
	for _, scorer := range scorers {
		modelScoreKeys = append(modelScoreKeys, scorerModelKey(scorer))
	}

	filter := repository.CaseFilter{
		SourceName:            getenvDefault("SOURCE_NAME", ""),
		DatasetName:           getenvDefault("DATASET_NAME", ""),
		DatasetSplit:          getenvDefault("DATASET_SPLIT", ""),
		Label:                 getenvDefault("CASE_LABEL", ""),
		OnlyUnscored:          getenvBool("ONLY_UNSCORED", true),
		MissingModelScoreKeys: modelScoreKeys,
		Limit:                 getenvInt("CASE_LIMIT", 20),
	}

	db, err := repository.NewPostgresDB(connStr)
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}
	defer db.Close()

	job := audittrail.NewCLIJob(db, "case_ml_scorer").
		WithSource("dataset", filter.SourceName, filter.DatasetName, filter.DatasetSplit).
		WithPayload(map[string]interface{}{
			"case_limit":    filter.Limit,
			"case_label":    filter.Label,
			"only_unscored": filter.OnlyUnscored,
			"scorers":       strings.Join(scorers, ","),
			"model_keys":    strings.Join(modelScoreKeys, ","),
		})
	job.Start()
	defer job.FinishAndExit()

	cases, err := db.GetCasesForScoring(filter)
	if err != nil {
		job.Failf("load cases for ML scoring failed: %v", err)
		return
	}
	if len(cases) == 0 {
		job.Skipf("no cases selected for ML scoring")
		return
	}
	job.Set("selected_cases", len(cases))

	mlURL := getenvDefault("ML_SERVICE_URL", "http://localhost:8000")
	job.Set("ml_service_url", mlURL)
	mlClient := collector.NewMLClient(mlURL)

	processed := 0
	failed := 0
	started := time.Now()

	log.Printf(
		"case ML scorer started: cases=%d dataset=%s split=%s source=%s ml=%s scorers=%s only_unscored=%t",
		len(cases),
		filter.DatasetName,
		filter.DatasetSplit,
		filter.SourceName,
		mlURL,
		strings.Join(scorers, ","),
		filter.OnlyUnscored,
	)

	for _, c := range cases {
		req := buildCaseMLRequest(c)
		caseSucceeded := false

		if hasScorer(scorers, scorerFeature) {
			resp, err := mlClient.AnalyzeCase(req)
			if err != nil {
				failed++
				log.Printf("case #%d feature ML scoring failed: %v", c.ID, err)
			} else if err := saveFeatureScore(db, c, resp, mlURL); err != nil {
				failed++
				log.Printf("case #%d save feature score failed: %v", c.ID, err)
			} else {
				caseSucceeded = true
				log.Printf(
					"case #%d feature ML scored: external_case_id=%s risk=%.3f level=%s confidence=%.3f",
					c.ID,
					c.ExternalCaseID,
					resp.RiskScore,
					resp.RiskLevel,
					resp.ConfidenceScore,
				)
			}
		}

		if hasScorer(scorers, scorerText) {
			resp, err := mlClient.AnalyzeCaseText(req)
			if err != nil {
				failed++
				log.Printf("case #%d text ML scoring failed: %v", c.ID, err)
			} else if err := saveTextScore(db, c, resp, mlURL); err != nil {
				failed++
				log.Printf("case #%d save text score failed: %v", c.ID, err)
			} else {
				caseSucceeded = true
				log.Printf(
					"case #%d text ML scored: external_case_id=%s risk=%.3f level=%s confidence=%.3f reactions=%d",
					c.ID,
					c.ExternalCaseID,
					resp.RiskScore,
					resp.RiskLevel,
					resp.ConfidenceScore,
					resp.ReactionCountUsed,
				)
			}
		}

		if caseSucceeded {
			processed++
		}
	}

	log.Printf(
		"case ML scorer finished: processed=%d failed=%d duration=%s",
		processed,
		failed,
		time.Since(started).Round(time.Millisecond),
	)
	job.Set("processed", processed)
	job.Set("failed", failed)
	if processed == 0 && failed > 0 {
		job.Failf("case ML scorer failed for all selected cases: failed=%d", failed)
	}
}

func saveFeatureScore(db *repository.PostgresDB, c repository.CaseForScoring, resp *collector.CaseMLResponse, mlURL string) error {
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
	modelScore := repository.CaseModelScoreRecord{
		CaseID:            c.ID,
		ScorerKey:         scorerModelKey(scorerFeature),
		ModelVersion:      resp.ScoreVersion,
		RiskScore:         resp.RiskScore,
		RiskLevel:         resp.RiskLevel,
		ConfidenceScore:   floatPtr(resp.ConfidenceScore),
		TemporalScore:     floatPtr(resp.TemporalScore),
		CoordinationScore: floatPtr(resp.CoordinationScore),
		ContentScore:      floatPtr(resp.ContentScore),
		Evidence:          append([]string(nil), resp.Evidence...),
		FeaturePayload: mergeMaps(
			copyMap(resp.FeaturePayload),
			map[string]interface{}{
				"feature_version":   resp.FeatureVersion,
				"scoring_source":    "ml_service",
				"ml_service_url":    mlURL,
				"active_case_score": true,
			},
		),
		ModelInfo:      copyMap(resp.ModelInfo),
		PipelineHash:   resp.PipelineHash,
		SourceEndpoint: "/analyze/case",
	}

	if err := db.SaveCaseFeatures(features); err != nil {
		return err
	}
	if err := db.SaveCaseScore(score); err != nil {
		return err
	}
	return db.SaveCaseModelScore(modelScore)
}

func saveTextScore(db *repository.PostgresDB, c repository.CaseForScoring, resp *collector.CaseTextMLResponse, mlURL string) error {
	modelScore := repository.CaseModelScoreRecord{
		CaseID:          c.ID,
		ScorerKey:       scorerModelKey(scorerText),
		ModelVersion:    resp.ModelVersion,
		RiskScore:       resp.RiskScore,
		RiskLevel:       resp.RiskLevel,
		ConfidenceScore: floatPtr(resp.ConfidenceScore),
		Evidence:        append([]string(nil), resp.Evidence...),
		FeaturePayload: mergeMaps(
			copyMap(resp.FeaturePayload),
			map[string]interface{}{
				"model_path":          resp.ModelPath,
				"text_mode":           resp.TextMode,
				"max_length":          resp.MaxLength,
				"reaction_count_used": resp.ReactionCountUsed,
				"scoring_source":      "ml_service",
				"ml_service_url":      mlURL,
			},
		),
		ModelInfo:      copyMap(resp.ModelInfo),
		PipelineHash:   resp.PipelineHash,
		SourceEndpoint: "/analyze/case-text",
	}
	return db.SaveCaseModelScore(modelScore)
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
			PostID:         post.ID,
			ExternalID:     post.ExternalID,
			AccountID:      post.AccountID,
			Username:       post.Username,
			PublishedAt:    post.PublishedAt.UTC().Format(time.RFC3339),
			Content:        post.Content,
			IsCaseRoot:     post.IsCaseRoot,
			LikesCount:     post.LikesCount,
			RepostsCount:   post.RepostsCount,
			RepliesCount:   post.RepliesCount,
			FollowersCount: post.FollowersCount,
			FollowingCount: post.FollowingCount,
			PostsCount:     post.PostsCount,
			IsVerified:     post.IsVerified,
			AccountURL:     post.AccountURL,
			Tags:           cloneStrings(post.Tags),
			Links:          cloneStrings(post.Links),
		}
		if post.AccountCreatedAt != nil {
			reqPost.AccountCreatedAt = post.AccountCreatedAt.UTC().Format(time.RFC3339)
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

func parseScorers(raw string) []string {
	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		key := strings.ToLower(strings.TrimSpace(part))
		if key == "" {
			continue
		}
		if key == "all" {
			for _, scorer := range []string{scorerFeature, scorerText} {
				if _, ok := seen[scorer]; !ok {
					seen[scorer] = struct{}{}
					out = append(out, scorer)
				}
			}
			continue
		}
		if key != scorerFeature && key != scorerText {
			log.Fatalf("unsupported CASE_SCORERS value %q (supported: feature,text,all)", key)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	if len(out) == 0 {
		return []string{scorerFeature}
	}
	return out
}

func hasScorer(scorers []string, scorer string) bool {
	for _, item := range scorers {
		if item == scorer {
			return true
		}
	}
	return false
}

func scorerModelKey(scorer string) string {
	switch scorer {
	case scorerFeature:
		return getenvDefault("CASE_FEATURE_SCORER_KEY", "case_feature")
	case scorerText:
		return getenvDefault("CASE_TEXT_SCORER_KEY", "pheme_transformer_text")
	default:
		return scorer
	}
}

func floatPtr(value float64) *float64 {
	return &value
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
