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

	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = os.Getenv("DATABASE_URI")
	}
	if connStr == "" {
		log.Fatal("DATABASE_URL or DATABASE_URI is required")
	}

	db, err := repository.NewPostgresDB(connStr)
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}
	defer db.Close()

	filter := repository.AnalysisPostFilter{
		SourceType:     getenvDefault("SOURCE_TYPE", "dataset"),
		DatasetName:    os.Getenv("DATASET_NAME"),
		DatasetSplit:   os.Getenv("DATASET_SPLIT"),
		OnlyUnanalyzed: getenvBool("ONLY_UNANALYZED", true),
		Limit:          getenvInt("BATCH_LIMIT", 100),
	}

	if rawRunID := strings.TrimSpace(os.Getenv("INGESTION_RUN_ID")); rawRunID != "" {
		runID, err := strconv.ParseInt(rawRunID, 10, 64)
		if err != nil || runID <= 0 {
			log.Fatalf("invalid INGESTION_RUN_ID: %q", rawRunID)
		}
		filter.IngestionRunID = &runID
	}

	posts, err := db.GetPostsForAnalysis(filter)
	if err != nil {
		log.Fatalf("get posts for analysis failed: %v", err)
	}
	if len(posts) == 0 {
		log.Println("no posts found for analysis")
		return
	}

	mlURL := getenvDefault("ML_SERVICE_URL", "http://localhost:8000")
	mlClient := collector.NewMLClient(mlURL)

	processed := 0
	failed := 0
	high := 0
	medium := 0
	low := 0
	started := time.Now()

	log.Printf("dataset analyzer started: posts=%d source_type=%s dataset=%s split=%s ml=%s",
		len(posts), filter.SourceType, filter.DatasetName, filter.DatasetSplit, mlURL)

	for _, post := range posts {
		cleanText := strings.TrimSpace(stripHTMLTags(post.Content))
		if cleanText == "" {
			failed++
			log.Printf("post #%d skipped: empty content", post.ID)
			continue
		}

		mlResp, err := mlClient.AnalyzeText(
			post.ID,
			cleanText,
			post.AccountID,
			post.Username,
			post.PublishedAt,
			post.FollowersCount,
			post.AccountCreatedAt,
		)
		if err != nil {
			failed++
			log.Printf("post #%d analysis failed: %v", post.ID, err)
			continue
		}

		priority := collector.GetEscalationPriority(
			mlResp.ManipulationScore,
			mlResp.ConfidenceScore,
		)

		repoResp := &repository.MLResponse{
			ManipulationScore:        mlResp.ManipulationScore,
			ConfidenceScore:          mlResp.ConfidenceScore,
			CoordinationContribution: mlResp.CoordinationContribution,
			TemporalContribution:     mlResp.TemporalContribution,
			NarrativeContribution:    mlResp.NarrativeContribution,
			ConfidenceNote:           mlResp.ConfidenceNote,
			KeyEvidence:              mlResp.KeyEvidence,
			Tactics:                  mlResp.Tactics,
		}

		if err := db.SaveAnalysisResult(post.ID, repoResp, priority); err != nil {
			failed++
			log.Printf("post #%d save failed: %v", post.ID, err)
			continue
		}

		processed++
		switch priority {
		case 1:
			high++
		case 2:
			medium++
		default:
			low++
		}

		log.Printf("post #%d analyzed: score=%.3f confidence=%.3f priority=%d",
			post.ID, mlResp.ManipulationScore, mlResp.ConfidenceScore, priority)
	}

	summary, err := db.GetAnalysisSummary(filter)
	if err != nil {
		log.Printf("analysis summary failed: %v", err)
	}

	log.Printf(
		"dataset analyzer finished: processed=%d failed=%d high=%d medium=%d low=%d duration=%s total_analyzed=%d avg_score=%.3f",
		processed,
		failed,
		high,
		medium,
		low,
		time.Since(started).Round(time.Millisecond),
		summary.TotalAnalyzed,
		summary.AverageScore,
	)
}

func stripHTMLTags(text string) string {
	var b strings.Builder
	inTag := false
	for _, ch := range text {
		switch ch {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				b.WriteRune(ch)
			}
		}
	}
	return b.String()
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
