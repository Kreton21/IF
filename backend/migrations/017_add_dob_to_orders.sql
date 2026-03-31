-- Store date of birth on orders for PDF ticket display
ALTER TABLE orders ADD COLUMN IF NOT EXISTS date_of_birth VARCHAR(10);
