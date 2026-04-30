package main

import (
	"encoding/json"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/audittrail"
	applog "github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/logger"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

type evaluatedCase struct {
	ID         int64   `json:"id"`
	Label      string  `json:"label"`
	IsPositive bool    `json:"is_positive"`
	RiskScore  float64 `json:"risk_score"`
	RiskLevel  string  `json:"risk_level"`
}

type evaluationSummary struct {
	EvaluationRunID int64           `json:"evaluation_run_id,omitempty"`
	SourceName      string          `json:"source_name"`
	DatasetName     string          `json:"dataset_name"`
	DatasetSplit    string          `json:"dataset_split"`
	ScorerVersion   string          `json:"scorer_version"`
	PipelineHash    string          `json:"pipeline_hash"`
	Threshold       float64         `json:"threshold"`
	PositiveLabels  []string        `json:"positive_labels"`
	ScoredCases     int             `json:"scored_cases"`
	LabeledCases    int             `json:"labeled_cases"`
	PositiveCases   int             `json:"positive_cases"`
	NegativeCases   int             `json:"negative_cases"`
	PrecisionAt10   *float64        `json:"precision_at_10,omitempty"`
	PrecisionAt20   *float64        `json:"precision_at_20,omitempty"`
	Precision       *float64        `json:"precision,omitempty"`
	Recall          *float64        `json:"recall,omitempty"`
	F1              *float64        `json:"f1,omitempty"`
	ROCAUC          *float64        `json:"roc_auc,omitempty"`
	PRAUC           *float64        `json:"pr_auc,omitempty"`
	Confusion       map[string]int  `json:"confusion"`
	TopCases        []evaluatedCase `json:"top_cases"`
	GeneratedAt     time.Time       `json:"generated_at"`
}

func main() {
	_ = godotenv.Load()
	logFile, err := applog.ConfigureStandardLog("evaluate_cases")
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

	filter := repository.CaseFilter{
		SourceName:   getenvDefault("SOURCE_NAME", ""),
		DatasetName:  getenvDefault("DATASET_NAME", ""),
		DatasetSplit: getenvDefault("DATASET_SPLIT", ""),
		Label:        getenvDefault("CASE_LABEL", ""),
		OnlyUnscored: false,
		Limit:        getenvInt("EVAL_LIMIT", 10000),
	}
	positiveLabels := parseLabelSet(getenvDefault("POSITIVE_LABELS", "rumour,rumor"))
	threshold := getenvFloat("RISK_THRESHOLD", 0.50)
	saveRun := getenvBool("SAVE_EVALUATION_RUN", true)
	outPath := strings.TrimSpace(os.Getenv("OUTPUT_PATH"))

	db, err := repository.NewPostgresDB(connStr)
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}
	defer db.Close()

	job := audittrail.NewCLIJob(db, "evaluate_cases").
		WithSource("dataset", filter.SourceName, filter.DatasetName, filter.DatasetSplit).
		WithPayload(map[string]interface{}{
			"eval_limit":          filter.Limit,
			"case_label":          filter.Label,
			"positive_labels":     sortedKeys(positiveLabels),
			"risk_threshold":      threshold,
			"save_evaluation_run": saveRun,
			"output_path":         outPath,
		})
	job.Start()
	defer job.FinishAndExit()

	cases, err := db.ListScoredCases(filter)
	if err != nil {
		job.Failf("load scored cases failed: %v", err)
		return
	}
	if len(cases) == 0 {
		job.Skipf("no scored cases found for evaluation")
		return
	}
	job.Set("scored_cases", len(cases))

	labeled := make([]repository.ScoredCaseItem, 0, len(cases))
	for _, item := range cases {
		if strings.TrimSpace(item.Label) == "" {
			continue
		}
		labeled = append(labeled, item)
	}
	if len(labeled) == 0 {
		job.Skipf("no labeled scored cases found for evaluation")
		return
	}
	job.Set("labeled_cases", len(labeled))

	sort.SliceStable(labeled, func(i, j int) bool {
		if labeled[i].RiskScore == labeled[j].RiskScore {
			return labeled[i].ID < labeled[j].ID
		}
		return labeled[i].RiskScore > labeled[j].RiskScore
	})

	positives := 0
	negatives := 0
	for _, item := range labeled {
		if isPositiveLabel(item.Label, positiveLabels) {
			positives++
		} else {
			negatives++
		}
	}

	precisionAt10 := precisionAtK(labeled, positiveLabels, 10)
	precisionAt20 := precisionAtK(labeled, positiveLabels, 20)
	precision, recall, f1, tp, fp, tn, fn := thresholdMetrics(labeled, positiveLabels, threshold)
	rocAUC := computeROCAUC(labeled, positiveLabels)
	prAUC := computePRAUC(labeled, positiveLabels)

	topCases := make([]evaluatedCase, 0, min(len(labeled), 10))
	for _, item := range labeled[:min(len(labeled), 10)] {
		topCases = append(topCases, evaluatedCase{
			ID:         item.ID,
			Label:      item.Label,
			IsPositive: isPositiveLabel(item.Label, positiveLabels),
			RiskScore:  item.RiskScore,
			RiskLevel:  item.RiskLevel,
		})
	}

	scorerVersion := commonOrMixedScoreVersion(labeled)
	pipelineHash := commonOrMixedPipelineHash(labeled)
	summary := evaluationSummary{
		SourceName:     filter.SourceName,
		DatasetName:    filter.DatasetName,
		DatasetSplit:   filter.DatasetSplit,
		ScorerVersion:  scorerVersion,
		PipelineHash:   pipelineHash,
		Threshold:      threshold,
		PositiveLabels: sortedKeys(positiveLabels),
		ScoredCases:    len(cases),
		LabeledCases:   len(labeled),
		PositiveCases:  positives,
		NegativeCases:  negatives,
		PrecisionAt10:  precisionAt10,
		PrecisionAt20:  precisionAt20,
		Precision:      precision,
		Recall:         recall,
		F1:             f1,
		ROCAUC:         rocAUC,
		PRAUC:          prAUC,
		Confusion: map[string]int{
			"tp": tp,
			"fp": fp,
			"tn": tn,
			"fn": fn,
		},
		TopCases:    topCases,
		GeneratedAt: time.Now().UTC(),
	}

	if saveRun {
		runID, err := db.SaveEvaluationRun(repository.EvaluationRunRecord{
			SourceName:    filter.SourceName,
			DatasetName:   filter.DatasetName,
			DatasetSplit:  filter.DatasetSplit,
			PipelineHash:  pipelineHash,
			ScorerVersion: scorerVersion,
			CaseCount:     len(labeled),
			PrecisionAt10: precisionAt10,
			PrecisionAt20: precisionAt20,
			Recall:        recall,
			F1:            f1,
			ROCAUC:        rocAUC,
			PRAUC:         prAUC,
			Metrics: map[string]interface{}{
				"threshold":       threshold,
				"positive_labels": sortedKeys(positiveLabels),
				"scored_cases":    len(cases),
				"labeled_cases":   len(labeled),
				"positive_cases":  positives,
				"negative_cases":  negatives,
				"tp":              tp,
				"fp":              fp,
				"tn":              tn,
				"fn":              fn,
			},
			Notes: getenvDefault("EVALUATION_NOTES", "case-level evaluation"),
		})
		if err != nil {
			job.Failf("save evaluation run failed: %v", err)
			return
		}
		summary.EvaluationRunID = runID
		job.Set("evaluation_run_id", runID)
	}

	if err := writeSummary(summary, outPath); err != nil {
		job.Failf("write summary failed: %v", err)
		return
	}
	job.Set("positive_cases", positives)
	job.Set("negative_cases", negatives)
	job.Set("roc_auc", rocAUC)
	job.Set("pr_auc", prAUC)
}

func writeSummary(summary evaluationSummary, outPath string) error {
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}

	if outPath == "" {
		os.Stdout.Write(data)
		os.Stdout.Write([]byte("\n"))
		return nil
	}

	dir := strings.TrimSpace(outPath)
	if parent := strings.TrimSpace(filepathDir(dir)); parent != "" && parent != "." {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(outPath, append(data, '\n'), 0o644)
}

func precisionAtK(items []repository.ScoredCaseItem, positiveLabels map[string]struct{}, k int) *float64 {
	if len(items) == 0 || k <= 0 {
		return nil
	}
	topK := min(len(items), k)
	positives := 0
	for _, item := range items[:topK] {
		if isPositiveLabel(item.Label, positiveLabels) {
			positives++
		}
	}
	value := float64(positives) / float64(topK)
	return &value
}

func thresholdMetrics(items []repository.ScoredCaseItem, positiveLabels map[string]struct{}, threshold float64) (*float64, *float64, *float64, int, int, int, int) {
	tp := 0
	fp := 0
	tn := 0
	fn := 0
	positiveTotal := 0

	for _, item := range items {
		isPositive := isPositiveLabel(item.Label, positiveLabels)
		if isPositive {
			positiveTotal++
		}
		predictedPositive := item.RiskScore >= threshold
		switch {
		case isPositive && predictedPositive:
			tp++
		case !isPositive && predictedPositive:
			fp++
		case isPositive && !predictedPositive:
			fn++
		default:
			tn++
		}
	}

	var precisionPtr *float64
	if tp+fp > 0 {
		value := float64(tp) / float64(tp+fp)
		precisionPtr = &value
	} else if positiveTotal > 0 {
		value := 0.0
		precisionPtr = &value
	}

	var recallPtr *float64
	if positiveTotal > 0 {
		value := float64(tp) / float64(positiveTotal)
		recallPtr = &value
	}

	var f1Ptr *float64
	if precisionPtr != nil && recallPtr != nil {
		denom := *precisionPtr + *recallPtr
		if denom > 0 {
			value := 2.0 * (*precisionPtr) * (*recallPtr) / denom
			f1Ptr = &value
		} else {
			value := 0.0
			f1Ptr = &value
		}
	}

	return precisionPtr, recallPtr, f1Ptr, tp, fp, tn, fn
}

func computeROCAUC(items []repository.ScoredCaseItem, positiveLabels map[string]struct{}) *float64 {
	if len(items) == 0 {
		return nil
	}

	type rankedItem struct {
		score      float64
		isPositive bool
	}
	ranked := make([]rankedItem, 0, len(items))
	positives := 0
	negatives := 0
	for _, item := range items {
		isPositive := isPositiveLabel(item.Label, positiveLabels)
		if isPositive {
			positives++
		} else {
			negatives++
		}
		ranked = append(ranked, rankedItem{score: item.RiskScore, isPositive: isPositive})
	}
	if positives == 0 || negatives == 0 {
		return nil
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			if ranked[i].isPositive == ranked[j].isPositive {
				return false
			}
			return !ranked[i].isPositive && ranked[j].isPositive
		}
		return ranked[i].score < ranked[j].score
	})

	sumPositiveRanks := 0.0
	for i := 0; i < len(ranked); {
		j := i + 1
		for j < len(ranked) && ranked[j].score == ranked[i].score {
			j++
		}
		avgRank := float64(i+j+1) / 2.0
		for _, item := range ranked[i:j] {
			if item.isPositive {
				sumPositiveRanks += avgRank
			}
		}
		i = j
	}

	value := (sumPositiveRanks - float64(positives*(positives+1))/2.0) / float64(positives*negatives)
	return &value
}

func computePRAUC(items []repository.ScoredCaseItem, positiveLabels map[string]struct{}) *float64 {
	if len(items) == 0 {
		return nil
	}
	positiveTotal := 0
	for _, item := range items {
		if isPositiveLabel(item.Label, positiveLabels) {
			positiveTotal++
		}
	}
	if positiveTotal == 0 {
		return nil
	}

	sortedItems := make([]repository.ScoredCaseItem, len(items))
	copy(sortedItems, items)
	sort.SliceStable(sortedItems, func(i, j int) bool {
		if sortedItems[i].RiskScore == sortedItems[j].RiskScore {
			return sortedItems[i].ID < sortedItems[j].ID
		}
		return sortedItems[i].RiskScore > sortedItems[j].RiskScore
	})

	tp := 0
	fp := 0
	prevRecall := 0.0
	area := 0.0

	for _, item := range sortedItems {
		if isPositiveLabel(item.Label, positiveLabels) {
			tp++
		} else {
			fp++
		}
		recall := float64(tp) / float64(positiveTotal)
		precision := float64(tp) / float64(tp+fp)
		area += (recall - prevRecall) * precision
		prevRecall = recall
	}

	return &area
}

func parseLabelSet(raw string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, part := range strings.Split(raw, ",") {
		normalized := normalizeLabel(part)
		if normalized != "" {
			result[normalized] = struct{}{}
		}
	}
	return result
}

func isPositiveLabel(label string, positiveLabels map[string]struct{}) bool {
	_, ok := positiveLabels[normalizeLabel(label)]
	return ok
}

func normalizeLabel(raw string) string {
	normalized := strings.TrimSpace(strings.ToLower(raw))
	replacer := strings.NewReplacer("-", "_", " ", "_")
	return replacer.Replace(normalized)
}

func commonOrMixedScoreVersion(items []repository.ScoredCaseItem) string {
	if len(items) == 0 {
		return ""
	}
	value := items[0].ScoreVersion
	for _, item := range items[1:] {
		if item.ScoreVersion != value {
			return "mixed"
		}
	}
	return value
}

func commonOrMixedPipelineHash(items []repository.ScoredCaseItem) string {
	if len(items) == 0 {
		return ""
	}
	value := items[0].PipelineHash
	for _, item := range items[1:] {
		if item.PipelineHash != value {
			return "mixed"
		}
	}
	return value
}

func sortedKeys(values map[string]struct{}) []string {
	items := make([]string, 0, len(values))
	for key := range values {
		items = append(items, key)
	}
	sort.Strings(items)
	return items
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
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
