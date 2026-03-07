-- IF Festival - Schema initial
-- PostgreSQL

-- Extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- ============================================
-- Types de tickets (catégories)
-- ============================================
CREATE TABLE ticket_types (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name            VARCHAR(255) NOT NULL,
    description     TEXT,
    price_cents     INTEGER NOT NULL,          -- Prix en centimes (ex: 3500 = 35.00€)
    quantity_total  INTEGER NOT NULL,           -- Nombre total disponible
    quantity_sold   INTEGER NOT NULL DEFAULT 0, -- Nombre vendu
    sale_start      TIMESTAMPTZ NOT NULL,
    sale_end        TIMESTAMPTZ NOT NULL,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    max_per_order   INTEGER NOT NULL DEFAULT 10,
    allowed_domains TEXT[] DEFAULT '{}',        -- Domaines email autorisés (banque de domaines)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index pour les requêtes fréquentes
CREATE INDEX idx_ticket_types_active ON ticket_types(is_active) WHERE is_active = true;
CREATE INDEX idx_ticket_types_sale_period ON ticket_types(sale_start, sale_end);

-- ============================================
-- Catégories de tickets (subdivisions par domaine)
-- ============================================
CREATE TABLE ticket_categories (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    ticket_type_id      UUID NOT NULL REFERENCES ticket_types(id) ON DELETE CASCADE,
    name                VARCHAR(255) NOT NULL,
    quantity_allocated  INTEGER NOT NULL DEFAULT 0,
    quantity_sold       INTEGER NOT NULL DEFAULT 0,
    allowed_domains     TEXT[] DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ticket_categories_type ON ticket_categories(ticket_type_id);

-- ============================================
-- Commandes
-- ============================================
CREATE TYPE order_status AS ENUM (
    'pending',      -- En attente de paiement
    'paid',         -- Payé via HelloAsso
    'confirmed',    -- Confirmé, QR généré
    'cancelled',    -- Annulé
    'refunded'      -- Remboursé
);

CREATE TABLE orders (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    order_number        VARCHAR(20) NOT NULL UNIQUE,  -- Numéro lisible (ex: IF-2026-00001)
    
    -- Client
    customer_email      VARCHAR(255) NOT NULL,
    customer_first_name VARCHAR(100) NOT NULL,
    customer_last_name  VARCHAR(100) NOT NULL,
    customer_phone      VARCHAR(20),
    
    -- Paiement
    total_cents         INTEGER NOT NULL,
    status              order_status NOT NULL DEFAULT 'pending',
    
    -- HelloAsso
    helloasso_checkout_id   VARCHAR(255),
    helloasso_payment_id    VARCHAR(255),
    helloasso_checkout_url  TEXT,
    
    -- Métadonnées
    ip_address          INET,
    user_agent          TEXT,
    
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    paid_at             TIMESTAMPTZ,
    confirmed_at        TIMESTAMPTZ
);

-- Index pour les recherches admin et lookups fréquents
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_email ON orders(customer_email);
CREATE INDEX idx_orders_created ON orders(created_at DESC);
CREATE INDEX idx_orders_helloasso_checkout ON orders(helloasso_checkout_id) WHERE helloasso_checkout_id IS NOT NULL;
CREATE INDEX idx_orders_helloasso_payment ON orders(helloasso_payment_id) WHERE helloasso_payment_id IS NOT NULL;
CREATE INDEX idx_orders_number ON orders(order_number);

-- Séquence pour les numéros de commande
CREATE SEQUENCE order_number_seq START 1;

-- ============================================
-- Tickets (un ticket par entrée)
-- ============================================
CREATE TABLE tickets (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    order_id        UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    ticket_type_id  UUID NOT NULL REFERENCES ticket_types(id),
    
    -- QR Code
    qr_token        VARCHAR(64) NOT NULL UNIQUE,  -- Token unique pour le QR
    qr_code_data    BYTEA,                         -- Image QR encodée PNG
    
    -- Validation le jour J
    is_validated     BOOLEAN NOT NULL DEFAULT false,
    validated_at     TIMESTAMPTZ,
    validated_by     VARCHAR(100),                  -- Nom de l'admin qui a validé
    
    -- Attendee info (si différent du client)
    attendee_first_name VARCHAR(100),
    attendee_last_name  VARCHAR(100),
    
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index critique pour la validation rapide des QR codes
CREATE UNIQUE INDEX idx_tickets_qr_token ON tickets(qr_token);
CREATE INDEX idx_tickets_order ON tickets(order_id);
CREATE INDEX idx_tickets_type ON tickets(ticket_type_id);
CREATE INDEX idx_tickets_validated ON tickets(is_validated) WHERE is_validated = false;

-- ============================================
-- Admins
-- ============================================
CREATE TABLE admins (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username        VARCHAR(100) NOT NULL UNIQUE,
    password_hash   VARCHAR(255) NOT NULL,
    display_name    VARCHAR(100) NOT NULL,
    role            VARCHAR(50) NOT NULL DEFAULT 'staff', -- 'admin' ou 'staff'
    is_active       BOOLEAN NOT NULL DEFAULT true,
    last_login      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================
-- Log de webhooks HelloAsso (audit trail)
-- ============================================
CREATE TABLE webhook_logs (
    id              BIGSERIAL PRIMARY KEY,
    event_type      VARCHAR(100) NOT NULL,
    payload         JSONB NOT NULL,
    processed       BOOLEAN NOT NULL DEFAULT false,
    error_message   TEXT,
    received_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at    TIMESTAMPTZ
);

CREATE INDEX idx_webhook_logs_processed ON webhook_logs(processed) WHERE processed = false;
CREATE INDEX idx_webhook_logs_received ON webhook_logs(received_at DESC);

-- ============================================
-- Vue matérialisée pour les stats (performance)
-- ============================================
CREATE MATERIALIZED VIEW sales_stats AS
SELECT 
    tt.id AS ticket_type_id,
    tt.name AS ticket_type_name,
    tt.price_cents,
    tt.quantity_total,
    tt.quantity_sold,
    COUNT(t.id) FILTER (WHERE o.status IN ('paid', 'confirmed')) AS tickets_sold,
    COUNT(t.id) FILTER (WHERE t.is_validated = true) AS tickets_validated,
    SUM(tt.price_cents) FILTER (WHERE o.status IN ('paid', 'confirmed')) AS revenue_cents,
    DATE(o.created_at) AS sale_date
FROM ticket_types tt
LEFT JOIN tickets t ON t.ticket_type_id = tt.id
LEFT JOIN orders o ON o.id = t.order_id
GROUP BY tt.id, tt.name, tt.price_cents, tt.quantity_total, tt.quantity_sold, DATE(o.created_at);

CREATE UNIQUE INDEX idx_sales_stats_type_date ON sales_stats(ticket_type_id, sale_date);

-- Fonction pour rafraîchir les stats
CREATE OR REPLACE FUNCTION refresh_sales_stats()
RETURNS void AS $$
BEGIN
    REFRESH MATERIALIZED VIEW CONCURRENTLY sales_stats;
END;
$$ LANGUAGE plpgsql;

-- ============================================
-- Trigger pour updated_at
-- ============================================
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_orders_updated_at
    BEFORE UPDATE ON orders
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trg_ticket_types_updated_at
    BEFORE UPDATE ON ticket_types
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
