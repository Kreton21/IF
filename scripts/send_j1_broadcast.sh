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
STATUS_URL="${ENDPOINT}/status"

echo "  → Lancement du broadcast..."

# Fire the POST — returns immediately with {"started":true}
HTTP_CODE=$(curl -sS -m 30 \
  -X POST "$ENDPOINT" \
  -H "X-Broadcast-Key: $KEY" \
  -H "Content-Type: application/json" \
  -d "$BODY" \
  -o "$RAW_OUT" \
  -w "%{http_code}" 2>"$CURL_ERR")

if [[ "$HTTP_CODE" != "202" ]]; then
  echo "❌  Le serveur a refusé la requête (HTTP $HTTP_CODE)"
  cat "$RAW_OUT"
  [[ -s "$CURL_ERR" ]] && echo "$(cat "$CURL_ERR")"
  rm -f "$RAW_OUT" "$CURL_ERR"
  exit 1
fi

echo "  ✔  Broadcast démarré sur le serveur."
echo ""

# Poll /status every second until running=false
BAR_WIDTH=40
START_TIME=$(date +%s)
FINAL_SENT=0
FINAL_FAILED=0
DONE_FILE=$(mktemp)

while true; do
  STATUS=$(curl -sS -m 5 \
    -H "X-Broadcast-Key: $KEY" \
    "$STATUS_URL" 2>/dev/null)

  if [[ -z "$STATUS" ]]; then
    sleep 1
    continue
  fi

  python3 - "$STATUS" "$START_TIME" "$BAR_WIDTH" "$DONE_FILE" << 'PYEOF'
import sys, json, time

raw, start_ts, bar_width, done_file = sys.argv[1], int(sys.argv[2]), int(sys.argv[3]), sys.argv[4]
try:
    d = json.loads(raw)
except Exception:
    sys.exit(0)

running = d.get("running", False)
sent    = d.get("sent",    0)
failed  = d.get("failed",  0)
total   = d.get("total",   0)

elapsed = time.time() - start_ts
rate    = (sent + failed) / elapsed if elapsed > 0 else 0

if total > 0:
    pct        = (sent + failed) / total
    done_chars = int(bar_width * pct)
    bar        = "█" * done_chars + "░" * (bar_width - done_chars)
    eta        = (total - sent - failed) / rate if rate > 0 and running else 0
    eta_str    = f"  ETA {eta:.0f}s" if eta > 0 else ""
    fail_str   = f"  ⚠️ {failed} échec(s)" if failed > 0 else ""
    print(f"\r  [{bar}] {sent+failed}/{total}  {rate:.1f}/s{eta_str}{fail_str}   ", end="", flush=True)
else:
    print(f"\r  ⠸ En attente du démarrage...   ", end="", flush=True)

if not running and total > 0:
    open(done_file, "w").write(f"{sent}:{failed}:{total}")
PYEOF

  if [[ -s "$DONE_FILE" ]]; then
    IFS=: read -r FINAL_SENT FINAL_FAILED FINAL_TOTAL < "$DONE_FILE"
    break
  fi

  sleep 1
done

printf "\n\n"
echo "  ✅  Envoyés  : $FINAL_SENT / $FINAL_TOTAL"
[[ "$FINAL_FAILED" -gt 0 ]] && echo "  ❌  Échoués  : $FINAL_FAILED"

rm -f "$RAW_OUT" "$CURL_ERR" "$DONE_FILE"

