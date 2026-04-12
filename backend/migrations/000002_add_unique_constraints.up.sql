-- Миграция: добавление уникальных ограничений для ON CONFLICT
-- Дата: 2026-04-06

-- 1. Добавляем уникальное ограничение на post_id в таблицу analysis_results
-- Если ограничение уже существует, команда выдаст ошибку, но это нормально
ALTER TABLE analysis_results ADD CONSTRAINT unique_analysis_results_post_id UNIQUE (post_id);

-- 2. Добавляем уникальное ограничение на analysis_result_id в таблицу evidence_cards
ALTER TABLE evidence_cards ADD CONSTRAINT unique_evidence_cards_analysis_result_id UNIQUE (analysis_result_id);