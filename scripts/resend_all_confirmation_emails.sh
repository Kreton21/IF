#!/usr/bin/env bash

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
ADMIN_USER="${ADMIN_USER:-}"
ADMIN_PASS="${ADMIN_PASS:-}"

if [[ -z "$ADMIN_USER" || -z "$ADMIN_PASS" ]]; then
  echo "❌ ADMIN_USER et ADMIN_PASS sont requis."
  echo "   Exemple: ADMIN_USER=superadmin ADMIN_PASS='xxx' BASE_URL='https://interfilieres.fr' $0"
  exit 1
fi

echo "🔐 Connexion admin sur $BASE_URL..."
LOGIN_PAYLOAD=$(printf '{"username":"%s","password":"%s"}' "$ADMIN_USER" "$ADMIN_PASS")
LOGIN_RESPONSE=$(curl -sS -X POST "$BASE_URL/api/v1/admin/login" \
  -H "Content-Type: application/json" \
  -d "$LOGIN_PAYLOAD")

TOKEN=$(python3 - <<'PY' "$LOGIN_RESPONSE"
import json, sys
try:
    data = json.loads(sys.argv[1])
except Exception:
    print("")
    raise SystemExit(0)
print(data.get("token", ""))
PY
)

if [[ -z "$TOKEN" ]]; then
  echo "❌ Login admin échoué. Réponse:"
  echo "$LOGIN_RESPONSE"
  exit 1
fi

echo "📧 Lancement du renvoi global des confirmations..."
RESEND_RESPONSE=$(curl -sS -X POST "$BASE_URL/api/v1/admin/orders/resend-confirmations" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json")

echo "✅ Réponse API:"
echo "$RESEND_RESPONSE" | python3 -m json.tool 2>/dev/null || echo "$RESEND_RESPONSE"
