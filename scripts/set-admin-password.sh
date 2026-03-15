#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BACKEND_DIR="$PROJECT_DIR/backend"
SERVICE_FILE="${SERVICE_FILE:-/etc/systemd/system/festival.service}"
SERVICE_NAME="${SERVICE_NAME:-festival.service}"

usage() {
  echo "Usage: $0 <username> <new_password>"
  echo "Example: $0 admin NewStrongPass123!"
}

if [[ $# -ne 2 ]]; then
  usage
  exit 1
fi

USERNAME="$1"
NEW_PASSWORD="$2"

if [[ ${#NEW_PASSWORD} -lt 8 ]]; then
  echo "Error: password must be at least 8 characters."
  exit 1
fi

if [[ -z "${DATABASE_URL:-}" ]]; then
  if [[ -f "$BACKEND_DIR/.env" ]]; then
    DATABASE_URL="$(grep -E '^DATABASE_URL=' "$BACKEND_DIR/.env" | head -n1 | cut -d'=' -f2- || true)"
    REDIS_URL="${REDIS_URL:-$(grep -E '^REDIS_URL=' "$BACKEND_DIR/.env" | head -n1 | cut -d'=' -f2- || true)}"
  fi
fi

if [[ -z "${DATABASE_URL:-}" && -r "$SERVICE_FILE" ]]; then
  DATABASE_URL="$(grep -E 'Environment="?DATABASE_URL=' "$SERVICE_FILE" | head -n1 | sed -E 's/.*Environment="?DATABASE_URL=([^" ]+)"?.*/\1/' || true)"
  REDIS_URL="${REDIS_URL:-$(grep -E 'Environment="?REDIS_URL=' "$SERVICE_FILE" | head -n1 | sed -E 's/.*Environment="?REDIS_URL=([^" ]+)"?.*/\1/' || true)}"

  ENV_FILE_PATH="$(grep -E '^EnvironmentFile=' "$SERVICE_FILE" | head -n1 | cut -d'=' -f2- || true)"
  if [[ -n "$ENV_FILE_PATH" && -r "$ENV_FILE_PATH" ]]; then
    if [[ -z "${DATABASE_URL:-}" ]]; then
      DATABASE_URL="$(grep -E '^DATABASE_URL=' "$ENV_FILE_PATH" | head -n1 | cut -d'=' -f2- || true)"
    fi
    if [[ -z "${REDIS_URL:-}" ]]; then
      REDIS_URL="$(grep -E '^REDIS_URL=' "$ENV_FILE_PATH" | head -n1 | cut -d'=' -f2- || true)"
    fi
  fi
fi

if [[ -z "${DATABASE_URL:-}" ]] && command -v systemctl >/dev/null 2>&1; then
  SYSTEMD_ENV="$(systemctl show "$SERVICE_NAME" --property=Environment --value 2>/dev/null || true)"
  if [[ -n "$SYSTEMD_ENV" ]]; then
    for kv in $SYSTEMD_ENV; do
      case "$kv" in
        DATABASE_URL=*) DATABASE_URL="${kv#DATABASE_URL=}" ;;
        REDIS_URL=*) REDIS_URL="${REDIS_URL:-${kv#REDIS_URL=}}" ;;
      esac
    done
  fi
fi

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "Error: DATABASE_URL is not set. Export it or set it in backend/.env"
  exit 1
fi

if ! command -v psql >/dev/null 2>&1; then
  echo "Error: psql is required but not installed."
  exit 1
fi

if ! command -v go >/dev/null 2>&1; then
  echo "Error: go is required but not installed."
  exit 1
fi

mkdir -p "$BACKEND_DIR/tmp"
TMP_HASH_FILE="$(mktemp "$BACKEND_DIR/tmp/hash-password.XXXXXX.go")"
cleanup() {
  rm -f "$TMP_HASH_FILE"
}
trap cleanup EXIT

cat > "$TMP_HASH_FILE" <<'GOEOF'
package main

import (
  "fmt"
  "log"
  "os"

  "golang.org/x/crypto/bcrypt"
)

func main() {
  password := os.Getenv("PASSWORD")
  hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
  if err != nil {
    log.Fatal(err)
  }
  fmt.Print(string(hash))
}
GOEOF

HASHED_PASSWORD="$(cd "$BACKEND_DIR" && PASSWORD="$NEW_PASSWORD" go run "$TMP_HASH_FILE")"

UPDATED_ROWS="$(psql "$DATABASE_URL" -t -A -v ON_ERROR_STOP=1 -v username="$USERNAME" -v password_hash="$HASHED_PASSWORD" <<'SQLEOF'
UPDATE admins
SET password_hash = :'password_hash'
WHERE username = :'username' AND is_active = true;
SELECT COUNT(*) FROM admins WHERE username = :'username' AND is_active = true;
SQLEOF
)"

ACTIVE_COUNT="$(echo "$UPDATED_ROWS" | tail -n1)"
if [[ "$ACTIVE_COUNT" == "0" ]]; then
  echo "Error: active account '$USERNAME' not found."
  exit 1
fi

ADMIN_INFO="$(psql "$DATABASE_URL" -t -A -F '|' -v ON_ERROR_STOP=1 -v username="$USERNAME" <<'SQLEOF'
SELECT id, role FROM admins WHERE username = :'username' AND is_active = true LIMIT 1;
SQLEOF
)"

ADMIN_ID="$(echo "$ADMIN_INFO" | cut -d'|' -f1)"

if [[ -n "${REDIS_URL:-}" && -n "$ADMIN_ID" ]] && command -v redis-cli >/dev/null 2>&1; then
  NOW_TS="$(date +%s)"
  REDIS_URL="$REDIS_URL" redis-cli SET "auth:password_changed_at:${ADMIN_ID}" "$NOW_TS" >/dev/null || true
fi

echo "Password updated successfully for '$USERNAME'."
