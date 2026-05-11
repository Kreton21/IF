-- Add refund state and category linkage on tickets
ALTER TABLE tickets
  ADD COLUMN IF NOT EXISTS category_id UUID REFERENCES ticket_categories(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS is_refunded BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS refunded_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_tickets_refunded ON tickets(is_refunded);
CREATE INDEX IF NOT EXISTS idx_tickets_category_id ON tickets(category_id);
