-- Add is_checkbox flag on categories to support one optional checkbox category per ticket type
ALTER TABLE ticket_categories
  ADD COLUMN IF NOT EXISTS is_checkbox BOOLEAN NOT NULL DEFAULT false;

-- Enforce only one checkbox-category per ticket type
CREATE UNIQUE INDEX IF NOT EXISTS idx_ticket_categories_one_checkbox_per_type
  ON ticket_categories(ticket_type_id)
  WHERE is_checkbox = true;
