-- Ajouter une règle configurable "1 ticket par mail" au niveau du type de ticket
ALTER TABLE ticket_types
    ADD COLUMN IF NOT EXISTS one_ticket_per_email BOOLEAN NOT NULL DEFAULT true;

-- Ajouter l'email nominatif directement sur chaque ticket
ALTER TABLE tickets
    ADD COLUMN IF NOT EXISTS attendee_email VARCHAR(255);

-- Backfill des tickets historiques avec l'email client de la commande
UPDATE tickets t
SET attendee_email = o.customer_email
FROM orders o
WHERE t.order_id = o.id
  AND (t.attendee_email IS NULL OR t.attendee_email = '');

CREATE INDEX IF NOT EXISTS idx_ticket_types_one_ticket_per_email ON ticket_types(one_ticket_per_email);
CREATE INDEX IF NOT EXISTS idx_tickets_attendee_email ON tickets(LOWER(attendee_email));
