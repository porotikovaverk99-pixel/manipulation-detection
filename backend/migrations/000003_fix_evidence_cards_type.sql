-- +goose Up
-- Миграция: исправление типа key_evidence на JSONB
-- Дата: 2026-04-06

-- 1. Безопасно переводим key_evidence в JSONB.
-- Старые базы могли иметь TEXT[], новые уже имеют JSONB.
-- +goose StatementBegin
DO $$
DECLARE
    column_type TEXT;
BEGIN
    SELECT data_type INTO column_type
    FROM information_schema.columns
    WHERE table_name = 'evidence_cards'
      AND column_name = 'key_evidence';

    IF column_type IS NULL THEN
        ALTER TABLE evidence_cards ADD COLUMN key_evidence JSONB;
    ELSIF column_type <> 'jsonb' THEN
        ALTER TABLE evidence_cards
            ALTER COLUMN key_evidence TYPE JSONB
            USING to_jsonb(key_evidence);
    END IF;
END $$;
-- +goose StatementEnd

-- 2. Обновляем существующие записи (если есть)
UPDATE evidence_cards SET key_evidence = '[]'::JSONB WHERE key_evidence IS NULL;

-- +goose Down
-- Intentional no-op: converting JSONB evidence back to TEXT[] can lose shape
-- once evidence starts carrying structured values.
SELECT 1;
