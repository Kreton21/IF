-- Ajout de l'assurance remboursement (+1€)

ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS wants_refund_insurance BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_orders_wants_refund_insurance
    ON orders (wants_refund_insurance)
    WHERE wants_refund_insurance = true;
