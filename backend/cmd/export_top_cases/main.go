package main

import (
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

type exportPayload struct {
	GeneratedAt  time.Time                   `json:"generated_at"`
	SourceName   string                      `json:"source_name"`
	DatasetName  string                      `json:"dataset_name"`
	DatasetSplit string                      `json:"dataset_split"`
	Limit        int                         `json:"limit"`
	MinRiskScore float64                     `json:"min_risk_score"`
	Count        int                         `json:"count"`
	Items        []repository.ScoredCaseItem `json:"items"`
}

func main() {
	_ = godotenv.Load()
	logFile, err := applog.ConfigureStandardLog("export_top_cases")
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

	outPath := strings.TrimSpace(os.Getenv("OUTPUT_PATH"))
	if outPath == "" {
		log.Fatal("OUTPUT_PATH is required")
	}

	filter := repository.CaseFilter{
		SourceName:   getenvDefault("SOURCE_NAME", ""),
		DatasetName:  getenvDefault("DATASET_NAME", ""),
		DatasetSplit: getenvDefault("DATASET_SPLIT", ""),
		Label:        getenvDefault("CASE_LABEL", ""),
		OnlyUnscored: false,
		Limit:        getenvInt("EXPORT_LIMIT", 25),
	}
	minRiskScore := getenvFloat("MIN_RISK_SCORE", 0.0)

	db, err := repository.NewPostgresDB(connStr)
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}
	defer db.Close()

	job := audittrail.NewCLIJob(db, "export_top_cases").
		WithSource("dataset", filter.SourceName, filter.DatasetName, filter.DatasetSplit).
		WithPayload(map[string]interface{}{
			"export_limit":   filter.Limit,
			"case_label":     filter.Label,
			"min_risk_score": minRiskScore,
			"output_path":    outPath,
		})
	job.Start()
	defer job.FinishAndExit()

	items, err := db.ListScoredCases(filter)
	if err != nil {
		job.Failf("load scored cases failed: %v", err)
		return
	}
	job.Set("selected_cases", len(items))

	filtered := make([]repository.ScoredCaseItem, 0, len(items))
	for _, item := range items {
		if item.RiskScore < minRiskScore {
			continue
		}
		filtered = append(filtered, item)
	}

	payload := exportPayload{
		GeneratedAt:  time.Now().UTC(),
		SourceName:   filter.SourceName,
		DatasetName:  filter.DatasetName,
		DatasetSplit: filter.DatasetSplit,
		Limit:        filter.Limit,
		MinRiskScore: minRiskScore,
		Count:        len(filtered),
		Items:        filtered,
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		job.Failf("marshal export payload failed: %v", err)
		return
	}

	parent := filepathDir(outPath)
	if parent != "" && parent != "." {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			job.Failf("create output directory failed: %v", err)
			return
		}
	}
	if err := os.WriteFile(outPath, append(data, '\n'), 0o644); err != nil {
		job.Failf("write export file failed: %v", err)
		return
	}

	job.Set("exported_cases", len(filtered))
	log.Printf("exported top cases: count=%d path=%s", len(filtered), outPath)
}

func filepathDir(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return "."
	}
	if idx == 0 {
		return "/"
	}
	return path[:idx]
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

func getenvFloat(key string, fallback float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return n
}
