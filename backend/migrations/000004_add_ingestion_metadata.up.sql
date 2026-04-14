-- Миграция: ingestion metadata для dataset-first и live-track совместимости
-- Дата: 2026-04-14

-- 1) Таблица запусков ingestion/replay
CREATE TABLE IF NOT EXISTS ingestion_runs (
    id BIGSERIAL PRIMARY KEY,
    source_type VARCHAR(20) NOT NULL CHECK (source_type IN ('live', 'dataset')),
    source_name VARCHAR(100) NOT NULL,
    dataset_name VARCHAR(100),
    dataset_split VARCHAR(50),
    started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at TIMESTAMP,
    status VARCHAR(20) NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'completed', 'failed')),
    notes TEXT
);

COMMENT ON TABLE ingestion_runs IS 'Запуски ingestion/replay для трассировки и воспроизводимости';

-- 2) Расширяем posts canonical ingestion-полями
ALTER TABLE posts
    ADD COLUMN IF NOT EXISTS source_type VARCHAR(20) NOT NULL DEFAULT 'live' CHECK (source_type IN ('live', 'dataset')),
    ADD COLUMN IF NOT EXISTS raw_payload_ref TEXT,
    ADD COLUMN IF NOT EXISTS raw_payload_hash VARCHAR(64),
    ADD COLUMN IF NOT EXISTS dataset_name VARCHAR(100),
    ADD COLUMN IF NOT EXISTS dataset_split VARCHAR(50),
    ADD COLUMN IF NOT EXISTS dataset_record_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS ingestion_run_id BIGINT REFERENCES ingestion_runs(id) ON DELETE SET NULL;

COMMENT ON COLUMN posts.raw_payload_ref IS 'Ссылка на исходный payload: URL или dataset reference';
COMMENT ON COLUMN posts.raw_payload_hash IS 'SHA-256 исходного payload для аудита воспроизводимости';

CREATE INDEX IF NOT EXISTS idx_posts_source_type ON posts(source_type);
CREATE INDEX IF NOT EXISTS idx_posts_dataset_ref ON posts(dataset_name, dataset_split);
CREATE INDEX IF NOT EXISTS idx_posts_ingestion_run ON posts(ingestion_run_id);
