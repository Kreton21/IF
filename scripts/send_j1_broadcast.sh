#!/usr/bin/env bash
# ============================================================
#  send_j1_broadcast.sh
#  Envoie l'email J-1 (MailJ-2) avec billet PDF.
#
#  Usage:
#    ./send_j1_broadcast.sh                          # envoie à tout le monde
#    ./send_j1_broadcast.sh -u test@example.com      # test : un seul email
#
#  Variables d'environnement :
#    API_BASE           URL de base de l'API  (défaut: http://localhost:8080)
#    BROADCAST_API_KEY  Clé secrète           (obligatoire)
# ============================================================
set -euo pipefail

API_BASE="${API_BASE:-http://localhost:8080}"
KEY="${BROADCAST_API_KEY:-}"
TARGET_EMAIL=""

# Parse arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    -u|--user)
      TARGET_EMAIL="${2:-}"
      shift 2
      ;;
    *)
      echo "Usage: $0 [-u email@example.com]"
      exit 1
      ;;
  esac
done

if [[ -z "$KEY" ]]; then
  echo "❌  BROADCAST_API_KEY est vide. Définissez cette variable et relancez."
  exit 1
fi

ENDPOINT="${API_BASE}/api/v1/broadcast/j1"

if [[ -n "$TARGET_EMAIL" ]]; then
  echo "🧪  Mode test — envoi uniquement à : $TARGET_EMAIL"
  BODY="{\"target_email\":\"${TARGET_EMAIL}\"}"
else
  echo "🚀  Envoi du broadcast J-1 à tous les participants..."
  BODY="{}"
fi

echo "    Endpoint : $ENDPOINT"
echo ""

RESPONSE=$(curl -s -w "\nHTTP_STATUS:%{http_code}" \
  -X POST "$ENDPOINT" \
  -H "X-Broadcast-Key: $KEY" \
  -H "Content-Type: application/json" \
  -d "$BODY")

BODY_OUT=$(echo "$RESPONSE" | head -n1)
HTTP_STATUS=$(echo "$RESPONSE" | grep "HTTP_STATUS:" | cut -d: -f2)

echo "    Statut HTTP : $HTTP_STATUS"
echo "    Réponse     : $BODY_OUT"
echo ""

if [[ "$HTTP_STATUS" == "200" ]]; then
  SENT=$(echo "$BODY_OUT" | grep -o '"sent":[0-9]*' | cut -d: -f2)
  FAILED=$(echo "$BODY_OUT" | grep -o '"failed":[0-9]*' | cut -d: -f2)
  echo "✅  Terminé — Envoyés : ${SENT:-?}  |  Échoués : ${FAILED:-?}"
else
  echo "❌  Échec (HTTP $HTTP_STATUS)"
  exit 1
fi
