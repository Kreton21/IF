#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  ./scripts/import_db_dump.sh -f <file.dump> [options]

Options:
  -f, --file <file.dump>     Dump file to import (required)
      --db-url <url>         PostgreSQL connection URL (default: DATABASE_URL env)
      --no-clean             Do not drop existing objects before restore
  -y, --yes                  Skip confirmation prompt
  -h, --help                 Show this help

Examples:
  DATABASE_URL='postgres://user:pass@localhost:5432/db?sslmode=disable' ./scripts/import_db_dump.sh -f /tmp/festival.dump
  ./scripts/import_db_dump.sh -f ./backup.dump --db-url 'postgres://user:pass@host:5432/db?sslmode=disable' -y
EOF
}

DUMP_FILE=""
DB_URL="${DATABASE_URL:-}"
ASSUME_YES=0
CLEAN_ARGS=(--clean --if-exists)

while [[ $# -gt 0 ]]; do
  case "$1" in
    -f|--file)
      DUMP_FILE="${2:-}"
      shift 2
      ;;
    --db-url)
      DB_URL="${2:-}"
      shift 2
      ;;
    --no-clean)
      CLEAN_ARGS=()
      shift
      ;;
    -y|--yes)
      ASSUME_YES=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "❌ Unknown argument: $1"
      usage
      exit 1
      ;;
  esac
done

if [[ -z "$DUMP_FILE" ]]; then
  echo "❌ Missing required --file option"
  usage
  exit 1
fi

if [[ ! -f "$DUMP_FILE" ]]; then
  echo "❌ Dump file not found: $DUMP_FILE"
  exit 1
fi

if [[ -z "$DB_URL" ]]; then
  echo "❌ DATABASE_URL is not set."
  echo "   Use --db-url or export DATABASE_URL."
  exit 1
fi

if ! command -v pg_restore >/dev/null 2>&1; then
  echo "❌ pg_restore is not installed or not in PATH"
  exit 1
fi

if ! command -v psql >/dev/null 2>&1; then
  echo "❌ psql is not installed or not in PATH"
  exit 1
fi

echo "🔎 Checking database connectivity..."
psql "$DB_URL" -v ON_ERROR_STOP=1 -c 'SELECT 1;' >/dev/null

if [[ "$ASSUME_YES" -ne 1 ]]; then
  echo "⚠️  This will import '$DUMP_FILE' into the target database."
  if [[ ${#CLEAN_ARGS[@]} -gt 0 ]]; then
    echo "    Existing objects will be dropped/replaced (--clean --if-exists)."
  fi
  read -r -p "Continue? (y/N) " reply
  if [[ ! "$reply" =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 0
  fi
fi

echo "📥 Importing dump: $DUMP_FILE"
pg_restore -d "$DB_URL" -v --no-owner --no-privileges "${CLEAN_ARGS[@]}" "$DUMP_FILE"

echo "✅ Import completed"
