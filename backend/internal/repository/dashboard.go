package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ManipulationStats aggregates post-level analysis results for the dashboard.
type ManipulationStats struct {
	AvgManipulationScore  float64 `json:"avg_manipulation_score"`
	AvgCoordination       float64 `json:"avg_coordination"`
	AvgTemporal           float64 `json:"avg_temporal"`
	AvgNarrative          float64 `json:"avg_narrative"`
	AvgConfidence         float64 `json:"avg_confidence"`
	TotalAnalyzed         int     `json:"total_analyzed"`
	HighManipulationCount int     `json:"high_manipulation_count"`
}

type BranchTimeSeriesPoint struct {
	Date         string  `json:"date"`
	Coordination float64 `json:"coordination"`
	Temporal     float64 `json:"temporal"`
	Narrative    float64 `json:"narrative"`
	Count        int     `json:"count"`
}

type ManipulationTrendPoint struct {
	Date         string  `json:"date"`
	AvgScore     float64 `json:"avg_score"`
	Coordination float64 `json:"coordination"`
	Temporal     float64 `json:"temporal"`
	Narrative    float64 `json:"narrative"`
	CasesCount   int     `json:"cases_count"`
}

type RadarDataPoint struct {
	Branch string  `json:"branch"`
	Value  float64 `json:"value"`
}

type EvidenceItem struct {
	CaseID            int64     `json:"case_id"`
	CaseTitle         string    `json:"case_title"`
	ManipulationScore float64   `json:"manipulation_score"`
	Coordination      float64   `json:"coordination"`
	Temporal          float64   `json:"temporal"`
	Narrative         float64   `json:"narrative"`
	Summary           string    `json:"summary"`
	CreatedAt         time.Time `json:"created_at"`
}

type TacticStats struct {
	TacticName    string  `json:"tactic_name"`
	Count         int     `json:"count"`
	AvgConfidence float64 `json:"avg_confidence"`
	AvgScore      float64 `json:"avg_score"`
}

// ExtendedDashboardMetrics is the full dashboard payload used by the frontend.
type ExtendedDashboardMetrics struct {
	Summary          DashboardSummary              `json:"summary"`
	Engagement       DashboardEngagement           `json:"engagement"`
	CasesOverTime    []DashboardCasesOverTimePoint `json:"cases_over_time"`
	RiskDistribution struct {
		High   int `json:"high"`
		Medium int `json:"medium"`
		Low    int `json:"low"`
	} `json:"risk_distribution"`
	TopEvents         []DashboardTopEvent          `json:"top_events"`
	ScorerPerformance []DashboardScorerPerformance `json:"scorer_performance"`
	TimelineData      []DashboardTimelinePoint     `json:"timeline_data"`
	StatusBreakdown   []DashboardBreakdownItem     `json:"status_breakdown"`
	LabelBreakdown    []DashboardBreakdownItem     `json:"label_breakdown"`
	SourceBreakdown   []DashboardBreakdownItem     `json:"source_breakdown"`
	DatasetBreakdown  []DashboardBreakdownItem     `json:"dataset_breakdown"`
	TopRiskCases      []DashboardTopRiskCase       `json:"top_risk_cases"`

	ManipulationStats *ManipulationStats       `json:"manipulation_stats"`
	TopTactics        []TacticStats            `json:"top_tactics"`
	RecentEvidence    []EvidenceItem           `json:"recent_evidence"`
	BranchTimeSeries  []BranchTimeSeriesPoint  `json:"branch_time_series"`
	ManipulationTrend []ManipulationTrendPoint `json:"manipulation_trend"`
}

// GetExtendedDashboardMetrics builds dashboard metrics from the actual database schema.
func (p *PostgresDB) GetExtendedDashboardMetrics(filter DashboardFilter) (ExtendedDashboardMetrics, error) {
	whereSQL, args := dashboardWhere(filter)
	scorerKey := normalizeScorerKey(filter.ScorerKey)
	useModelScore := useCaseModelScore(scorerKey)

	scoreJoin := "LEFT JOIN case_scores cs ON cs.case_id = c.id"
	scoreExpr := "cs.risk_score"
	riskExpr := "COALESCE(cs.risk_level, '')"
	argsWithScore := append([]interface{}{}, args...)
	if useModelScore {
		argsWithScore = append(argsWithScore, scorerKey)
		scoreJoin = fmt.Sprintf("LEFT JOIN case_model_scores cms ON cms.case_id = c.id AND cms.scorer_key = $%d", len(argsWithScore))
		scoreExpr = "cms.risk_score"
		riskExpr = "COALESCE(cms.risk_level, '')"
	}
	if meaningfulDashboardFilter(filter.RiskLevel) {
		argsWithScore = append(argsWithScore, filter.RiskLevel)
		whereSQL = fmt.Sprintf("%s AND %s = $%d", whereSQL, riskExpr, len(argsWithScore))
	}

	baseSQL := fmt.Sprintf("FROM cases c %s WHERE %s", scoreJoin, whereSQL)

	var metrics ExtendedDashboardMetrics
	if err := p.loadDashboardSummary(&metrics, baseSQL, scoreExpr, argsWithScore); err != nil {
		return metrics, err
	}
	if err := p.loadDashboardRiskDistribution(&metrics, baseSQL, riskExpr, argsWithScore); err != nil {
		return metrics, err
	}
	if err := p.loadDashboardEngagement(&metrics, baseSQL, argsWithScore); err != nil {
		return metrics, err
	}
	if err := p.loadDashboardCasesOverTime(&metrics, baseSQL, riskExpr, argsWithScore); err != nil {
		return metrics, err
	}
	if err := p.loadDashboardTopEvents(&metrics, baseSQL, scoreExpr, argsWithScore); err != nil {
		return metrics, err
	}
	if err := p.loadDashboardBreakdowns(&metrics, baseSQL, scoreExpr, argsWithScore); err != nil {
		return metrics, err
	}
	if err := p.loadDashboardTopRiskCases(&metrics, baseSQL, scoreExpr, riskExpr, argsWithScore); err != nil {
		return metrics, err
	}
	if err := p.loadDashboardScorerPerformance(&metrics, whereSQL, args); err != nil {
		return metrics, err
	}
	// Post-level manipulation branches/evidence are intentionally not loaded in the main
	// dashboard payload now: the frontend does not render these panels, and keeping
	// dashboard metrics case-level avoids expensive joins and placeholder conflicts.

	if metrics.Summary.TotalCases > 0 {
		metrics.Summary.HighRiskShare = float64(metrics.RiskDistribution.High) / float64(metrics.Summary.TotalCases)
	}
	normalizeDashboardSlices(&metrics)
	return metrics, nil
}

func normalizeDashboardSlices(metrics *ExtendedDashboardMetrics) {
	if metrics.CasesOverTime == nil {
		metrics.CasesOverTime = []DashboardCasesOverTimePoint{}
	}
	if metrics.TopEvents == nil {
		metrics.TopEvents = []DashboardTopEvent{}
	}
	if metrics.ScorerPerformance == nil {
		metrics.ScorerPerformance = []DashboardScorerPerformance{}
	}
	if metrics.TimelineData == nil {
		metrics.TimelineData = []DashboardTimelinePoint{}
	}
	if metrics.StatusBreakdown == nil {
		metrics.StatusBreakdown = []DashboardBreakdownItem{}
	}
	if metrics.LabelBreakdown == nil {
		metrics.LabelBreakdown = []DashboardBreakdownItem{}
	}
	if metrics.SourceBreakdown == nil {
		metrics.SourceBreakdown = []DashboardBreakdownItem{}
	}
	if metrics.DatasetBreakdown == nil {
		metrics.DatasetBreakdown = []DashboardBreakdownItem{}
	}
	if metrics.TopRiskCases == nil {
		metrics.TopRiskCases = []DashboardTopRiskCase{}
	}
	if metrics.TopTactics == nil {
		metrics.TopTactics = []TacticStats{}
	}
	if metrics.RecentEvidence == nil {
		metrics.RecentEvidence = []EvidenceItem{}
	}
	if metrics.BranchTimeSeries == nil {
		metrics.BranchTimeSeries = []BranchTimeSeriesPoint{}
	}
	if metrics.ManipulationTrend == nil {
		metrics.ManipulationTrend = []ManipulationTrendPoint{}
	}
}

func (p *PostgresDB) loadDashboardSummary(metrics *ExtendedDashboardMetrics, baseSQL, scoreExpr string, args []interface{}) error {
	query := fmt.Sprintf(`
		WITH matched_cases AS (
			SELECT c.id, c.status, c.label, %s AS risk_score %s
		), post_counts AS (
			SELECT COUNT(p.id) AS total_posts
			FROM posts p
			JOIN matched_cases mc ON mc.id = p.case_id
		)
		SELECT
			COUNT(*) AS total_cases,
			COALESCE(AVG(risk_score), 0) AS mean_risk,
			COALESCE((SELECT total_posts FROM post_counts), 0) AS total_posts,
			COALESCE((SELECT total_posts FROM post_counts)::float / NULLIF(COUNT(*), 0), 0) AS avg_posts_per_case,
			COUNT(*) FILTER (WHERE status = 'open') AS open_cases,
			COUNT(*) FILTER (WHERE status = 'closed') AS closed_cases,
			COUNT(*) FILTER (WHERE status = 'finalized') AS finalized_cases,
			COUNT(*) FILTER (WHERE label IN ('rumour', 'rumor')) AS rumour_cases,
			COUNT(*) FILTER (WHERE label IN ('non_rumour', 'non-rumour')) AS non_rumour_cases
		FROM matched_cases
	`, scoreExpr, baseSQL)
	return p.db.QueryRow(query, args...).Scan(
		&metrics.Summary.TotalCases,
		&metrics.Summary.MeanRisk,
		&metrics.Summary.TotalPosts,
		&metrics.Summary.AvgPostsPerCase,
		&metrics.Summary.OpenCases,
		&metrics.Summary.ClosedCases,
		&metrics.Summary.FinalizedCases,
		&metrics.Summary.RumourCases,
		&metrics.Summary.NonRumourCases,
	)
}

func (p *PostgresDB) loadDashboardRiskDistribution(metrics *ExtendedDashboardMetrics, baseSQL, riskExpr string, args []interface{}) error {
	query := fmt.Sprintf(`
		SELECT
			COUNT(*) FILTER (WHERE %s = 'high') AS high_risk,
			COUNT(*) FILTER (WHERE %s = 'medium') AS medium_risk,
			COUNT(*) FILTER (WHERE %s = 'low') AS low_risk
		%s
	`, riskExpr, riskExpr, riskExpr, baseSQL)
	return p.db.QueryRow(query, args...).Scan(
		&metrics.RiskDistribution.High,
		&metrics.RiskDistribution.Medium,
		&metrics.RiskDistribution.Low,
	)
}

func (p *PostgresDB) loadDashboardEngagement(metrics *ExtendedDashboardMetrics, baseSQL string, args []interface{}) error {
	query := fmt.Sprintf(`
		WITH matched_cases AS (SELECT DISTINCT c.id %s)
		SELECT
			COUNT(p.id) AS post_count,
			COALESCE(SUM(p.likes_count), 0) AS total_likes,
			COALESCE(SUM(p.reposts_count), 0) AS total_reposts,
			COALESCE(SUM(p.replies_count), 0) AS total_replies,
			COALESCE(SUM(p.likes_count + p.reposts_count + p.replies_count), 0) AS total_engagement,
			COUNT(DISTINCT p.account_id) AS unique_accounts,
			COUNT(DISTINCT p.account_id) FILTER (WHERE COALESCE(a.is_verified, FALSE)) AS verified_accounts
		FROM matched_cases mc
		LEFT JOIN posts p ON p.case_id = mc.id
		LEFT JOIN accounts a ON a.id = p.account_id
	`, baseSQL)
	return p.db.QueryRow(query, args...).Scan(
		&metrics.Engagement.PostCount,
		&metrics.Engagement.TotalLikes,
		&metrics.Engagement.TotalReposts,
		&metrics.Engagement.TotalReplies,
		&metrics.Engagement.TotalEngagement,
		&metrics.Engagement.UniqueAccounts,
		&metrics.Engagement.VerifiedAccounts,
	)
}

func (p *PostgresDB) loadDashboardCasesOverTime(metrics *ExtendedDashboardMetrics, baseSQL, riskExpr string, args []interface{}) error {
	query := fmt.Sprintf(`
		SELECT
			DATE(COALESCE(c.first_event_at, c.opened_at, c.created_at)) AS day,
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE %s = 'high') AS high_risk,
			COUNT(*) FILTER (WHERE %s = 'medium') AS medium_risk,
			COUNT(*) FILTER (WHERE %s = 'low') AS low_risk
		%s
		GROUP BY day
		ORDER BY day ASC
	`, riskExpr, riskExpr, riskExpr, baseSQL)
	rows, err := p.db.Query(query, args...)
	if err != nil {
		return fmt.Errorf("dashboard cases over time: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var point DashboardCasesOverTimePoint
		var day time.Time
		if err := rows.Scan(&day, &point.Total, &point.HighRisk, &point.MediumRisk, &point.LowRisk); err != nil {
			return err
		}
		point.Date = day.Format("2006-01-02")
		metrics.CasesOverTime = append(metrics.CasesOverTime, point)
		metrics.TimelineData = append(metrics.TimelineData, DashboardTimelinePoint{Date: point.Date, EventCount: point.Total})
	}
	return rows.Err()
}

func (p *PostgresDB) loadDashboardTopEvents(metrics *ExtendedDashboardMetrics, baseSQL, scoreExpr string, args []interface{}) error {
	query := fmt.Sprintf(`
		SELECT COALESCE(NULLIF(c.event_name, ''), 'unknown') AS event_name,
		       COUNT(*) AS case_count,
		       COALESCE(AVG(%s), 0) AS avg_risk
		%s
		GROUP BY COALESCE(NULLIF(c.event_name, ''), 'unknown')
		ORDER BY case_count DESC, avg_risk DESC
		LIMIT 12
	`, scoreExpr, baseSQL)
	rows, err := p.db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var event DashboardTopEvent
		if err := rows.Scan(&event.EventName, &event.CaseCount, &event.AvgRisk); err != nil {
			return err
		}
		metrics.TopEvents = append(metrics.TopEvents, event)
	}
	return rows.Err()
}

func (p *PostgresDB) loadDashboardBreakdowns(metrics *ExtendedDashboardMetrics, baseSQL, scoreExpr string, args []interface{}) error {
	items := []struct {
		target *[]DashboardBreakdownItem
		expr   string
	}{
		{&metrics.StatusBreakdown, "COALESCE(NULLIF(c.status, ''), 'unknown')"},
		{&metrics.LabelBreakdown, "COALESCE(NULLIF(c.label, ''), 'unknown')"},
		{&metrics.SourceBreakdown, "COALESCE(NULLIF(c.source_name, ''), 'unknown')"},
		{&metrics.DatasetBreakdown, "COALESCE(NULLIF(c.dataset_name || '/' || c.dataset_split, '/'), 'unknown')"},
	}
	for _, item := range items {
		query := fmt.Sprintf(`
			SELECT %s AS name, COUNT(*) AS case_count, COALESCE(AVG(%s), 0) AS avg_risk
			%s
			GROUP BY %s
			ORDER BY case_count DESC, avg_risk DESC
			LIMIT 12
		`, item.expr, scoreExpr, baseSQL, item.expr)
		rows, err := p.db.Query(query, args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var row DashboardBreakdownItem
			if err := rows.Scan(&row.Name, &row.CaseCount, &row.AvgRisk); err != nil {
				_ = rows.Close()
				return err
			}
			*item.target = append(*item.target, row)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
	}
	return nil
}

func (p *PostgresDB) loadDashboardTopRiskCases(metrics *ExtendedDashboardMetrics, baseSQL, scoreExpr, riskExpr string, args []interface{}) error {
	query := fmt.Sprintf(`
		SELECT c.id,
		       COALESCE(c.external_case_id, '') AS external_case_id,
		       COALESCE(NULLIF(c.title, ''), NULLIF(c.event_name, ''), c.external_case_id, CONCAT('case #', c.id::text)) AS title,
		       COALESCE(c.event_name, '') AS event_name,
		       COALESCE(c.status, '') AS status,
		       COALESCE(c.label, '') AS label,
		       COALESCE((SELECT COUNT(*) FROM posts p WHERE p.case_id = c.id), 0) AS post_count,
		       COALESCE(%s, 0) AS risk_score,
		       COALESCE(NULLIF(%s, ''), 'low') AS risk_level
		%s
		ORDER BY COALESCE(%s, 0) DESC, post_count DESC, c.id ASC
		LIMIT 12
	`, scoreExpr, riskExpr, baseSQL, scoreExpr)
	rows, err := p.db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row DashboardTopRiskCase
		if err := rows.Scan(&row.ID, &row.ExternalCaseID, &row.Title, &row.EventName, &row.Status, &row.Label, &row.PostCount, &row.RiskScore, &row.RiskLevel); err != nil {
			return err
		}
		metrics.TopRiskCases = append(metrics.TopRiskCases, row)
	}
	return rows.Err()
}

func (p *PostgresDB) loadDashboardScorerPerformance(metrics *ExtendedDashboardMetrics, whereSQL string, args []interface{}) error {
	query := fmt.Sprintf(`
		SELECT scorer_key, COALESCE(AVG(risk_score), 0) AS avg_risk_score, COUNT(DISTINCT case_id) AS cases_analyzed
		FROM (
			SELECT 'case_scores' AS scorer_key, cs.case_id, cs.risk_score
			FROM case_scores cs JOIN cases c ON c.id = cs.case_id
			WHERE %s
			UNION ALL
			SELECT cms.scorer_key, cms.case_id, cms.risk_score
			FROM case_model_scores cms JOIN cases c ON c.id = cms.case_id
			WHERE %s
		) scores
		GROUP BY scorer_key
		ORDER BY cases_analyzed DESC, avg_risk_score DESC
	`, whereSQL, whereSQL)
	rows, err := p.db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item DashboardScorerPerformance
		if err := rows.Scan(&item.ScorerKey, &item.AvgRiskScore, &item.CasesAnalyzed); err != nil {
			return err
		}
		metrics.ScorerPerformance = append(metrics.ScorerPerformance, item)
	}
	return rows.Err()
}

func (p *PostgresDB) loadDashboardPostAnalysis(metrics *ExtendedDashboardMetrics, baseSQL string, args []interface{}, days int) error {
	if err := p.loadManipulationStats(metrics, baseSQL, args); err != nil {
		return err
	}
	if err := p.loadRecentEvidence(metrics, baseSQL, args); err != nil {
		return err
	}
	if err := p.loadBranchTimeSeries(metrics, baseSQL, args, days); err != nil {
		return err
	}
	return p.loadManipulationTrend(metrics, baseSQL, args, days)
}

func (p *PostgresDB) loadManipulationStats(metrics *ExtendedDashboardMetrics, baseSQL string, args []interface{}) error {
	query := fmt.Sprintf(`
		WITH matched_cases AS (SELECT DISTINCT c.id %s)
		SELECT COALESCE(AVG(ar.manipulation_score), 0),
		       COALESCE(AVG(ar.coordination_contribution), 0),
		       COALESCE(AVG(ar.temporal_contribution), 0),
		       COALESCE(AVG(ar.narrative_contribution), 0),
		       COALESCE(AVG(ar.confidence_score), 0),
		       COUNT(ar.id),
		       COUNT(ar.id) FILTER (WHERE ar.manipulation_score >= 0.7)
		FROM matched_cases mc
		JOIN posts p ON p.case_id = mc.id
		JOIN analysis_results ar ON ar.post_id = p.id
	`, baseSQL)
	stats := &ManipulationStats{}
	if err := p.db.QueryRow(query, args...).Scan(&stats.AvgManipulationScore, &stats.AvgCoordination, &stats.AvgTemporal, &stats.AvgNarrative, &stats.AvgConfidence, &stats.TotalAnalyzed, &stats.HighManipulationCount); err != nil {
		return err
	}
	metrics.ManipulationStats = stats
	return nil
}

func (p *PostgresDB) loadRecentEvidence(metrics *ExtendedDashboardMetrics, baseSQL string, args []interface{}) error {
	query := fmt.Sprintf(`
		WITH matched_cases AS (SELECT DISTINCT c.id %s)
		SELECT mc.id,
		       COALESCE(NULLIF(c.title, ''), NULLIF(c.event_name, ''), c.external_case_id, CONCAT('case #', c.id::text)),
		       ar.manipulation_score,
		       ar.coordination_contribution,
		       ar.temporal_contribution,
		       ar.narrative_contribution,
		       COALESCE(ec.summary, ''),
		       ar.created_at
		FROM matched_cases mc
		JOIN cases c ON c.id = mc.id
		JOIN posts p ON p.case_id = mc.id
		JOIN analysis_results ar ON ar.post_id = p.id
		LEFT JOIN evidence_cards ec ON ec.analysis_result_id = ar.id
		ORDER BY ar.manipulation_score DESC, ar.created_at DESC
		LIMIT 12
	`, baseSQL)
	rows, err := p.db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item EvidenceItem
		if err := rows.Scan(&item.CaseID, &item.CaseTitle, &item.ManipulationScore, &item.Coordination, &item.Temporal, &item.Narrative, &item.Summary, &item.CreatedAt); err != nil {
			return err
		}
		metrics.RecentEvidence = append(metrics.RecentEvidence, item)
	}
	return rows.Err()
}

func (p *PostgresDB) loadBranchTimeSeries(metrics *ExtendedDashboardMetrics, baseSQL string, args []interface{}, days int) error {
	dateFilter := ""
	if days > 0 {
		dateFilter = fmt.Sprintf("WHERE day >= CURRENT_DATE - INTERVAL '%d days'", days)
	}
	query := fmt.Sprintf(`
		WITH matched_cases AS (SELECT DISTINCT c.id %s),
		series AS (
			SELECT DATE(ar.created_at) AS day,
			       COALESCE(AVG(ar.coordination_contribution), 0) AS coordination,
			       COALESCE(AVG(ar.temporal_contribution), 0) AS temporal,
			       COALESCE(AVG(ar.narrative_contribution), 0) AS narrative,
			       COUNT(*) AS cnt
			FROM matched_cases mc
			JOIN posts p ON p.case_id = mc.id
			JOIN analysis_results ar ON ar.post_id = p.id
			GROUP BY DATE(ar.created_at)
		)
		SELECT day, coordination, temporal, narrative, cnt FROM series %s ORDER BY day ASC
	`, baseSQL, dateFilter)
	rows, err := p.db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row BranchTimeSeriesPoint
		var day time.Time
		if err := rows.Scan(&day, &row.Coordination, &row.Temporal, &row.Narrative, &row.Count); err != nil {
			return err
		}
		row.Date = day.Format("2006-01-02")
		metrics.BranchTimeSeries = append(metrics.BranchTimeSeries, row)
	}
	return rows.Err()
}

func (p *PostgresDB) loadManipulationTrend(metrics *ExtendedDashboardMetrics, baseSQL string, args []interface{}, days int) error {
	dateFilter := ""
	if days > 0 {
		dateFilter = fmt.Sprintf("WHERE day >= CURRENT_DATE - INTERVAL '%d days'", days)
	}
	query := fmt.Sprintf(`
		WITH matched_cases AS (SELECT DISTINCT c.id %s),
		series AS (
			SELECT DATE(ar.created_at) AS day,
			       COALESCE(AVG(ar.manipulation_score), 0) AS avg_score,
			       COALESCE(AVG(ar.coordination_contribution), 0) AS coordination,
			       COALESCE(AVG(ar.temporal_contribution), 0) AS temporal,
			       COALESCE(AVG(ar.narrative_contribution), 0) AS narrative,
			       COUNT(DISTINCT mc.id) AS cases_count
			FROM matched_cases mc
			JOIN posts p ON p.case_id = mc.id
			JOIN analysis_results ar ON ar.post_id = p.id
			GROUP BY DATE(ar.created_at)
		)
		SELECT day, avg_score, coordination, temporal, narrative, cases_count FROM series %s ORDER BY day ASC
	`, baseSQL, dateFilter)
	rows, err := p.db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var row ManipulationTrendPoint
		var day time.Time
		if err := rows.Scan(&day, &row.AvgScore, &row.Coordination, &row.Temporal, &row.Narrative, &row.CasesCount); err != nil {
			return err
		}
		row.Date = day.Format("2006-01-02")
		metrics.ManipulationTrend = append(metrics.ManipulationTrend, row)
	}
	return rows.Err()
}

func (p *PostgresDB) GetRadarDataForCase(caseID int64, scorerKey string) ([]RadarDataPoint, error) {
	var coordination, temporal, narrative float64
	scorerKey = normalizeScorerKey(scorerKey)
	if useCaseModelScore(scorerKey) {
		err := p.db.QueryRow(`
			SELECT COALESCE(coordination_score, 0), COALESCE(temporal_score, 0), COALESCE(content_score, 0)
			FROM case_model_scores
			WHERE case_id = $1 AND scorer_key = $2
		`, caseID, scorerKey).Scan(&coordination, &temporal, &narrative)
		if err == nil {
			return radarPoints(coordination, temporal, narrative), nil
		}
		if err != sql.ErrNoRows {
			return nil, err
		}
	}
	err := p.db.QueryRow(`
		SELECT COALESCE(coordination_score, 0), COALESCE(temporal_score, 0), COALESCE(content_score, 0)
		FROM case_scores
		WHERE case_id = $1
	`, caseID).Scan(&coordination, &temporal, &narrative)
	if err != nil {
		return nil, err
	}
	return radarPoints(coordination, temporal, narrative), nil
}

func radarPoints(coordination, temporal, narrative float64) []RadarDataPoint {
	return []RadarDataPoint{
		{Branch: "coordination", Value: coordination},
		{Branch: "temporal", Value: temporal},
		{Branch: "content", Value: narrative},
	}
}

func dashboardWhere(filter DashboardFilter) (string, []interface{}) {
	conditions := make([]string, 0, 8)
	args := make([]interface{}, 0, 8)
	if meaningfulDashboardFilter(filter.SourceName) {
		args = append(args, filter.SourceName)
		conditions = append(conditions, fmt.Sprintf("c.source_name = $%d", len(args)))
	}
	if meaningfulDashboardFilter(filter.DatasetName) {
		args = append(args, filter.DatasetName)
		conditions = append(conditions, fmt.Sprintf("c.dataset_name = $%d", len(args)))
	}
	if meaningfulDashboardFilter(filter.DatasetSplit) {
		args = append(args, filter.DatasetSplit)
		conditions = append(conditions, fmt.Sprintf("c.dataset_split = $%d", len(args)))
	}
	if meaningfulDashboardFilter(filter.EventName) {
		args = append(args, filter.EventName)
		conditions = append(conditions, fmt.Sprintf("c.event_name = $%d", len(args)))
	}
	if meaningfulDashboardFilter(filter.Label) {
		args = append(args, filter.Label)
		conditions = append(conditions, fmt.Sprintf("c.label = $%d", len(args)))
	}
	if meaningfulDashboardFilter(filter.Status) {
		args = append(args, filter.Status)
		conditions = append(conditions, fmt.Sprintf("c.status = $%d", len(args)))
	}
	if len(conditions) == 0 {
		return "1=1", args
	}
	return strings.Join(conditions, " AND "), args
}

func meaningfulDashboardFilter(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value != "all"
}
