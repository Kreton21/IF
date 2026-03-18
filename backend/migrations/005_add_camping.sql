-- Ajout de la gestion camping

ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS wants_camping BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE tickets
    ADD COLUMN IF NOT EXISTS is_camping BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_orders_wants_camping
    ON orders (wants_camping)
    WHERE wants_camping = true;

CREATE INDEX IF NOT EXISTS idx_tickets_is_camping
    ON tickets (is_camping)
    WHERE is_camping = true;
