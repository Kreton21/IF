#!/bin/bash
# Seed the IF Festival database with mock ticket types, categories, and transactions
# Usage: ./scripts/seed_db.sh
# Runs clear_db.sh first (with confirmation), then inserts seed data.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CONTAINER="if-festival-postgres"
DB_NAME="festival_db"
DB_USER="festival"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

echo -e "${CYAN}🌱 IF Festival - Database Seeder${NC}\n"

# ── Optionally clear first ──
echo -e "${YELLOW}⚠  This will clear all existing data and insert fresh mock data.${NC}"
read -p "Continue? (y/N) " -n 1 -r
echo ""

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 0
fi

# Check Docker container is running
if ! docker exec "$CONTAINER" pg_isready -U "$DB_USER" -d "$DB_NAME" &>/dev/null; then
    echo -e "${RED}✗ PostgreSQL container '$CONTAINER' is not running.${NC}"
    echo -e "  Run: docker compose up -d postgres"
    exit 1
fi

echo -e "\n${YELLOW}Clearing existing data...${NC}"

docker exec -i "$CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" <<'SQL'
BEGIN;
DELETE FROM webhook_logs;
DELETE FROM tickets;
DELETE FROM orders;
DELETE FROM ticket_categories;
DELETE FROM ticket_types;
ALTER SEQUENCE order_number_seq RESTART WITH 1;
COMMIT;
SQL

echo -e "${GREEN}✓ Cleared${NC}\n"
echo -e "${YELLOW}Inserting seed data...${NC}"

docker exec -i "$CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" <<'SQL'
BEGIN;

-- ============================================
-- Ticket Types
-- ============================================

-- 1) Pass Journée (Day Pass) - 35€
INSERT INTO ticket_types (id, name, description, price_cents, quantity_total, quantity_sold, sale_start, sale_end, is_active, max_per_order)
VALUES (
    'a0000000-0000-0000-0000-000000000001',
    'Pass Journée',
    'Accès à L''Interfilières pour une journée complète. Inclut l''accès à tous les stands et défilés.',
    3500, 500, 0,
    '2026-01-01 00:00:00+01', '2026-07-14 23:59:59+02',
    true, 10
);

-- 2) Pass 2 Jours (2-Day Pass) - 60€
INSERT INTO ticket_types (id, name, description, price_cents, quantity_total, quantity_sold, sale_start, sale_end, is_active, max_per_order)
VALUES (
    'a0000000-0000-0000-0000-000000000002',
    'Pass 2 Jours',
    'Accès à L''Interfilières sur deux jours consécutifs. Inclut les défilés, stands, et espace networking.',
    6000, 300, 0,
    '2026-01-01 00:00:00+01', '2026-07-14 23:59:59+02',
    true, 5
);

-- 3) Pass VIP - 120€
INSERT INTO ticket_types (id, name, description, price_cents, quantity_total, quantity_sold, sale_start, sale_end, is_active, max_per_order)
VALUES (
    'a0000000-0000-0000-0000-000000000003',
    'Pass VIP',
    'Accès VIP illimité : front row défilés, lounge privé, cocktail de bienvenue, gift bag exclusif.',
    12000, 100, 0,
    '2026-01-01 00:00:00+01', '2026-07-14 23:59:59+02',
    true, 2
);

-- 4) Pass Étudiant (Student Pass) - 15€
INSERT INTO ticket_types (id, name, description, price_cents, quantity_total, quantity_sold, sale_start, sale_end, is_active, max_per_order)
VALUES (
    'a0000000-0000-0000-0000-000000000004',
    'Pass Étudiant',
    'Tarif réduit pour étudiants en mode, textile et design. Justificatif demandé à l''entrée.',
    1500, 200, 0,
    '2026-01-01 00:00:00+01', '2026-07-14 23:59:59+02',
    true, 4
);

-- ============================================
-- Categories for "Pass Journée"
-- ============================================
INSERT INTO ticket_categories (id, ticket_type_id, name, quantity_allocated, quantity_sold, allowed_domains) VALUES
    ('b0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000001', 'Professionnels Textile', 200, 0, ARRAY['textile.fr', 'mode.fr', 'fashion.com']),
    ('b0000000-0000-0000-0000-000000000002', 'a0000000-0000-0000-0000-000000000001', 'Presse & Média',        100, 0, ARRAY['lemonde.fr', 'vogue.fr', 'elle.fr', 'media.com']),
    ('b0000000-0000-0000-0000-000000000003', 'a0000000-0000-0000-0000-000000000001', 'Grand Public',          200, 0, '{}');

-- Categories for "Pass Étudiant"
INSERT INTO ticket_categories (id, ticket_type_id, name, quantity_allocated, quantity_sold, allowed_domains) VALUES
    ('b0000000-0000-0000-0000-000000000004', 'a0000000-0000-0000-0000-000000000004', 'Écoles de Mode',   100, 0, ARRAY['esmod.com', 'ifm-paris.com', 'lisaa.com', 'studio-bercot.com']),
    ('b0000000-0000-0000-0000-000000000005', 'a0000000-0000-0000-0000-000000000004', 'Universités',      100, 0, ARRAY['univ-paris.fr', 'sorbonne.fr', 'edu.fr']);

-- ============================================
-- Mock Orders & Tickets
-- ============================================

-- Order 1: Marie Dupont - 2x Pass Journée (confirmed)
INSERT INTO orders (id, order_number, customer_email, customer_first_name, customer_last_name, customer_phone, total_cents, status, ip_address, created_at, paid_at, confirmed_at)
VALUES (
    'c0000000-0000-0000-0000-000000000001',
    'IF-2026-00001',
    'marie.dupont@textile.fr', 'Marie', 'Dupont', '+33612345678',
    7000, 'confirmed', '192.168.1.10',
    '2026-02-10 14:30:00+01', '2026-02-10 14:32:00+01', '2026-02-10 14:32:05+01'
);
SELECT setval('order_number_seq', 1);

INSERT INTO tickets (id, order_id, ticket_type_id, qr_token, attendee_first_name, attendee_last_name, created_at) VALUES
    ('d0000000-0000-0000-0000-000000000001', 'c0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000001', 'MOCK-QR-001-marie-dupont-a', 'Marie', 'Dupont', '2026-02-10 14:32:05+01'),
    ('d0000000-0000-0000-0000-000000000002', 'c0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000001', 'MOCK-QR-002-marie-dupont-b', 'Pierre', 'Dupont', '2026-02-10 14:32:05+01');

-- Order 2: Jean Martin - 1x Pass 2 Jours (confirmed)
INSERT INTO orders (id, order_number, customer_email, customer_first_name, customer_last_name, customer_phone, total_cents, status, ip_address, created_at, paid_at, confirmed_at)
VALUES (
    'c0000000-0000-0000-0000-000000000002',
    'IF-2026-00002',
    'jean.martin@mode.fr', 'Jean', 'Martin', '+33698765432',
    6000, 'confirmed', '192.168.1.20',
    '2026-02-12 09:15:00+01', '2026-02-12 09:17:00+01', '2026-02-12 09:17:05+01'
);
SELECT setval('order_number_seq', 2);

INSERT INTO tickets (id, order_id, ticket_type_id, qr_token, attendee_first_name, attendee_last_name, created_at) VALUES
    ('d0000000-0000-0000-0000-000000000003', 'c0000000-0000-0000-0000-000000000002', 'a0000000-0000-0000-0000-000000000002', 'MOCK-QR-003-jean-martin', 'Jean', 'Martin', '2026-02-12 09:17:05+01');

-- Order 3: Sophie Leblanc - 1x Pass VIP (confirmed, already validated)
INSERT INTO orders (id, order_number, customer_email, customer_first_name, customer_last_name, total_cents, status, ip_address, created_at, paid_at, confirmed_at)
VALUES (
    'c0000000-0000-0000-0000-000000000003',
    'IF-2026-00003',
    'sophie.leblanc@vogue.fr', 'Sophie', 'Leblanc',
    12000, 'confirmed', '10.0.0.5',
    '2026-02-15 11:00:00+01', '2026-02-15 11:02:00+01', '2026-02-15 11:02:10+01'
);
SELECT setval('order_number_seq', 3);

INSERT INTO tickets (id, order_id, ticket_type_id, qr_token, attendee_first_name, attendee_last_name, is_validated, validated_at, validated_by, created_at) VALUES
    ('d0000000-0000-0000-0000-000000000004', 'c0000000-0000-0000-0000-000000000003', 'a0000000-0000-0000-0000-000000000003', 'MOCK-QR-004-sophie-leblanc', 'Sophie', 'Leblanc', true, '2026-02-20 10:05:00+01', 'admin', '2026-02-15 11:02:10+01');

-- Order 4: Lucas Bernard - 3x Pass Étudiant (confirmed)
INSERT INTO orders (id, order_number, customer_email, customer_first_name, customer_last_name, total_cents, status, ip_address, created_at, paid_at, confirmed_at)
VALUES (
    'c0000000-0000-0000-0000-000000000004',
    'IF-2026-00004',
    'lucas.bernard@esmod.com', 'Lucas', 'Bernard',
    4500, 'confirmed', '172.16.0.50',
    '2026-02-18 16:45:00+01', '2026-02-18 16:47:00+01', '2026-02-18 16:47:05+01'
);
SELECT setval('order_number_seq', 4);

INSERT INTO tickets (id, order_id, ticket_type_id, qr_token, attendee_first_name, attendee_last_name, created_at) VALUES
    ('d0000000-0000-0000-0000-000000000005', 'c0000000-0000-0000-0000-000000000004', 'a0000000-0000-0000-0000-000000000004', 'MOCK-QR-005-lucas-bernard-a', 'Lucas',    'Bernard', '2026-02-18 16:47:05+01'),
    ('d0000000-0000-0000-0000-000000000006', 'c0000000-0000-0000-0000-000000000004', 'a0000000-0000-0000-0000-000000000004', 'MOCK-QR-006-lucas-bernard-b', 'Emma',     'Bernard', '2026-02-18 16:47:05+01'),
    ('d0000000-0000-0000-0000-000000000007', 'c0000000-0000-0000-0000-000000000004', 'a0000000-0000-0000-0000-000000000004', 'MOCK-QR-007-lucas-bernard-c', 'Chloé',    'Moreau',  '2026-02-18 16:47:05+01');

-- Order 5: Amélie Petit - 1x Pass Journée (pending - not yet paid)
INSERT INTO orders (id, order_number, customer_email, customer_first_name, customer_last_name, total_cents, status, ip_address, created_at)
VALUES (
    'c0000000-0000-0000-0000-000000000005',
    'IF-2026-00005',
    'amelie.petit@gmail.com', 'Amélie', 'Petit',
    3500, 'pending', '192.168.1.100',
    '2026-02-20 08:00:00+01'
);
SELECT setval('order_number_seq', 5);

INSERT INTO tickets (id, order_id, ticket_type_id, qr_token, attendee_first_name, attendee_last_name, created_at) VALUES
    ('d0000000-0000-0000-0000-000000000008', 'c0000000-0000-0000-0000-000000000005', 'a0000000-0000-0000-0000-000000000001', 'MOCK-QR-008-amelie-petit', 'Amélie', 'Petit', '2026-02-20 08:00:00+01');

-- Order 6: Thomas Roux - 2x Pass 2 Jours (cancelled)
INSERT INTO orders (id, order_number, customer_email, customer_first_name, customer_last_name, customer_phone, total_cents, status, ip_address, created_at)
VALUES (
    'c0000000-0000-0000-0000-000000000006',
    'IF-2026-00006',
    'thomas.roux@fashion.com', 'Thomas', 'Roux', '+33611223344',
    12000, 'cancelled', '10.10.10.10',
    '2026-02-22 19:30:00+01'
);
SELECT setval('order_number_seq', 6);

-- Order 7: Claire Moreau - 1x Pass VIP + 2x Pass Journée (confirmed)
INSERT INTO orders (id, order_number, customer_email, customer_first_name, customer_last_name, customer_phone, total_cents, status, ip_address, created_at, paid_at, confirmed_at)
VALUES (
    'c0000000-0000-0000-0000-000000000007',
    'IF-2026-00007',
    'claire.moreau@lemonde.fr', 'Claire', 'Moreau', '+33699887766',
    19000, 'confirmed', '192.168.2.30',
    '2026-02-25 10:20:00+01', '2026-02-25 10:22:00+01', '2026-02-25 10:22:10+01'
);
SELECT setval('order_number_seq', 7);

INSERT INTO tickets (id, order_id, ticket_type_id, qr_token, attendee_first_name, attendee_last_name, created_at) VALUES
    ('d0000000-0000-0000-0000-000000000009', 'c0000000-0000-0000-0000-000000000007', 'a0000000-0000-0000-0000-000000000003', 'MOCK-QR-009-claire-moreau-vip', 'Claire',   'Moreau', '2026-02-25 10:22:10+01'),
    ('d0000000-0000-0000-0000-000000000010', 'c0000000-0000-0000-0000-000000000007', 'a0000000-0000-0000-0000-000000000001', 'MOCK-QR-010-claire-moreau-a',   'Antoine',  'Moreau', '2026-02-25 10:22:10+01'),
    ('d0000000-0000-0000-0000-000000000011', 'c0000000-0000-0000-0000-000000000007', 'a0000000-0000-0000-0000-000000000001', 'MOCK-QR-011-claire-moreau-b',   'Isabelle', 'Duval',  '2026-02-25 10:22:10+01');

-- Order 8: Hugo Lefevre - 1x Pass Journée (paid, not yet confirmed)
INSERT INTO orders (id, order_number, customer_email, customer_first_name, customer_last_name, total_cents, status, ip_address, created_at, paid_at)
VALUES (
    'c0000000-0000-0000-0000-000000000008',
    'IF-2026-00008',
    'hugo.lefevre@gmail.com', 'Hugo', 'Lefevre',
    3500, 'paid', '172.16.5.5',
    '2026-03-01 15:00:00+01', '2026-03-01 15:02:00+01'
);
SELECT setval('order_number_seq', 8);

INSERT INTO tickets (id, order_id, ticket_type_id, qr_token, attendee_first_name, attendee_last_name, created_at) VALUES
    ('d0000000-0000-0000-0000-000000000012', 'c0000000-0000-0000-0000-000000000008', 'a0000000-0000-0000-0000-000000000001', 'MOCK-QR-012-hugo-lefevre', 'Hugo', 'Lefevre', '2026-03-01 15:02:00+01');

-- ============================================
-- Update sold counts to match inserted tickets
-- ============================================

-- Pass Journée: orders 1 (2 tickets), 5 (1 pending), 7 (2 tickets), 8 (1 paid) = 6 confirmed/paid
UPDATE ticket_types SET quantity_sold = 6 WHERE id = 'a0000000-0000-0000-0000-000000000001';
-- Pass 2 Jours: order 2 (1 ticket)
UPDATE ticket_types SET quantity_sold = 1 WHERE id = 'a0000000-0000-0000-0000-000000000002';
-- Pass VIP: order 3 (1 ticket), order 7 (1 ticket) = 2
UPDATE ticket_types SET quantity_sold = 2 WHERE id = 'a0000000-0000-0000-0000-000000000003';
-- Pass Étudiant: order 4 (3 tickets)
UPDATE ticket_types SET quantity_sold = 3 WHERE id = 'a0000000-0000-0000-0000-000000000004';

-- Category sold counts (based on customer email domains)
-- marie.dupont@textile.fr → Professionnels Textile: 2
UPDATE ticket_categories SET quantity_sold = 2 WHERE id = 'b0000000-0000-0000-0000-000000000001';
-- sophie.leblanc@vogue.fr → Presse & Média: counted via VIP (no category on VIP)
-- claire.moreau@lemonde.fr → Presse & Média: 2 (her 2 day passes)
UPDATE ticket_categories SET quantity_sold = 2 WHERE id = 'b0000000-0000-0000-0000-000000000002';
-- Grand Public: hugo + amélie = 2
UPDATE ticket_categories SET quantity_sold = 2 WHERE id = 'b0000000-0000-0000-0000-000000000003';
-- lucas.bernard@esmod.com → Écoles de Mode: 3
UPDATE ticket_categories SET quantity_sold = 3 WHERE id = 'b0000000-0000-0000-0000-000000000004';

-- Refresh stats
REFRESH MATERIALIZED VIEW sales_stats;

COMMIT;
SQL

echo -e "${GREEN}✓ Database seeded successfully!${NC}\n"
echo -e "${CYAN}  Ticket Types:${NC}"
echo -e "    • Pass Journée     (35€)  — 500 total, 6 sold"
echo -e "    • Pass 2 Jours     (60€)  — 300 total, 1 sold"
echo -e "    • Pass VIP        (120€)  — 100 total, 2 sold"
echo -e "    • Pass Étudiant    (15€)  — 200 total, 3 sold"
echo -e ""
echo -e "${CYAN}  Categories:${NC}"
echo -e "    • Pass Journée → Professionnels Textile (200), Presse & Média (100), Grand Public (200)"
echo -e "    • Pass Étudiant → Écoles de Mode (100), Universités (100)"
echo -e ""
echo -e "${CYAN}  Orders:${NC}"
echo -e "    • IF-2026-00001  Marie Dupont     2x Journée       confirmed"
echo -e "    • IF-2026-00002  Jean Martin      1x 2 Jours       confirmed"
echo -e "    • IF-2026-00003  Sophie Leblanc   1x VIP           confirmed (validated)"
echo -e "    • IF-2026-00004  Lucas Bernard    3x Étudiant      confirmed"
echo -e "    • IF-2026-00005  Amélie Petit     1x Journée       pending"
echo -e "    • IF-2026-00006  Thomas Roux      2x 2 Jours       cancelled"
echo -e "    • IF-2026-00007  Claire Moreau    1x VIP + 2x Jour confirmed"
echo -e "    • IF-2026-00008  Hugo Lefevre     1x Journée       paid"
echo -e ""
echo -e "${CYAN}  Total: 8 orders, 12 tickets, revenue: 67,500 centimes (675€)${NC}"
