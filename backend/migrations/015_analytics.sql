-- Analytics: page visits, clicks, and session duration tracking

CREATE TABLE IF NOT EXISTS analytics_sessions (
    id              BIGSERIAL PRIMARY KEY,
    session_id      TEXT NOT NULL,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at        TIMESTAMPTZ,
    duration_ms     BIGINT,
    page            TEXT NOT NULL DEFAULT '/',
    referrer        TEXT,
    user_agent      TEXT,
    ip              TEXT
);

CREATE INDEX IF NOT EXISTS idx_analytics_sessions_started_at ON analytics_sessions (started_at);
CREATE INDEX IF NOT EXISTS idx_analytics_sessions_session_id ON analytics_sessions (session_id);

CREATE TABLE IF NOT EXISTS analytics_clicks (
    id          BIGSERIAL PRIMARY KEY,
    session_id  TEXT NOT NULL,
    clicked_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    target      TEXT,
    page        TEXT NOT NULL DEFAULT '/'
);

CREATE INDEX IF NOT EXISTS idx_analytics_clicks_clicked_at ON analytics_clicks (clicked_at);
CREATE INDEX IF NOT EXISTS idx_analytics_clicks_session_id ON analytics_clicks (session_id);
