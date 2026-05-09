package repository

import (
	"fmt"
	"strings"
	"time"
)

// GetDashboardMetrics возвращает агрегаты для аналитического дашборда.
func (p *PostgresDB) GetDashboardMetrics(filter DashboardFilter) (DashboardMetrics, error) {
	if filter.Days <= 0 {
		filter.Days = 30
	}
	if filter.Days > 365 {
		filter.Days = 365
	}

	whereSQL, baseArgs := dashboardWhere(filter)
	scorerKey := normalizeScorerKey(filter.ScorerKey)
	useModelScore := useCaseModelScore(scorerKey)

	scoreJoin := "LEFT JOIN case_scores cs ON cs.case_id = c.id"
	riskExpr := "COALESCE(cs.risk_level, '')"
	scoreExpr := "cs.risk_score"
	baseArgsWithScore := append([]interface{}{}, baseArgs...)
	if useModelScore {
		baseArgsWithScore = append(baseArgsWithScore, scorerKey)
		scoreJoin = fmt.Sprintf("LEFT JOIN case_model_scores cms ON cms.case_id = c.id AND cms.scorer_key = $%d", len(baseArgsWithScore))
		riskExpr = "COALESCE(cms.risk_level, '')"
		scoreExpr = "cms.risk_score"
	}
	baseSQL := fmt.Sprintf("FROM cases c %s WHERE %s", scoreJoin, whereSQL)

	var metrics DashboardMetrics
	if err := p.loadDashboardRiskDistribution(&metrics, baseSQL, riskExpr, baseArgsWithScore); err != nil {
		return DashboardMetrics{}, err
	}
	if err := p.loadDashboardCasesOverTime(&metrics, baseSQL, riskExpr, baseArgsWithScore); err != nil {
		return DashboardMetrics{}, err
	}
	if err := p.loadDashboardTopEvents(&metrics, baseSQL, scoreExpr, baseArgsWithScore); err != nil {
		return DashboardMetrics{}, err
	}
	if err := p.loadDashboardTimeline(&metrics, baseSQL, baseArgsWithScore); err != nil {
		return DashboardMetrics{}, err
	}
	if err := p.loadDashboardScorerPerformance(&metrics, whereSQL, baseArgs); err != nil {
		return DashboardMetrics{}, err
	}

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

	return metrics, nil
}

func dashboardWhere(filter DashboardFilter) (string, []interface{}) {
	conditions := make([]string, 0, 8)
	args := make([]interface{}, 0, 8)

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
	if filter.EventName != "" {
		args = append(args, filter.EventName)
		conditions = append(conditions, fmt.Sprintf("c.event_name = $%d", len(args)))
	}
	if filter.Label != "" {
		args = append(args, filter.Label)
		conditions = append(conditions, fmt.Sprintf("c.label = $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		conditions = append(conditions, fmt.Sprintf("c.status = $%d", len(args)))
	}
	if len(conditions) == 0 {
		conditions = append(conditions, "1=1")
	}
	return strings.Join(conditions, " AND "), args
}

func (p *PostgresDB) loadDashboardRiskDistribution(metrics *DashboardMetrics, baseSQL, riskExpr string, args []interface{}) error {
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

func (p *PostgresDB) loadDashboardCasesOverTime(metrics *DashboardMetrics, baseSQL, riskExpr string, args []interface{}) error {
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
			return fmt.Errorf("scan dashboard cases over time: %w", err)
		}
		point.Date = day.Format("2006-01-02")
		metrics.CasesOverTime = append(metrics.CasesOverTime, point)
	}
	return rows.Err()
}

func (p *PostgresDB) loadDashboardTopEvents(metrics *DashboardMetrics, baseSQL, scoreExpr string, args []interface{}) error {
	query := fmt.Sprintf(`
		SELECT COALESCE(NULLIF(c.event_name, ''), 'unknown') AS event_name,
		       COUNT(*) AS case_count,
		       COALESCE(AVG(%s), 0) AS avg_risk
		%s
		GROUP BY COALESCE(NULLIF(c.event_name, ''), 'unknown')
		ORDER BY case_count DESC, avg_risk DESC
		LIMIT 10
	`, scoreExpr, baseSQL)
	rows, err := p.db.Query(query, args...)
	if err != nil {
		return fmt.Errorf("dashboard top events: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var event DashboardTopEvent
		if err := rows.Scan(&event.EventName, &event.CaseCount, &event.AvgRisk); err != nil {
			return fmt.Errorf("scan dashboard top events: %w", err)
		}
		metrics.TopEvents = append(metrics.TopEvents, event)
	}
	return rows.Err()
}

func (p *PostgresDB) loadDashboardTimeline(metrics *DashboardMetrics, baseSQL string, args []interface{}) error {
	query := fmt.Sprintf(`
		SELECT DATE(COALESCE(c.first_event_at, c.opened_at, c.created_at)) AS day, COUNT(*) AS event_count
		%s
		GROUP BY day
		ORDER BY day ASC
	`, baseSQL)
	rows, err := p.db.Query(query, args...)
	if err != nil {
		return fmt.Errorf("dashboard timeline: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var point DashboardTimelinePoint
		var day time.Time
		if err := rows.Scan(&day, &point.EventCount); err != nil {
			return fmt.Errorf("scan dashboard timeline: %w", err)
		}
		point.Date = day.Format("2006-01-02")
		metrics.TimelineData = append(metrics.TimelineData, point)
	}
	return rows.Err()
}

func (p *PostgresDB) loadDashboardScorerPerformance(metrics *DashboardMetrics, whereSQL string, args []interface{}) error {
	query := fmt.Sprintf(`
		SELECT cms.scorer_key,
		       COALESCE(AVG(cms.risk_score), 0) AS avg_risk_score,
		       COUNT(*) AS cases_analyzed
		FROM case_model_scores cms
		JOIN cases c ON c.id = cms.case_id
		WHERE %s
		GROUP BY cms.scorer_key
		ORDER BY cases_analyzed DESC, avg_risk_score DESC
	`, whereSQL)
	rows, err := p.db.Query(query, args...)
	if err != nil {
		return fmt.Errorf("dashboard scorer performance: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var perf DashboardScorerPerformance
		if err := rows.Scan(&perf.ScorerKey, &perf.AvgRiskScore, &perf.CasesAnalyzed); err != nil {
			return fmt.Errorf("scan dashboard scorer performance: %w", err)
		}
		metrics.ScorerPerformance = append(metrics.ScorerPerformance, perf)
	}
	return rows.Err()
}
