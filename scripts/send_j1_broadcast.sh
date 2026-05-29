#!/usr/bin/env bash
# ============================================================
#  send_j1_broadcast.sh
#  Envoie l'email J-1 (MailJ-2) avec billet PDF.
#  Affiche une barre de progression en temps réel.
#  Envoi en parallèle (5 workers) sans surcharger le système.
#
#  Usage:
#    ./send_j1_broadcast.sh                          # envoie à tout le monde
#    ./send_j1_broadcast.sh -u test@example.com      # test : un seul email
#
#  Variables d'environnement :
#    API_BASE           URL de base de l'API  (défaut: http://localhost:8080)
#    BROADCAST_API_KEY  Clé secrète           (obligatoire)
# ============================================================
set -uo pipefail

API_BASE="${API_BASE:-http://localhost:8080}"
KEY="${BROADCAST_API_KEY:-}"
TARGET_EMAIL=""

# ── Parse arguments ──────────────────────────────────────────
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
  read -rsp "🔑  BROADCAST_API_KEY : " KEY
  echo ""
  if [[ -z "$KEY" ]]; then
    echo "❌  Clé vide, abandon."
    exit 1
  fi
fi

ENDPOINT="${API_BASE}/api/v1/broadcast/j1"

if [[ -n "$TARGET_EMAIL" ]]; then
  echo "🧪  Mode test — envoi uniquement à : $TARGET_EMAIL"
  BODY="{\"target_email\":\"${TARGET_EMAIL}\"}"
else
  echo "🚀  Envoi du broadcast J-1 à tous les participants..."
  BODY="{}"
fi

echo "    Endpoint  : $ENDPOINT"
echo "    Parallèle : 5 workers"
echo ""

# ── Lancer le broadcast ───────────────────────────────────────
CURL_ERR=$(mktemp)
RAW_OUT=$(mktemp)

echo "  → Envoi en cours (patientez)..."

curl -sS -m 600 \
  -X POST "$ENDPOINT" \
  -H "X-Broadcast-Key: $KEY" \
  -H "Content-Type: application/json" \
  -d "$BODY" \
  -o "$RAW_OUT" \
  -w "%{http_code}" 2>"$CURL_ERR" &
CURL_PID=$!

# Spinner while waiting
SPIN='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
i=0
while kill -0 "$CURL_PID" 2>/dev/null; do
  printf "\r  %s  En attente de la réponse..." "${SPIN:$((i % ${#SPIN})):1}"
  sleep 0.15
  i=$((i+1))
done
printf "\r                                          \r"

wait "$CURL_PID"
CURL_CODE=$?

if [[ "$CURL_CODE" -ne 0 ]]; then
  echo "❌  curl a échoué (code $CURL_CODE)"
  [[ -s "$CURL_ERR" ]] && echo "    $(cat "$CURL_ERR")"
  rm -f "$RAW_OUT" "$CURL_ERR"
  exit "$CURL_CODE"
fi

# Parse result from response body
python3 - "$RAW_OUT" << 'PYEOF'
import sys, json

path = sys.argv[1]
try:
    content = open(path).read().strip()
except Exception as e:
    print(f"❌  Impossible de lire la réponse : {e}")
    sys.exit(1)

if not content:
    print("❌  Aucune donnée reçue du serveur (réponse vide).")
    sys.exit(1)

# Find the "done" line (last NDJSON line with done:true), or try single JSON
done_data = None
error_data = None
lines = [l.strip() for l in content.splitlines() if l.strip()]
for line in reversed(lines):
    try:
        d = json.loads(line)
        if d.get("done"):
            done_data = d
            break
        if "error" in d and "total" not in d:
            error_data = d
            break
    except json.JSONDecodeError:
        pass

if error_data:
    print(f"❌  Erreur serveur : {error_data['error']}")
    sys.exit(1)

if done_data:
    sent   = done_data.get("sent",   0)
    failed = done_data.get("failed", 0)
    print(f"  ✅  Envoyés  : {sent}")
    if failed:
        print(f"  ❌  Échoués  : {failed}")
    err = done_data.get("error")
    if err:
        print(f"  ⚠️  Erreur   : {err}")
        sys.exit(1)
    sys.exit(0)

# No done line found — show raw
print("⚠️  Réponse reçue mais sans confirmation :")
for l in lines[-5:]:
    print(f"    {l}")
sys.exit(1)
PYEOF

rm -f "$RAW_OUT" "$CURL_ERR"

