#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

MIGRATIONS_DIR="${1:-$PROJECT_ROOT/backend/migrations}"
DB_URL="${DATABASE_URL:-}"

if [[ -z "$DB_URL" ]]; then
  echo "❌ DATABASE_URL is not set."
  echo "   Example: export DATABASE_URL='postgres://user:pass@localhost:5432/db?sslmode=disable'"
  exit 1
fi

if [[ ! -d "$MIGRATIONS_DIR" ]]; then
  echo "❌ Migrations directory not found: $MIGRATIONS_DIR"
  exit 1
fi

if ! command -v psql >/dev/null 2>&1; then
  echo "❌ psql is not installed or not in PATH"
  exit 1
fi

if ! command -v sha256sum >/dev/null 2>&1; then
  echo "❌ sha256sum is not installed or not in PATH"
  exit 1
fi

echo "📦 Running SQL migrations from: $MIGRATIONS_DIR"

psql "$DB_URL" -v ON_ERROR_STOP=1 <<'SQL'
CREATE TABLE IF NOT EXISTS schema_migrations (
  filename TEXT PRIMARY KEY,
  checksum TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
SQL

applied=0
skipped=0

shopt -s nullglob
migration_files=("$MIGRATIONS_DIR"/*.sql)
shopt -u nullglob

if [[ ${#migration_files[@]} -eq 0 ]]; then
  echo "ℹ️  No .sql files found in $MIGRATIONS_DIR"
  exit 0
fi

IFS=$'\n' migration_files=($(printf '%s\n' "${migration_files[@]}" | sort))
unset IFS

for file in "${migration_files[@]}"; do
  filename="$(basename "$file")"
  checksum="$(sha256sum "$file" | awk '{print $1}')"

  existing_checksum="$({ psql "$DB_URL" -tA -v ON_ERROR_STOP=1 -c "SELECT checksum FROM schema_migrations WHERE filename = '$filename'"; } | tr -d '[:space:]')"

  if [[ -n "$existing_checksum" ]]; then
    if [[ "$existing_checksum" != "$checksum" ]]; then
      echo "⚠️  Migration already applied but file changed: $filename"
      echo "    Existing checksum: $existing_checksum"
      echo "    Current  checksum: $checksum"
      echo "    Skipping to avoid non-idempotent re-run."
      skipped=$((skipped + 1))
      continue
    fi

    echo "⏭️  Already applied: $filename"
    skipped=$((skipped + 1))
    continue
  fi

  echo "▶️  Applying: $filename"
  psql "$DB_URL" -v ON_ERROR_STOP=1 -f "$file"
  psql "$DB_URL" -v ON_ERROR_STOP=1 -c "INSERT INTO schema_migrations (filename, checksum) VALUES ('$filename', '$checksum')"

  applied=$((applied + 1))
  echo "✅ Applied: $filename"
done

echo ""
echo "Done. Applied: $applied | Skipped: $skipped"
