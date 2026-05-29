#!/usr/bin/env bash
# ============================================================
#  send_bus_reminder_broadcast.sh
#  Envoie un email court de rappel navette (horaire + adresse).
#  Usage:
#    ./send_bus_reminder_broadcast.sh
#    ./send_bus_reminder_broadcast.sh -u test@example.com
#    ./send_bus_reminder_broadcast.sh --force
# ============================================================
set -uo pipefail

API_BASE="${API_BASE:-http://localhost:8080}"
KEY="${BROADCAST_API_KEY:-}"
TARGET_EMAIL=""
FORCE="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -u|--user)
      TARGET_EMAIL="${2:-}"
      shift 2
      ;;
    --force)
      FORCE="true"
      shift
      ;;
    *)
      echo "Usage: $0 [-u email@example.com] [--force]"
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

ENDPOINT="${API_BASE}/api/v1/broadcast/bus-reminder"

if [[ -n "$TARGET_EMAIL" ]]; then
  echo "🧪  Mode test — envoi uniquement à : $TARGET_EMAIL"
  BODY="{\"target_email\":\"${TARGET_EMAIL}\",\"force\":${FORCE}}"
else
  echo "🚀  Envoi du broadcast navette (rappel horaire + adresse)..."
  BODY="{\"force\":${FORCE}}"
fi

[[ "$FORCE" == "true" ]] && echo "    ⚠️  Mode --force : ignore la table broadcast_sent"

echo "    Endpoint  : $ENDPOINT"
echo "    Parallèle : 5 workers"
echo ""

CURL_ERR=$(mktemp)
RAW_OUT=$(mktemp)
STATUS_URL="${ENDPOINT}/status"

echo "  → Lancement du broadcast..."
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

if not running and total == 0:
    open(done_file, "w").write("0:0:0")
    print("\n  ℹ️  Aucun email à envoyer (toutes les commandes bus ont déjà reçu le rappel).")
    sys.exit(0)

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
