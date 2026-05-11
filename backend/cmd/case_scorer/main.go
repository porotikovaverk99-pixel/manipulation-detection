package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/joho/godotenv"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

var (
	urgencyMarkers = []string{"breaking", "urgent", "alert", "now", "immediately", "warning", "must", "watch"}
	claimMarkers   = []string{"reportedly", "rumour", "rumor", "claim", "claims", "according to", "unconfirmed", "reports"}
	denialMarkers  = []string{"false", "fake", "hoax", "debunk", "not true", "isn't true", "is not true", "no evidence"}
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
	featureVersion := getenvDefault("FEATURE_VERSION", "case-features-v1")
	scoreVersion := getenvDefault("SCORE_VERSION", "case-score-v1")
	pipelineHash := computePipelineHash(featureVersion, scoreVersion)

	db, err := repository.NewPostgresDB(connStr)
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}
	defer db.Close()

	cases, err := db.GetCasesForScoring(filter)
	if err != nil {
		log.Fatalf("load cases for scoring failed: %v", err)
	}
	if len(cases) == 0 {
		log.Printf("no cases selected for scoring")
		return
	}

	scored := 0
	for _, c := range cases {
		features, score := scoreCase(c, featureVersion, scoreVersion, pipelineHash)
		if err := db.SaveCaseFeatures(features); err != nil {
			log.Fatalf("save features for case %d failed: %v", c.ID, err)
		}
		if err := db.SaveCaseScore(score); err != nil {
			log.Fatalf("save score for case %d failed: %v", c.ID, err)
		}
		if err := db.SaveCaseModelScore(caseModelScore(c.ID, score)); err != nil {
			log.Fatalf("save model score for case %d failed: %v", c.ID, err)
		}
		scored++
		log.Printf("scored case=%d external_case_id=%s risk=%.3f level=%s", c.ID, c.ExternalCaseID, score.RiskScore, score.RiskLevel)
	}

	log.Printf("case scorer completed: scored=%d", scored)
}

func scoreCase(
	c repository.CaseForScoring,
	featureVersion, scoreVersion, pipelineHash string,
) (repository.CaseFeaturesRecord, repository.CaseScoreRecord) {
	eventCount := len(c.Posts)
	accountCounts := make(map[int64]int)
	urlCounts := make(map[string]int)
	tagSet := make(map[string]struct{})
	textCounts := make(map[string]int)
	postTextsWithUrgency := 0
	postTextsWithClaim := 0
	postTextsWithDenial := 0
	replyCount := 0
	totalURLRefs := 0

	firstEvent := zeroTimeToNow(c.FirstEventAt)
	lastEvent := zeroTimeToNow(c.LastEventAt)
	if eventCount > 0 {
		firstEvent = c.Posts[0].PublishedAt.UTC()
		lastEvent = c.Posts[0].PublishedAt.UTC()
	}

	firstWindowCount := 0
	var gapSum float64

	for idx, post := range c.Posts {
		accountCounts[post.AccountID]++

		ts := post.PublishedAt.UTC()
		if ts.Before(firstEvent) {
			firstEvent = ts
		}
		if ts.After(lastEvent) {
			lastEvent = ts
		}

		if post.ReplyToPostID != nil {
			replyCount++
		}

		normalized := normalizeText(post.Content)
		if normalized != "" {
			textCounts[normalized]++
		}

		lowered := strings.ToLower(post.Content)
		if containsAny(lowered, urgencyMarkers) {
			postTextsWithUrgency++
		}
		if containsAny(lowered, claimMarkers) {
			postTextsWithClaim++
		}
		if containsAny(lowered, denialMarkers) {
			postTextsWithDenial++
		}

		for _, tag := range post.Tags {
			tagSet[strings.ToLower(strings.TrimSpace(tag))] = struct{}{}
		}
		for _, link := range post.Links {
			resolved := strings.TrimSpace(strings.ToLower(link))
			if resolved == "" {
				continue
			}
			urlCounts[resolved]++
			totalURLRefs++
		}

		if idx > 0 {
			gap := ts.Sub(c.Posts[idx-1].PublishedAt.UTC()).Seconds()
			if gap > 0 {
				gapSum += gap
			}
		}
	}

	firstWindowEnd := firstEvent.Add(time.Hour)
	for _, post := range c.Posts {
		if !post.PublishedAt.UTC().After(firstWindowEnd) {
			firstWindowCount++
		}
	}

	spanSeconds := math.Max(0, lastEvent.Sub(firstEvent).Seconds())
	spanHours := math.Max(spanSeconds/3600.0, 1.0/60.0)
	avgGapSeconds := 3600.0
	if eventCount > 1 {
		avgGapSeconds = gapSum / float64(eventCount-1)
		if avgGapSeconds <= 0 {
			avgGapSeconds = 3600.0
		}
	}

	maxAuthorPosts := 0
	for _, count := range accountCounts {
		if count > maxAuthorPosts {
			maxAuthorPosts = count
		}
	}

	duplicatePosts := 0
	for _, count := range textCounts {
		if count > 1 {
			duplicatePosts += count - 1
		}
	}

	repeatedURLRefs := 0
	for _, count := range urlCounts {
		if count > 1 {
			repeatedURLRefs += count - 1
		}
	}

	firstWindowShare := safeRatio(float64(firstWindowCount), float64(eventCount))
	activityDensity := clamp01(float64(eventCount) / (spanHours * 10.0))
	gapIntensity := clamp01(1800.0 / math.Max(avgGapSeconds, 60.0))

	authorConcentration := safeRatio(float64(maxAuthorPosts), float64(eventCount))
	replyRatio := safeRatio(float64(replyCount), float64(max(1, eventCount-1)))
	duplicateTextRatio := safeRatio(float64(duplicatePosts), float64(eventCount))
	repeatedURLRatio := 0.0
	if totalURLRefs > 0 {
		repeatedURLRatio = safeRatio(float64(repeatedURLRefs), float64(totalURLRefs))
	}

	urgencyRatio := safeRatio(float64(postTextsWithUrgency), float64(eventCount))
	claimRatio := safeRatio(float64(postTextsWithClaim), float64(eventCount))
	denialRatio := safeRatio(float64(postTextsWithDenial), float64(eventCount))
	lexicalRepetitionRatio := duplicateTextRatio

	temporalScore := clamp01(0.40*firstWindowShare + 0.35*activityDensity + 0.25*gapIntensity)
	coordinationScore := clamp01(0.35*authorConcentration + 0.25*duplicateTextRatio + 0.20*repeatedURLRatio + 0.20*replyRatio)
	contentScore := clamp01(0.40*urgencyRatio + 0.30*claimRatio + 0.15*denialRatio + 0.15*lexicalRepetitionRatio)
	riskScore := clamp01(0.40*coordinationScore + 0.35*temporalScore + 0.25*contentScore)

	riskLevel := "low"
	if riskScore >= 0.70 {
		riskLevel = "high"
	} else if riskScore >= 0.40 {
		riskLevel = "medium"
	}

	evidence := buildEvidence(map[string]float64{
		"first_window_share":       firstWindowShare,
		"activity_density":         activityDensity,
		"gap_intensity":            gapIntensity,
		"author_concentration":     authorConcentration,
		"reply_ratio":              replyRatio,
		"duplicate_text_ratio":     duplicateTextRatio,
		"repeated_url_ratio":       repeatedURLRatio,
		"urgency_marker_ratio":     urgencyRatio,
		"claim_marker_ratio":       claimRatio,
		"denial_marker_ratio":      denialRatio,
		"lexical_repetition_ratio": lexicalRepetitionRatio,
	})

	temporalFeatures := map[string]interface{}{
		"first_window_share": firstWindowShare,
		"activity_density":   activityDensity,
		"gap_intensity":      gapIntensity,
		"time_span_seconds":  spanSeconds,
	}
	coordinationFeatures := map[string]interface{}{
		"author_concentration": authorConcentration,
		"reply_ratio":          replyRatio,
		"duplicate_text_ratio": duplicateTextRatio,
		"repeated_url_ratio":   repeatedURLRatio,
	}
	contentFeatures := map[string]interface{}{
		"urgency_marker_ratio":     urgencyRatio,
		"claim_marker_ratio":       claimRatio,
		"denial_marker_ratio":      denialRatio,
		"lexical_repetition_ratio": lexicalRepetitionRatio,
	}
	featurePayload := map[string]interface{}{
		"case_id":              c.ID,
		"external_case_id":     c.ExternalCaseID,
		"event_count":          eventCount,
		"unique_account_count": len(accountCounts),
		"unique_url_count":     len(urlCounts),
		"unique_hashtag_count": len(tagSet),
		"time_span_seconds":    spanSeconds,
	}

	return repository.CaseFeaturesRecord{
			CaseID:               c.ID,
			FeatureVersion:       featureVersion,
			EventCount:           eventCount,
			UniqueAccountCount:   len(accountCounts),
			UniqueURLCount:       len(urlCounts),
			UniqueHashtagCount:   len(tagSet),
			TemporalFeatures:     temporalFeatures,
			CoordinationFeatures: coordinationFeatures,
			ContentFeatures:      contentFeatures,
			FeaturePayload:       featurePayload,
		}, repository.CaseScoreRecord{
			CaseID:            c.ID,
			ScoreVersion:      scoreVersion,
			TemporalScore:     temporalScore,
			CoordinationScore: coordinationScore,
			ContentScore:      contentScore,
			RiskScore:         riskScore,
			RiskLevel:         riskLevel,
			Evidence:          evidence,
			PipelineHash:      pipelineHash,
		}
}

func buildEvidence(featureScores map[string]float64) []string {
	type pair struct {
		Name  string
		Value float64
	}
	items := make([]pair, 0, len(featureScores))
	for name, value := range featureScores {
		items = append(items, pair{Name: name, Value: value})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Value > items[j].Value
	})

	limit := 3
	if len(items) < limit {
		limit = len(items)
	}
	evidence := make([]string, 0, limit)
	for _, item := range items[:limit] {
		if item.Value < 0.20 {
			continue
		}
		evidence = append(evidence, fmt.Sprintf("%s=%.3f", item.Name, item.Value))
	}
	return evidence
}

func normalizeText(text string) string {
	text = strings.ToLower(text)
	var b strings.Builder
	b.Grow(len(text))
	lastSpace := false
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if unicode.IsSpace(r) {
			if !lastSpace {
				b.WriteRune(' ')
				lastSpace = true
			}
			continue
		}
	}
	return strings.TrimSpace(b.String())
}

func containsAny(text string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func caseModelScore(caseID int64, score repository.CaseScoreRecord) repository.CaseModelScoreRecord {
	return repository.CaseModelScoreRecord{
		CaseID:            caseID,
		ScorerKey:         getenvDefault("CASE_FEATURE_SCORER_KEY", "case_feature"),
		ModelVersion:      score.ScoreVersion,
		RiskScore:         score.RiskScore,
		RiskLevel:         score.RiskLevel,
		TemporalScore:     floatPtr(score.TemporalScore),
		CoordinationScore: floatPtr(score.CoordinationScore),
		ContentScore:      floatPtr(score.ContentScore),
		Evidence:          append([]string(nil), score.Evidence...),
		FeaturePayload: map[string]interface{}{
			"scoring_source":     "go_case_scorer",
			"active_case_score":  true,
			"confidence_present": false,
		},
		ModelInfo: map[string]interface{}{
			"type":    "go_heuristic_case_baseline",
			"trained": false,
		},
		PipelineHash:   score.PipelineHash,
		SourceEndpoint: "cmd/case_scorer",
	}
}

func floatPtr(value float64) *float64 {
	return &value
}

func computePipelineHash(featureVersion, scoreVersion string) string {
	payload := fmt.Sprintf("%s|%s|weights=coord:0.40,temp:0.35,content:0.25|temporal=0.40,0.35,0.25|coord=0.35,0.25,0.20,0.20|content=0.40,0.30,0.15,0.15", featureVersion, scoreVersion)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func zeroTimeToNow(ts *time.Time) time.Time {
	if ts == nil || ts.IsZero() {
		return time.Now().UTC()
	}
	return ts.UTC()
}

func safeRatio(numerator, denominator float64) float64 {
	if denominator <= 0 {
		return 0
	}
	return clamp01(numerator / denominator)
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func getenvDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getenvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

func getenvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "y":
		return true
	case "0", "false", "no", "n":
		return false
	default:
		return fallback
	}
}
