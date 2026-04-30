-- +goose Up
-- Persistent audit log for API actions, pipeline jobs and operational checks.

CREATE TABLE IF NOT EXISTS audit_events (
    id BIGSERIAL PRIMARY KEY,
    event_time TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    actor_type VARCHAR(50) NOT NULL DEFAULT 'system',
    actor_id VARCHAR(255),
    action VARCHAR(120) NOT NULL,
    entity_type VARCHAR(80),
    entity_id VARCHAR(255),
    status VARCHAR(20) NOT NULL CHECK (status IN ('started', 'succeeded', 'failed', 'skipped')),
    request_id VARCHAR(120),
    source_type VARCHAR(20),
    source_name VARCHAR(100),
    dataset_name VARCHAR(100),
    dataset_split VARCHAR(50),
    payload JSONB NOT NULL DEFAULT '{}',
    error_message TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audit_events_time
    ON audit_events(event_time DESC);

CREATE INDEX IF NOT EXISTS idx_audit_events_action_status
    ON audit_events(action, status, event_time DESC);

CREATE INDEX IF NOT EXISTS idx_audit_events_entity
    ON audit_events(entity_type, entity_id);

CREATE INDEX IF NOT EXISTS idx_audit_events_request_id
    ON audit_events(request_id);

-- +goose Down

DROP INDEX IF EXISTS idx_audit_events_request_id;
DROP INDEX IF EXISTS idx_audit_events_entity;
DROP INDEX IF EXISTS idx_audit_events_action_status;
DROP INDEX IF EXISTS idx_audit_events_time;
DROP TABLE IF EXISTS audit_events;
