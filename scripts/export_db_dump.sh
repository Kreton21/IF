#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  ./scripts/export_db_dump.sh [options]

Options:
  -o, --output <file.dump>   Output file path (default: ./backup_YYYYmmdd_HHMMSS.dump)
      --db-url <url>         PostgreSQL connection URL (default: DATABASE_URL env)
  -h, --help                 Show this help

Examples:
  DATABASE_URL='postgres://user:pass@localhost:5432/db?sslmode=disable' ./scripts/export_db_dump.sh
  ./scripts/export_db_dump.sh --db-url 'postgres://user:pass@host:5432/db?sslmode=disable' -o /tmp/festival.dump
EOF
}

OUTPUT_FILE="backup_$(date +%Y%m%d_%H%M%S).dump"
DB_URL="${DATABASE_URL:-}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -o|--output)
      OUTPUT_FILE="${2:-}"
      shift 2
      ;;
    --db-url)
      DB_URL="${2:-}"
      shift 2
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

if [[ -z "$DB_URL" ]]; then
  echo "❌ DATABASE_URL is not set."
  echo "   Use --db-url or export DATABASE_URL."
  exit 1
fi

if ! command -v pg_dump >/dev/null 2>&1; then
  echo "❌ pg_dump is not installed or not in PATH"
  exit 1
fi

mkdir -p "$(dirname "$OUTPUT_FILE")"

echo "📦 Exporting database to: $OUTPUT_FILE"
pg_dump "$DB_URL" -F c -b -v -f "$OUTPUT_FILE"

if command -v sha256sum >/dev/null 2>&1; then
  checksum="$(sha256sum "$OUTPUT_FILE" | awk '{print $1}')"
  echo "🔐 SHA256: $checksum"
fi

echo "✅ Export completed"
