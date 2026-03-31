#!/usr/bin/env bash

set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
ORDER_REF=""
ASSUME_YES=false

usage() {
  cat <<EOF
Usage:
  $SCRIPT_NAME                # delete ALL orders
  $SCRIPT_NAME -i IF-2026-00008   # delete one specific order (by order_number or UUID)

Options:
  -i <order_ref>   Order number (e.g. IF-2026-00008) or order UUID
  -y               Skip confirmation prompt
  -h               Show this help

Environment:
  DATABASE_URL must be set (postgres://...)
EOF
}

while getopts ":i:yh" opt; do
  case "$opt" in
    i) ORDER_REF="$OPTARG" ;;
    y) ASSUME_YES=true ;;
    h)
      usage
      exit 0
      ;;
    :)
      echo "❌ Option -$OPTARG requires an argument"
      usage
      exit 1
      ;;
    \?)
      echo "❌ Unknown option: -$OPTARG"
      usage
      exit 1
      ;;
  esac
done

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "❌ DATABASE_URL is not set"
  exit 1
fi

if ! command -v psql >/dev/null 2>&1; then
  echo "❌ psql is not installed or not in PATH"
  exit 1
fi

if [[ -n "$ORDER_REF" ]]; then
  echo "⚠️  This will DELETE order '$ORDER_REF' and its dependent rows (tickets, order_items, bus_order_rides, referral conversions...)."
else
  echo "⚠️  This will DELETE ALL orders and dependent rows."
fi

echo "ℹ️  It will recompute ticket/category/bus sold counters from remaining active orders afterwards."

if [[ "$ASSUME_YES" != true ]]; then
  read -r -p "Continue? (y/N) " reply
  if [[ ! "$reply" =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 0
  fi
fi

run_recompute_sql() {
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
-- Recompute sold counters from remaining active orders
WITH sold_by_type AS (
  SELECT oi.ticket_type_id, COALESCE(SUM(oi.quantity), 0)::int AS sold
  FROM order_items oi
  JOIN orders o ON o.id = oi.order_id
  WHERE o.status IN ('pending', 'paid', 'confirmed')
  GROUP BY oi.ticket_type_id
)
UPDATE ticket_types tt
SET quantity_sold = COALESCE(sbt.sold, 0)
FROM sold_by_type sbt
WHERE tt.id = sbt.ticket_type_id;

UPDATE ticket_types tt
SET quantity_sold = 0
WHERE NOT EXISTS (
  SELECT 1 FROM order_items oi
  JOIN orders o ON o.id = oi.order_id
  WHERE o.status IN ('pending', 'paid', 'confirmed')
    AND oi.ticket_type_id = tt.id
);

WITH sold_by_category AS (
  SELECT oi.category_id, COALESCE(SUM(oi.quantity), 0)::int AS sold
  FROM order_items oi
  JOIN orders o ON o.id = oi.order_id
  WHERE o.status IN ('pending', 'paid', 'confirmed')
    AND oi.category_id IS NOT NULL
  GROUP BY oi.category_id
)
UPDATE ticket_categories tc
SET quantity_sold = COALESCE(sbc.sold, 0)
FROM sold_by_category sbc
WHERE tc.id = sbc.category_id;

UPDATE ticket_categories tc
SET quantity_sold = 0
WHERE NOT EXISTS (
  SELECT 1 FROM order_items oi
  JOIN orders o ON o.id = oi.order_id
  WHERE o.status IN ('pending', 'paid', 'confirmed')
    AND oi.category_id = tc.id
);

WITH sold_by_departure AS (
  SELECT bor.departure_id, COUNT(*)::int AS sold
  FROM bus_order_rides bor
  JOIN orders o ON o.id = bor.order_id
  WHERE o.status IN ('pending', 'paid', 'confirmed')
  GROUP BY bor.departure_id
)
UPDATE bus_departures bd
SET sold = COALESCE(sbd.sold, 0)
FROM sold_by_departure sbd
WHERE bd.id = sbd.departure_id;

UPDATE bus_departures bd
SET sold = 0
WHERE NOT EXISTS (
  SELECT 1 FROM bus_order_rides bor
  JOIN orders o ON o.id = bor.order_id
  WHERE o.status IN ('pending', 'paid', 'confirmed')
    AND bor.departure_id = bd.id
);

REFRESH MATERIALIZED VIEW sales_stats;
SQL
}

if [[ -n "$ORDER_REF" ]]; then
  deleted_count=$(psql "$DATABASE_URL" -tA -v ON_ERROR_STOP=1 -v order_ref="$ORDER_REF" <<'SQL'
WITH target AS (
  SELECT id FROM orders WHERE order_number = :'order_ref' OR id::text = :'order_ref'
), deleted AS (
  DELETE FROM orders o
  USING target t
  WHERE o.id = t.id
  RETURNING o.id
)
SELECT COUNT(*) FROM deleted;
SQL
)
  deleted_count="$(echo "$deleted_count" | tr -d '[:space:]')"

  if [[ "$deleted_count" == "0" ]]; then
    echo "⚠️  No order found for '$ORDER_REF'"
    exit 1
  fi

  run_recompute_sql
  echo "✅ Deleted order '$ORDER_REF' (rows: $deleted_count) and recomputed counters."
else
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
DELETE FROM orders;
SQL

  run_recompute_sql
  echo "✅ Deleted ALL orders and recomputed counters."
fi
