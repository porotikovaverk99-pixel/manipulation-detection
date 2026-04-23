-- +goose Up
-- Миграция: добавление уникальных ограничений для ON CONFLICT
-- Дата: 2026-04-06

-- 1. Добавляем уникальное ограничение на post_id в таблицу analysis_results
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'unique_analysis_results_post_id'
    ) THEN
        ALTER TABLE analysis_results
            ADD CONSTRAINT unique_analysis_results_post_id UNIQUE (post_id);
    END IF;
END $$;
-- +goose StatementEnd

-- 2. Добавляем уникальное ограничение на analysis_result_id в таблицу evidence_cards
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'unique_evidence_cards_analysis_result_id'
    ) THEN
        ALTER TABLE evidence_cards
            ADD CONSTRAINT unique_evidence_cards_analysis_result_id UNIQUE (analysis_result_id);
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE evidence_cards
    DROP CONSTRAINT IF EXISTS unique_evidence_cards_analysis_result_id;

ALTER TABLE analysis_results
    DROP CONSTRAINT IF EXISTS unique_analysis_results_post_id;
