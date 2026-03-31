#!/usr/bin/env bash

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
ADMIN_USER="${ADMIN_USER:-}"
ADMIN_PASS="${ADMIN_PASS:-}"
PAGE_SIZE="${PAGE_SIZE:-100}"
RETRIES="${RETRIES:-3}"
SLEEP_BETWEEN="${SLEEP_BETWEEN:-0.15}"

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

echo "📥 Chargement des commandes paid/confirmed (pagination)..."

declare -A ORDER_IDS_SET=()

fetch_ids_for_status() {
  local status="$1"
  local page=1

  while true; do
    local url="$BASE_URL/api/v1/admin/orders?page=$page&page_size=$PAGE_SIZE&status=$status"
    local resp
    resp=$(curl -sS -w '\n%{http_code}' "$url" \
      -H "Authorization: Bearer $TOKEN" \
      -H "Content-Type: application/json")

    local body code
    body="$(echo "$resp" | sed '$d')"
    code="$(echo "$resp" | tail -n1)"

    if [[ "$code" != "200" ]]; then
      echo "❌ Impossible de lister les commandes status=$status page=$page (HTTP $code)"
      echo "$body"
      exit 1
    fi

    local ids
    ids=$(python3 - <<'PY' "$body"
import json, sys
  try:
    data = json.loads(sys.argv[1])
  except Exception:
    data = {}
  orders = data.get("orders") or []
  if not isinstance(orders, list):
    orders = []
for o in orders:
    if not isinstance(o, dict):
      continue
    oid = o.get("id")
    if oid:
        print(oid)
PY
)

    local count
    count=$(python3 - <<'PY' "$body"
import json, sys
  try:
    data = json.loads(sys.argv[1])
  except Exception:
    print(0)
    raise SystemExit(0)
  orders = data.get("orders") or []
  if not isinstance(orders, list):
    orders = []
  print(len(orders))
PY
)

    if [[ -n "$ids" ]]; then
      while IFS= read -r oid; do
        [[ -n "$oid" ]] && ORDER_IDS_SET["$oid"]=1
      done <<< "$ids"
    fi

    if [[ "$count" -lt "$PAGE_SIZE" ]]; then
      break
    fi
    page=$((page + 1))
  done
}

fetch_ids_for_status "paid"
fetch_ids_for_status "confirmed"

ORDER_IDS=("${!ORDER_IDS_SET[@]}")
TOTAL=${#ORDER_IDS[@]}

if [[ "$TOTAL" -eq 0 ]]; then
  echo "ℹ️ Aucune commande paid/confirmed à traiter."
  exit 0
fi

echo "📧 Renvoi des confirmations pour $TOTAL commande(s)..."

success=0
failed=0

for oid in "${ORDER_IDS[@]}"; do
  ok=false
  for ((attempt=1; attempt<=RETRIES; attempt++)); do
    resp=$(curl -sS -w '\n%{http_code}' -X POST "$BASE_URL/api/v1/admin/orders/$oid/resend-email" \
      -H "Authorization: Bearer $TOKEN" \
      -H "Content-Type: application/json")
    body="$(echo "$resp" | sed '$d')"
    code="$(echo "$resp" | tail -n1)"

    if [[ "$code" == "200" ]]; then
      ok=true
      break
    fi

    if [[ "$attempt" -lt "$RETRIES" ]]; then
      sleep 1
    fi
  done

  if [[ "$ok" == true ]]; then
    success=$((success + 1))
  else
    failed=$((failed + 1))
    echo "⚠️ Échec renvoi commande $oid (dernier HTTP $code)"
    echo "$body"
  fi

  sleep "$SLEEP_BETWEEN"
done

echo ""
echo "✅ Terminé: success=$success failed=$failed total=$TOTAL"

if [[ "$failed" -gt 0 ]]; then
  exit 2
fi
