package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/audittrail"
	applog "github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/logger"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

type datasetRow map[string]interface{}

func main() {
	_ = godotenv.Load()
	logFile, err := applog.ConfigureStandardLog("dataset_loader")
	if err != nil {
		log.Printf("configure file logging failed: %v", err)
	}
	if logFile != nil {
		defer logFile.Close()
	}

	datasetPath := os.Getenv("DATASET_PATH")
	if datasetPath == "" {
		log.Fatal("DATASET_PATH is required (JSONL file)")
	}

	datasetName := getenvDefault("DATASET_NAME", "unknown_dataset")
	datasetSplit := getenvDefault("DATASET_SPLIT", "default")
	sourceName := strings.TrimSpace(os.Getenv("SOURCE_NAME"))
	if sourceName == "" {
		sourceName = datasetName
	}

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

	job := audittrail.NewCLIJob(db, "dataset_loader").
		WithSource("dataset", sourceName, datasetName, datasetSplit).
		WithPayload(map[string]interface{}{
			"dataset_path": datasetPath,
		})
	job.Start()
	defer job.FinishAndExit()

	sourceID, err := db.EnsureDataSource(sourceName, "")
	if err != nil {
		job.Failf("ensure data source failed: %v", err)
		return
	}

	runID, err := db.StartIngestionRun("dataset", sourceName, datasetName, datasetSplit)
	if err != nil {
		job.Failf("start ingestion run failed: %v", err)
		return
	}
	job.Set("ingestion_run_id", runID)

	processed := 0
	failed := 0
	runStatus := "completed"
	runNotes := "ok"

	defer func() {
		if failed > 0 && runStatus == "completed" {
			runNotes = "processed=" + strconv.Itoa(processed) + ", failed=" + strconv.Itoa(failed)
		}
		if err := db.FinishIngestionRun(runID, runStatus, runNotes); err != nil {
			log.Printf("finish ingestion run failed: %v", err)
		}
	}()

	f, err := os.Open(datasetPath)
	if err != nil {
		runStatus = "failed"
		runNotes = "open dataset failed: " + err.Error()
		job.Failf("%s", runNotes)
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var row datasetRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			failed++
			log.Printf("line %d: invalid json: %v", lineNo, err)
			continue
		}

		post := mapRowToDatasetPost(row, sourceID, datasetName, datasetSplit, lineNo, runID, line)
		if err := db.SaveDatasetPost(post); err != nil {
			failed++
			log.Printf("line %d: save failed: %v", lineNo, err)
			continue
		}
		processed++
	}

	if err := scanner.Err(); err != nil {
		runStatus = "failed"
		runNotes = "scan dataset failed: " + err.Error()
		job.Failf("%s", runNotes)
		return
	}

	job.Set("processed", processed)
	job.Set("failed_rows", failed)
	job.Set("run_status", runStatus)
	log.Printf("dataset ingestion completed: processed=%d failed=%d run_id=%d", processed, failed, runID)
}

func mapRowToDatasetPost(row datasetRow, sourceID int, datasetName, datasetSplit string, lineNo int, runID int64, rawLine string) repository.DatasetPost {
	now := time.Now().UTC()
	publishedAt := parseTime(lookupString(row, "published_at", "timestamp", "created_at"), now)
	externalID := lookupString(row, "id", "external_id", "post_id")
	if externalID == "" {
		externalID = "dataset-row-" + strconv.Itoa(lineNo)
	}

	username := lookupString(row, "username", "author", "user")
	content := lookupString(row, "content", "text", "body")

	hashBytes := sha256.Sum256([]byte(rawLine))
	hash := hex.EncodeToString(hashBytes[:])

	return repository.DatasetPost{
		SourceID:        sourceID,
		ExternalID:      externalID,
		Username:        username,
		DisplayName:     lookupString(row, "display_name", "author_name"),
		Content:         content,
		Language:        lookupString(row, "lang", "language"),
		PublishedAt:     publishedAt,
		PostURL:         lookupString(row, "url", "post_url"),
		LikesCount:      lookupInt(row, "likes_count", "likes"),
		RepostsCount:    lookupInt(row, "reposts_count", "reposts", "retweets"),
		RepliesCount:    lookupInt(row, "replies_count", "replies"),
		RawPayloadRef:   datasetName + ":" + datasetSplit + ":line=" + strconv.Itoa(lineNo),
		RawPayloadHash:  hash,
		DatasetName:     datasetName,
		DatasetSplit:    datasetSplit,
		DatasetRecordID: externalID,
		IngestionRunID:  &runID,
	}
}

func lookupString(row datasetRow, keys ...string) string {
	for _, k := range keys {
		if v, ok := row[k]; ok && v != nil {
			switch t := v.(type) {
			case string:
				if strings.TrimSpace(t) != "" {
					return t
				}
			case float64:
				return strconv.FormatFloat(t, 'f', -1, 64)
			}
		}
	}
	return ""
}

func lookupInt(row datasetRow, keys ...string) int {
	for _, k := range keys {
		if v, ok := row[k]; ok && v != nil {
			switch t := v.(type) {
			case float64:
				return int(t)
			case int:
				return t
			case string:
				n, err := strconv.Atoi(t)
				if err == nil {
					return n
				}
			}
		}
	}
	return 0
}

func parseTime(raw string, fallback time.Time) time.Time {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		t, err := time.Parse(layout, raw)
		if err == nil {
			return t.UTC()
		}
	}
	return fallback
}

func getenvDefault(key, fallback string) string {
	v := os.Getenv(key)
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
