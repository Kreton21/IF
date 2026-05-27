-- Allow negative coupon discounts (price increase)
ALTER TABLE coupons
    DROP CONSTRAINT IF EXISTS coupons_discount_cents_check;
