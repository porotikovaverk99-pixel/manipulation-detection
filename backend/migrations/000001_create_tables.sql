-- +goose Up
-- =====================================================
-- ДИПЛОМ: Детекция манипулятивных тактик в соцсетях
-- Миграция: создание всех таблиц
-- Версия: 1.0
-- Дата: 2026-04-06
-- =====================================================

-- =====================================================
-- СХЕМА 1: ЯДРО (Core)
-- =====================================================

-- 1. Источники данных (соцсети)
CREATE TABLE IF NOT EXISTS data_sources (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE,
    api_endpoint VARCHAR(500),
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE data_sources IS 'Источники данных: Mastodon, Telegram, VK и т.д.';
COMMENT ON COLUMN data_sources.name IS 'Название соцсети (mastodon, telegram, vk)';
COMMENT ON COLUMN data_sources.api_endpoint IS 'Базовый URL API';

-- 2. Аккаунты (общие для всех соцсетей)
CREATE TABLE IF NOT EXISTS accounts (
    id BIGSERIAL PRIMARY KEY,
    source_id INT NOT NULL REFERENCES data_sources(id) ON DELETE CASCADE,
    external_id VARCHAR(100) NOT NULL,
    username VARCHAR(100) NOT NULL,
    display_name VARCHAR(200),
    account_url VARCHAR(500),
    avatar_url VARCHAR(500),
    metadata JSONB DEFAULT '{}',
    followers_count INT DEFAULT 0,
    following_count INT DEFAULT 0,
    posts_count INT DEFAULT 0,
    is_bot BOOLEAN DEFAULT FALSE,
    is_verified BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP,
    collected_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_seen_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(source_id, external_id)
);

COMMENT ON TABLE accounts IS 'Аккаунты пользователей из разных соцсетей';
COMMENT ON COLUMN accounts.metadata IS 'Source-specific поля в JSON';

-- 3. Посты (универсальная структура)
CREATE TABLE IF NOT EXISTS posts (
    id BIGSERIAL PRIMARY KEY,
    source_id INT NOT NULL REFERENCES data_sources(id) ON DELETE CASCADE,
    external_id VARCHAR(100) NOT NULL,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    language VARCHAR(10),
    published_at TIMESTAMP NOT NULL,
    collected_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    post_url VARCHAR(500),
    reply_to_post_id BIGINT REFERENCES posts(id) ON DELETE SET NULL,
    likes_count INT DEFAULT 0,
    reposts_count INT DEFAULT 0,
    replies_count INT DEFAULT 0,
    views_count INT DEFAULT 0,
    metadata JSONB DEFAULT '{}',
    UNIQUE(source_id, external_id)
);

COMMENT ON TABLE posts IS 'Посты из соцсетей';
COMMENT ON COLUMN posts.reply_to_post_id IS 'Ссылка на родительский пост (для тредов)';

-- 4. Связи: упоминания
CREATE TABLE IF NOT EXISTS post_mentions (
    post_id BIGINT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    mentioned_account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    PRIMARY KEY (post_id, mentioned_account_id)
);

COMMENT ON TABLE post_mentions IS 'Кто кого упоминает в постах';

-- 5. Связи: хэштеги
CREATE TABLE IF NOT EXISTS post_tags (
    post_id BIGINT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    tag_name VARCHAR(200) NOT NULL,
    PRIMARY KEY (post_id, tag_name)
);

COMMENT ON TABLE post_tags IS 'Хэштеги постов';

-- 6. Связи: ссылки в постах
CREATE TABLE IF NOT EXISTS post_links (
    id BIGSERIAL PRIMARY KEY,
    post_id BIGINT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    domain VARCHAR(255),
    expanded_url TEXT,
    title TEXT,
    description TEXT,
    image_url TEXT
);

COMMENT ON TABLE post_links IS 'Ссылки, которыми делятся в постах';

-- =====================================================
-- СХЕМА 2: ТРЕНДЫ (Mastodon admin/trends)
-- =====================================================

-- 7. Трендовые ссылки
CREATE TABLE IF NOT EXISTS trending_links (
    id BIGSERIAL PRIMARY KEY,
    source_id INT NOT NULL REFERENCES data_sources(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    title TEXT,
    description TEXT,
    image_url VARCHAR(500),
    provider_name VARCHAR(100),
    today_accounts INT DEFAULT 0,
    today_uses INT DEFAULT 0,
    history JSONB DEFAULT '[]',
    first_seen_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_seen_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(source_id, url)
);

COMMENT ON TABLE trending_links IS 'Трендовые ссылки из Mastodon admin API';

-- 8. Трендовые хэштеги
CREATE TABLE IF NOT EXISTS trending_tags (
    id BIGSERIAL PRIMARY KEY,
    source_id INT NOT NULL REFERENCES data_sources(id) ON DELETE CASCADE,
    tag_name VARCHAR(200) NOT NULL,
    today_accounts INT DEFAULT 0,
    today_uses INT DEFAULT 0,
    history JSONB DEFAULT '[]',
    trendable BOOLEAN DEFAULT TRUE,
    usable BOOLEAN DEFAULT TRUE,
    requires_review BOOLEAN DEFAULT FALSE,
    first_seen_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_seen_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(source_id, tag_name)
);

COMMENT ON TABLE trending_tags IS 'Трендовые хэштеги из Mastodon admin API';

-- =====================================================
-- СХЕМА 3: АНАЛИЗ И EVIDENCE CARDS
-- =====================================================

-- 9. Окна анализа
CREATE TABLE IF NOT EXISTS analysis_windows (
    id BIGSERIAL PRIMARY KEY,
    window_start TIMESTAMP NOT NULL,
    window_end TIMESTAMP NOT NULL,
    window_type VARCHAR(20) DEFAULT 'hourly',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(window_start, window_end)
);

COMMENT ON TABLE analysis_windows IS 'Временные окна для батчевого анализа';

-- 10. Результаты анализа
CREATE TABLE IF NOT EXISTS analysis_results (
    id BIGSERIAL PRIMARY KEY,
    post_id BIGINT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    window_id BIGINT REFERENCES analysis_windows(id),
    manipulation_score FLOAT NOT NULL CHECK (manipulation_score BETWEEN 0 AND 1),
    confidence_score FLOAT NOT NULL CHECK (confidence_score BETWEEN 0 AND 1),
    coordination_contribution FLOAT CHECK (coordination_contribution BETWEEN 0 AND 1),
    temporal_contribution FLOAT CHECK (temporal_contribution BETWEEN 0 AND 1),
    narrative_contribution FLOAT CHECK (narrative_contribution BETWEEN 0 AND 1),
    coordination_signals JSONB,
    temporal_signals JSONB,
    narrative_signals JSONB,
    has_branch_conflict BOOLEAN DEFAULT FALSE,
    confidence_note TEXT,
    escalation_priority INT CHECK (escalation_priority BETWEEN 1 AND 3),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE analysis_results IS 'Главная таблица результатов анализа';
COMMENT ON COLUMN analysis_results.confidence_note IS 'Причина неопределённости: branch_conflict, sparse_data, distribution_shift';

-- 11. Evidence Cards
CREATE TABLE IF NOT EXISTS evidence_cards (
    id BIGSERIAL PRIMARY KEY,
    analysis_result_id BIGINT NOT NULL REFERENCES analysis_results(id) ON DELETE CASCADE,
    radar_data JSONB,
    summary TEXT,
    key_evidence TEXT[],
    uncertainty_explanation TEXT,
    evidence_links JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE evidence_cards IS 'Карточки доказательств для фронтенда';

-- =====================================================
-- СХЕМА 4: REVIEWER WORKFLOW (A/B тестирование)
-- =====================================================

-- 12. Сессии ревью
CREATE TABLE IF NOT EXISTS review_sessions (
    id BIGSERIAL PRIMARY KEY,
    reviewer_id VARCHAR(100) NOT NULL,
    interface_condition CHAR(1) CHECK (interface_condition IN ('A', 'B')),
    started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    ended_at TIMESTAMP,
    browser_info TEXT,
    session_notes TEXT
);

COMMENT ON TABLE review_sessions IS 'Сессии A/B тестирования';
COMMENT ON COLUMN review_sessions.interface_condition IS 'A=score-only, B=with evidence cards';

-- 13. Решения аналитика
CREATE TABLE IF NOT EXISTS reviewer_decisions (
    id BIGSERIAL PRIMARY KEY,
    session_id BIGINT NOT NULL REFERENCES review_sessions(id),
    evidence_card_id BIGINT REFERENCES evidence_cards(id),
    analysis_result_id BIGINT REFERENCES analysis_results(id),
    escalation_recommendation BOOLEAN NOT NULL,
    confidence_level INT CHECK (confidence_level BETWEEN 1 AND 5),
    rationale TEXT NOT NULL,
    time_spent_seconds INT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE reviewer_decisions IS 'Решения, принятые аналитиками';

-- =====================================================
-- СХЕМА 5: МЕТРИКИ И ЛОГИ
-- =====================================================

-- 14. Метрики оценки
CREATE TABLE IF NOT EXISTS evaluation_metrics (
    id BIGSERIAL PRIMARY KEY,
    evaluation_date DATE DEFAULT CURRENT_DATE,
    top_k_overlap_10 FLOAT,
    top_k_overlap_20 FLOAT,
    rank_correlation_kendall FLOAT,
    reviewer_agreement_kappa FLOAT,
    avg_rationale_length INT,
    avg_decision_time_seconds INT,
    domain_shift_delta FLOAT,
    language_shift_delta FLOAT,
    precision_at_k FLOAT,
    recall_at_k FLOAT,
    notes TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE evaluation_metrics IS 'Метрики для трёх objectives из proposal';

-- 15. Логи сбора данных
CREATE TABLE IF NOT EXISTS collection_logs (
    id BIGSERIAL PRIMARY KEY,
    source_id INT REFERENCES data_sources(id),
    endpoint VARCHAR(255),
    posts_collected INT,
    accounts_collected INT,
    start_time TIMESTAMP,
    end_time TIMESTAMP,
    status VARCHAR(20),
    error_message TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE collection_logs IS 'Логирование процесса сбора данных';

-- =====================================================
-- ИНДЕКСЫ ДЛЯ ПРОИЗВОДИТЕЛЬНОСТИ
-- =====================================================

CREATE INDEX IF NOT EXISTS idx_accounts_username ON accounts(username);
CREATE INDEX IF NOT EXISTS idx_accounts_source ON accounts(source_id);
CREATE INDEX IF NOT EXISTS idx_accounts_metadata ON accounts USING GIN(metadata);

CREATE INDEX IF NOT EXISTS idx_posts_account ON posts(account_id);
CREATE INDEX IF NOT EXISTS idx_posts_published ON posts(published_at DESC);
CREATE INDEX IF NOT EXISTS idx_posts_source ON posts(source_id);
CREATE INDEX IF NOT EXISTS idx_posts_language ON posts(language);
CREATE INDEX IF NOT EXISTS idx_posts_content_fulltext ON posts USING GIN(to_tsvector('russian', content));

CREATE INDEX IF NOT EXISTS idx_post_tags_name ON post_tags(tag_name);

CREATE INDEX IF NOT EXISTS idx_post_links_domain ON post_links(domain);

CREATE INDEX IF NOT EXISTS idx_trending_links_source_last ON trending_links(source_id, last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_trending_tags_name ON trending_tags(tag_name);

CREATE INDEX IF NOT EXISTS idx_results_post ON analysis_results(post_id);
CREATE INDEX IF NOT EXISTS idx_results_window ON analysis_results(window_id);
CREATE INDEX IF NOT EXISTS idx_results_manipulation_score ON analysis_results(manipulation_score DESC);
CREATE INDEX IF NOT EXISTS idx_results_escalation ON analysis_results(escalation_priority);

CREATE INDEX IF NOT EXISTS idx_decisions_session ON review_sessions(reviewer_id, started_at DESC);

-- =====================================================
-- НАЧАЛЬНЫЕ ДАННЫЕ
-- =====================================================

INSERT INTO data_sources (name, api_endpoint, is_active) VALUES
('mastodon', 'https://mastodon.social/api/v1/', true)
ON CONFLICT (name) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS collection_logs;
DROP TABLE IF EXISTS reviewer_decisions;
DROP TABLE IF EXISTS review_sessions;
DROP TABLE IF EXISTS evaluation_metrics;
DROP TABLE IF EXISTS evidence_cards;
DROP TABLE IF EXISTS analysis_results;
DROP TABLE IF EXISTS analysis_windows;
DROP TABLE IF EXISTS trending_tags;
DROP TABLE IF EXISTS trending_links;
DROP TABLE IF EXISTS post_links;
DROP TABLE IF EXISTS post_tags;
DROP TABLE IF EXISTS post_mentions;
DROP TABLE IF EXISTS posts;
DROP TABLE IF EXISTS accounts;
DROP TABLE IF EXISTS data_sources;
