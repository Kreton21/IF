-- Add active flag to coupons

ALTER TABLE coupons
    ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true;

CREATE INDEX IF NOT EXISTS idx_coupons_active ON coupons(is_active);
