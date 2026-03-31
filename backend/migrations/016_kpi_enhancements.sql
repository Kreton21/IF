-- KPI Enhancements: new session flag, UTM source, page views tracking, marketing costs

-- 1. Add is_new_session and utm_source to analytics_sessions
ALTER TABLE analytics_sessions
    ADD COLUMN IF NOT EXISTS is_new_session BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS utm_source      TEXT,
    ADD COLUMN IF NOT EXISTS utm_medium      TEXT,
    ADD COLUMN IF NOT EXISTS utm_campaign    TEXT;

-- 2. Table for per-page view time tracking
CREATE TABLE IF NOT EXISTS analytics_page_views (
    id          BIGSERIAL PRIMARY KEY,
    session_id  TEXT NOT NULL,
    page        TEXT NOT NULL DEFAULT '/',
    entered_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    duration_ms BIGINT
);

CREATE INDEX IF NOT EXISTS idx_analytics_pv_session ON analytics_page_views (session_id);
CREATE INDEX IF NOT EXISTS idx_analytics_pv_entered ON analytics_page_views (entered_at);
CREATE INDEX IF NOT EXISTS idx_analytics_pv_page    ON analytics_page_views (page);

-- 3. Marketing costs table (manual entries for CAC)
CREATE TABLE IF NOT EXISTS marketing_costs (
    id          BIGSERIAL PRIMARY KEY,
    cost_date   DATE NOT NULL,
    amount_eur  NUMERIC(10,2) NOT NULL CHECK (amount_eur >= 0),
    label       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_marketing_costs_date ON marketing_costs (cost_date);

-- 4. Existing sessions table index for is_new_session queries
CREATE INDEX IF NOT EXISTS idx_analytics_sessions_is_new ON analytics_sessions (is_new_session);
