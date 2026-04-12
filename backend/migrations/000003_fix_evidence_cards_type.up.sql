-- Миграция: исправление типа key_evidence на JSONB
-- Дата: 2026-04-06

-- 1. Удаляем старый столбец
ALTER TABLE evidence_cards DROP COLUMN IF EXISTS key_evidence;

-- 2. Добавляем новый столбец с типом JSONB
ALTER TABLE evidence_cards ADD COLUMN key_evidence JSONB;

-- 3. Обновляем существующие записи (если есть)
UPDATE evidence_cards SET key_evidence = '[]'::JSONB WHERE key_evidence IS NULL;