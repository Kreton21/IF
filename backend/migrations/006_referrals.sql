CREATE TABLE IF NOT EXISTS referral_links (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    code TEXT NOT NULL UNIQUE,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS referral_clicks (
    id BIGSERIAL PRIMARY KEY,
    referral_link_id UUID NOT NULL REFERENCES referral_links(id) ON DELETE CASCADE,
    visitor_id TEXT NOT NULL,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_referral_clicks_link_created ON referral_clicks(referral_link_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_referral_clicks_link_visitor ON referral_clicks(referral_link_id, visitor_id);

CREATE TABLE IF NOT EXISTS referral_order_conversions (
    order_id UUID PRIMARY KEY REFERENCES orders(id) ON DELETE CASCADE,
    referral_link_id UUID NOT NULL REFERENCES referral_links(id) ON DELETE CASCADE,
    visitor_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_referral_conversions_link ON referral_order_conversions(referral_link_id);
