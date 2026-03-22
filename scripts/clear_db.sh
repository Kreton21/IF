#!/bin/bash
# Clear all data from the IF Festival database
# Usage: ./scripts/clear_db.sh

set -euo pipefail

CONTAINER="if-festival-postgres"
DB_NAME="festival_db"
DB_USER="festival"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${YELLOW}⚠  This will DELETE ALL DATA from the festival database.${NC}"
echo -e "${YELLOW}   Tables affected: orders/tickets, navettes, parrainage, webhook_logs, catégories/types${NC}"
echo -e "${YELLOW}   The admins table will be preserved.${NC}"
echo ""
read -p "Are you sure? (y/N) " -n 1 -r
echo ""

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 0
fi

echo -e "\n${YELLOW}Clearing database...${NC}"

docker exec -i "$CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" <<'SQL'
BEGIN;

-- Delete in order respecting foreign key constraints
DELETE FROM webhook_logs;
DELETE FROM referral_order_conversions;
DELETE FROM referral_clicks;
DELETE FROM referral_links;
DELETE FROM bus_tickets;
DELETE FROM bus_order_rides;
DELETE FROM order_items;
DELETE FROM tickets;
DELETE FROM orders;
DELETE FROM bus_departures;
DELETE FROM bus_stations;
DELETE FROM ticket_categories;
DELETE FROM ticket_types;

-- Reset order number sequence
ALTER SEQUENCE order_number_seq RESTART WITH 1;

-- Refresh materialized view
REFRESH MATERIALIZED VIEW sales_stats;

COMMIT;
SQL

echo -e "${GREEN}✓ Database cleared successfully.${NC}"
echo -e "${GREEN}  - Tickets/commandes + navettes + parrainage + webhook logs supprimés${NC}"
echo -e "${GREEN}  - Order number sequence reset to 1${NC}"
echo -e "${GREEN}  - Admin accounts preserved${NC}"
