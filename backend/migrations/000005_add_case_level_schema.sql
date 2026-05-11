-- +goose Up
-- Case-level schema for dataset and future live pipelines.

CREATE TABLE IF NOT EXISTS cases (
    id BIGSERIAL PRIMARY KEY,
    source_type VARCHAR(20) NOT NULL CHECK (source_type IN ('dataset', 'live')),
    source_name VARCHAR(100) NOT NULL,
    dataset_name VARCHAR(100) NOT NULL DEFAULT '',
    dataset_split VARCHAR(50) NOT NULL DEFAULT '',
    external_case_id VARCHAR(255) NOT NULL,
    case_type VARCHAR(50) NOT NULL DEFAULT 'thread',
    label VARCHAR(100),
    title TEXT,
    description TEXT,
    event_name VARCHAR(255),
    root_post_external_id VARCHAR(255),
    status VARCHAR(20) NOT NULL DEFAULT 'closed' CHECK (status IN ('open', 'closed', 'finalized')),
    opened_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    closed_at TIMESTAMP,
    first_event_at TIMESTAMP,
    last_event_at TIMESTAMP,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (source_type, source_name, dataset_name, dataset_split, external_case_id)
);

COMMENT ON TABLE cases IS 'Case-level сущность для threads, campaign slices и future live clusters';

ALTER TABLE posts
    ADD COLUMN IF NOT EXISTS case_id BIGINT REFERENCES cases(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS is_case_root BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_cases_dataset_ref ON cases(dataset_name, dataset_split);
CREATE INDEX IF NOT EXISTS idx_cases_event_label ON cases(event_name, label);
CREATE INDEX IF NOT EXISTS idx_cases_time_range ON cases(first_event_at, last_event_at);
CREATE INDEX IF NOT EXISTS idx_posts_case_id ON posts(case_id);

CREATE TABLE IF NOT EXISTS case_features (
    case_id BIGINT PRIMARY KEY REFERENCES cases(id) ON DELETE CASCADE,
    feature_version VARCHAR(50) NOT NULL,
    event_count INT NOT NULL DEFAULT 0,
    unique_account_count INT NOT NULL DEFAULT 0,
    unique_url_count INT NOT NULL DEFAULT 0,
    unique_hashtag_count INT NOT NULL DEFAULT 0,
    temporal_features JSONB NOT NULL DEFAULT '{}',
    coordination_features JSONB NOT NULL DEFAULT '{}',
    content_features JSONB NOT NULL DEFAULT '{}',
    feature_payload JSONB NOT NULL DEFAULT '{}',
    computed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS case_scores (
    case_id BIGINT PRIMARY KEY REFERENCES cases(id) ON DELETE CASCADE,
    score_version VARCHAR(50) NOT NULL,
    temporal_score FLOAT CHECK (temporal_score BETWEEN 0 AND 1),
    coordination_score FLOAT CHECK (coordination_score BETWEEN 0 AND 1),
    content_score FLOAT CHECK (content_score BETWEEN 0 AND 1),
    risk_score FLOAT NOT NULL CHECK (risk_score BETWEEN 0 AND 1),
    risk_level VARCHAR(20) NOT NULL CHECK (risk_level IN ('low', 'medium', 'high')),
    evidence JSONB NOT NULL DEFAULT '[]',
    pipeline_hash VARCHAR(64),
    computed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_case_scores_risk_score ON case_scores(risk_score DESC);

CREATE TABLE IF NOT EXISTS evaluation_runs (
    id BIGSERIAL PRIMARY KEY,
    source_name VARCHAR(100) NOT NULL,
    dataset_name VARCHAR(100) NOT NULL DEFAULT '',
    dataset_split VARCHAR(50) NOT NULL DEFAULT '',
    pipeline_hash VARCHAR(64) NOT NULL,
    scorer_version VARCHAR(50),
    case_count INT NOT NULL DEFAULT 0,
    precision_at_10 FLOAT,
    precision_at_20 FLOAT,
    recall FLOAT,
    f1 FLOAT,
    roc_auc FLOAT,
    pr_auc FLOAT,
    metrics JSONB NOT NULL DEFAULT '{}',
    notes TEXT,
    evaluated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_evaluation_runs_dataset ON evaluation_runs(dataset_name, dataset_split, evaluated_at DESC);

-- +goose Down

DROP INDEX IF EXISTS idx_evaluation_runs_dataset;
DROP TABLE IF EXISTS evaluation_runs;

DROP INDEX IF EXISTS idx_case_scores_risk_score;
DROP TABLE IF EXISTS case_scores;

DROP TABLE IF EXISTS case_features;

DROP INDEX IF EXISTS idx_posts_case_id;
DROP INDEX IF EXISTS idx_cases_time_range;
DROP INDEX IF EXISTS idx_cases_event_label;
DROP INDEX IF EXISTS idx_cases_dataset_ref;

ALTER TABLE posts
    DROP COLUMN IF EXISTS is_case_root,
    DROP COLUMN IF EXISTS case_id;

DROP TABLE IF EXISTS cases;
