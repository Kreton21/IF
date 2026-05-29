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

# Fire the POST in background
curl -sS -m 600 \
  -X POST "$ENDPOINT" \
  -H "X-Broadcast-Key: $KEY" \
  -H "Content-Type: application/json" \
  -d "$BODY" \
  -o "$RAW_OUT" 2>"$CURL_ERR" &
CURL_PID=$!

# Give the server a moment to start
sleep 1

# Poll status endpoint while POST is running
BAR_WIDTH=40
START_TIME=$(date +%s)

while kill -0 "$CURL_PID" 2>/dev/null; do
  STATUS=$(curl -sS -m 3 \
    -H "X-Broadcast-Key: $KEY" \
    "$STATUS_URL" 2>/dev/null)

  if [[ -n "$STATUS" ]]; then
    python3 - "$STATUS" "$START_TIME" "$BAR_WIDTH" << 'PYEOF'
import sys, json, time

raw, start_ts, bar_width = sys.argv[1], int(sys.argv[2]), int(sys.argv[3])
try:
    d = json.loads(raw)
except Exception:
    sys.exit(0)
sent   = d.get("sent",   0)
failed = d.get("failed", 0)
total  = d.get("total",  0)
elapsed = time.time() - start_ts
rate    = (sent + failed) / elapsed if elapsed > 0 else 0
if total > 0:
    pct       = (sent + failed) / total
    done_chars = int(bar_width * pct)
    bar = "█" * done_chars + "░" * (bar_width - done_chars)
    eta = (total - sent - failed) / rate if rate > 0 else 0
    eta_str = f"  ETA {eta:.0f}s" if eta > 0 else ""
    fail_str = f"  ⚠️ {failed} échec(s)" if failed > 0 else ""
    print(f"\r  [{bar}] {sent+failed}/{total}  {rate:.1f}/s{eta_str}{fail_str}   ", end="", flush=True)
else:
    print(f"\r  ⠸ En attente du démarrage...   ", end="", flush=True)
PYEOF
  fi
  sleep 1
done

wait "$CURL_PID"
CURL_CODE=$?
printf "\n"

# Handle curl error
if [[ "$CURL_CODE" -ne 0 && "$CURL_CODE" -ne 18 ]]; then
  echo "❌  curl a échoué (code $CURL_CODE)"
  [[ -s "$CURL_ERR" ]] && echo "    $(cat "$CURL_ERR")"
  rm -f "$RAW_OUT" "$CURL_ERR"
  exit "$CURL_CODE"
fi

# Parse final result from response body
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

print("⚠️  Réponse reçue mais sans confirmation :")
for l in lines[-5:]:
    print(f"    {l}")
sys.exit(1)
PYEOF

rm -f "$RAW_OUT" "$CURL_ERR"

