-- Coupons for ticket discounts

CREATE TABLE IF NOT EXISTS coupons (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name            VARCHAR(255) NOT NULL,
    code            VARCHAR(32) NOT NULL UNIQUE,
    ticket_type_id  UUID NOT NULL REFERENCES ticket_types(id) ON DELETE CASCADE,
    max_uses        INTEGER NOT NULL CHECK (max_uses >= 0),
    used_count      INTEGER NOT NULL DEFAULT 0 CHECK (used_count >= 0),
    discount_cents  INTEGER NOT NULL CHECK (discount_cents >= 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_coupons_ticket_type ON coupons(ticket_type_id);
CREATE INDEX IF NOT EXISTS idx_coupons_code ON coupons(code);

CREATE TABLE IF NOT EXISTS coupon_redemptions (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    coupon_id   UUID NOT NULL REFERENCES coupons(id) ON DELETE CASCADE,
    order_id    UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    uses        INTEGER NOT NULL CHECK (uses > 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_coupon_redemptions_coupon ON coupon_redemptions(coupon_id);
CREATE INDEX IF NOT EXISTS idx_coupon_redemptions_order ON coupon_redemptions(order_id);

ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS coupon_id UUID REFERENCES coupons(id),
    ADD COLUMN IF NOT EXISTS coupon_code VARCHAR(32),
    ADD COLUMN IF NOT EXISTS coupon_discount_cents INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS coupon_uses_applied INTEGER NOT NULL DEFAULT 0;
