-- Persist attendees per order item so webhook fallback can restore recipient emails

ALTER TABLE order_items
    ADD COLUMN IF NOT EXISTS attendees_json JSONB NOT NULL DEFAULT '[]'::jsonb;
