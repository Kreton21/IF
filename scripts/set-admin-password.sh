#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BACKEND_DIR="$PROJECT_DIR/backend"

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
    # shellcheck disable=SC1090
    source "$BACKEND_DIR/.env"
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

HASHED_PASSWORD="$(cd "$BACKEND_DIR" && PASSWORD="$NEW_PASSWORD" go run /dev/stdin <<'GOEOF'
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
)"

UPDATED_ROWS="$(psql "$DATABASE_URL" -t -A -v ON_ERROR_STOP=1 -v username="$USERNAME" -v password_hash="$HASHED_PASSWORD" <<'SQLEOF'
UPDATE admins
SET password_hash = :'password_hash'
WHERE username = :'username' AND is_active = true;
SELECT COUNT(*) FROM admins WHERE username = :'username' AND is_active = true;
SQLEOF
)"

ACTIVE_COUNT="$(echo "$UPDATED_ROWS" | tail -n1)"
if [[ "$ACTIVE_COUNT" == "0" ]]; then
  echo "Error: active admin '$USERNAME' not found."
  exit 1
fi

echo "Password updated successfully for admin '$USERNAME'."
