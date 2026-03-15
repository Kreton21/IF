-- Add is_masked column to ticket_types and ticket_categories
-- Masked items are hidden from the public site but still exist in admin

ALTER TABLE ticket_types ADD COLUMN is_masked BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE ticket_categories ADD COLUMN is_masked BOOLEAN NOT NULL DEFAULT false;

-- Index for public queries filtering out masked items
CREATE INDEX idx_ticket_types_not_masked ON ticket_types(is_masked) WHERE is_masked = false;
CREATE INDEX idx_ticket_categories_not_masked ON ticket_categories(is_masked) WHERE is_masked = false;
