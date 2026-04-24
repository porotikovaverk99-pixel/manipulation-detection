-- +goose Up
-- Store multiple model outputs for the same case without overwriting the
-- active case_scores row used by existing API/UI queries.

CREATE TABLE IF NOT EXISTS case_model_scores (
    id BIGSERIAL PRIMARY KEY,
    case_id BIGINT NOT NULL REFERENCES cases(id) ON DELETE CASCADE,
    scorer_key VARCHAR(100) NOT NULL,
    model_version VARCHAR(120) NOT NULL,
    risk_score FLOAT NOT NULL CHECK (risk_score BETWEEN 0 AND 1),
    risk_level VARCHAR(20) NOT NULL CHECK (risk_level IN ('low', 'medium', 'high')),
    confidence_score FLOAT CHECK (confidence_score BETWEEN 0 AND 1),
    temporal_score FLOAT CHECK (temporal_score BETWEEN 0 AND 1),
    coordination_score FLOAT CHECK (coordination_score BETWEEN 0 AND 1),
    content_score FLOAT CHECK (content_score BETWEEN 0 AND 1),
    evidence JSONB NOT NULL DEFAULT '[]',
    feature_payload JSONB NOT NULL DEFAULT '{}',
    model_info JSONB NOT NULL DEFAULT '{}',
    pipeline_hash VARCHAR(64),
    source_endpoint VARCHAR(120),
    computed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (case_id, scorer_key)
);

CREATE INDEX IF NOT EXISTS idx_case_model_scores_scorer_risk
    ON case_model_scores(scorer_key, risk_score DESC);

CREATE INDEX IF NOT EXISTS idx_case_model_scores_case
    ON case_model_scores(case_id, computed_at DESC);

INSERT INTO case_model_scores (
    case_id,
    scorer_key,
    model_version,
    risk_score,
    risk_level,
    temporal_score,
    coordination_score,
    content_score,
    evidence,
    pipeline_hash,
    source_endpoint,
    computed_at
)
SELECT
    case_id,
    'case_feature',
    score_version,
    risk_score,
    risk_level,
    temporal_score,
    coordination_score,
    content_score,
    evidence,
    pipeline_hash,
    '/analyze/case',
    computed_at
FROM case_scores
ON CONFLICT (case_id, scorer_key) DO NOTHING;

-- +goose Down

DROP INDEX IF EXISTS idx_case_model_scores_case;
DROP INDEX IF EXISTS idx_case_model_scores_scorer_risk;
DROP TABLE IF EXISTS case_model_scores;
