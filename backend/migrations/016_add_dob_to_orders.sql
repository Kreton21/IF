-- Store date of birth on orders for ticket/email context
ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS date_of_birth VARCHAR(10);
