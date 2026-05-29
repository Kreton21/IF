#!/usr/bin/env bash
# ============================================================
#  send_j1_broadcast.sh
#  Envoie l'email J-1 (MailJ-2) avec billet PDF à tous les
#  participants confirmés.
#
#  Usage:
#    ./send_j1_broadcast.sh                          # utilise les vars d'env
#    BROADCAST_API_KEY=xxxx API_BASE=https://... ./send_j1_broadcast.sh
#
#  Variables d'environnement :
#    API_BASE         URL de base de l'API  (défaut: http://localhost:8080)
#    BROADCAST_API_KEY  Clé secrète           (obligatoire)
# ============================================================
set -euo pipefail

API_BASE="${API_BASE:-http://localhost:8080}"
KEY="${BROADCAST_API_KEY:-}"

if [[ -z "$KEY" ]]; then
  echo "❌  BROADCAST_API_KEY est vide. Définissez cette variable et relancez."
  exit 1
fi

ENDPOINT="${API_BASE}/api/v1/broadcast/j1"

echo "🚀  Envoi du broadcast J-1..."
echo "    Endpoint : $ENDPOINT"
echo ""

RESPONSE=$(curl -s -w "\nHTTP_STATUS:%{http_code}" \
  -X POST "$ENDPOINT" \
  -H "X-Broadcast-Key: $KEY" \
  -H "Content-Type: application/json")

BODY=$(echo "$RESPONSE" | sed -n '1{p}')
HTTP_STATUS=$(echo "$RESPONSE" | grep "HTTP_STATUS:" | cut -d: -f2)

echo "    Statut HTTP : $HTTP_STATUS"
echo "    Réponse     : $BODY"
echo ""

if [[ "$HTTP_STATUS" == "200" ]]; then
  SENT=$(echo "$BODY" | grep -o '"sent":[0-9]*' | cut -d: -f2)
  FAILED=$(echo "$BODY" | grep -o '"failed":[0-9]*' | cut -d: -f2)
  echo "✅  Terminé — Envoyés : ${SENT:-?}  |  Échoués : ${FAILED:-?}"
else
  echo "❌  Échec (HTTP $HTTP_STATUS)"
  exit 1
fi
