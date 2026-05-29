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

# ── Barre de progression ─────────────────────────────────────
# Lit le flux NDJSON ligne par ligne et affiche l'avancement.
draw_progress() {
python3 - "$@" << 'PYEOF'
import sys, json, time

BAR_WIDTH = 40
start = time.time()
exit_code = 0
first_line = True

for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    try:
        data = json.loads(line)
    except json.JSONDecodeError:
        if first_line:
            print(f"\n❌  Réponse inattendue du serveur : {line}")
            exit_code = 1
            break
        continue
    first_line = False

    # Server returned an error before streaming (e.g. 401 Unauthorized)
    if "error" in data and "total" not in data and "done" not in data:
        print(f"\n❌  Erreur serveur : {data['error']}")
        exit_code = 1
        break

    sent   = data.get("sent",   0)
    failed = data.get("failed", 0)
    total  = data.get("total",  0)
    done   = data.get("done",   False)

    if total > 0:
        pct  = (sent + failed) / total
        done_chars = int(BAR_WIDTH * pct)
        bar  = "█" * done_chars + "░" * (BAR_WIDTH - done_chars)
    else:
        bar  = "░" * BAR_WIDTH
        pct  = 0

    elapsed = time.time() - start
    rate    = (sent + failed) / elapsed if elapsed > 0 else 0
    remaining = (total - sent - failed) / rate if rate > 0 and not done else 0

    status = f"  ⚠️  {failed} échec(s)" if failed > 0 else ""

    if done:
        elapsed_str = f"{elapsed:.0f}s"
        print(f"\r  [{bar}] {sent+failed}/{total}  ✅ Terminé en {elapsed_str}{status}          ")
        print()
        print(f"  ✅  Envoyés  : {sent}")
        if failed:
            print(f"  ❌  Échoués  : {failed}")
        err = data.get("error")
        if err:
            print(f"  ⚠️  Erreur   : {err}")
            exit_code = 1
        break  # stop reading — don't close stdin abruptly
    else:
        eta_str = f"  ETA {remaining:.0f}s" if remaining > 0 else ""
        print(f"\r  [{bar}] {sent+failed}/{total}  ({pct*100:.0f}%)  {rate:.1f}/s{eta_str}{status}   ", end="", flush=True)

sys.exit(exit_code)
PYEOF
}

# ── Lancer le broadcast et streamer la progression ───────────
RAW_OUT=$(mktemp)
CURL_ERR=$(mktemp)

echo "  → Connexion à l'API..."

# Capture raw output to file AND stream to progress displayer simultaneously
# Use tee so we always have a copy for debugging
curl -sS -N --no-buffer \
  -X POST "$ENDPOINT" \
  -H "X-Broadcast-Key: $KEY" \
  -H "Content-Type: application/json" \
  -H "Accept: application/x-ndjson" \
  -d "$BODY" 2>"$CURL_ERR" > "$RAW_OUT" &
CURL_PID=$!

# Give curl a moment to connect and get first bytes
sleep 1
if ! kill -0 "$CURL_PID" 2>/dev/null; then
  CURL_CODE=$?
  echo "❌  curl a échoué immédiatement (code $CURL_CODE)"
  cat "$CURL_ERR"
  rm -f "$RAW_OUT" "$CURL_ERR"
  exit 1
fi

# Stream the file as it grows, pipe into progress
tail -f "$RAW_OUT" | draw_progress &
TAIL_PID=$!

# Wait for curl to finish
wait "$CURL_PID"
CURL_CODE=$?

# Give tail/python a moment to process the last line
sleep 0.5
kill "$TAIL_PID" 2>/dev/null || true

echo ""

# Always show raw response for debugging if empty or error
LINE_COUNT=$(wc -l < "$RAW_OUT" | tr -d ' ')
if [[ "$LINE_COUNT" -eq 0 ]]; then
  echo "❌  Aucune réponse reçue du serveur."
  if [[ -s "$CURL_ERR" ]]; then
    echo "    Erreur curl : $(cat "$CURL_ERR")"
  fi
  echo "    Code curl   : $CURL_CODE"
  rm -f "$RAW_OUT" "$CURL_ERR"
  exit 1
fi

# Show first line in case of non-NDJSON error
FIRST_LINE=$(head -1 "$RAW_OUT")
if ! echo "$FIRST_LINE" | python3 -c "import sys,json; json.load(sys.stdin)" 2>/dev/null; then
  echo "❌  Réponse non-JSON du serveur :"
  cat "$RAW_OUT"
  rm -f "$RAW_OUT" "$CURL_ERR"
  exit 1
fi

rm -f "$RAW_OUT" "$CURL_ERR"

if [[ "$CURL_CODE" -ne 0 && "$CURL_CODE" -ne 23 ]]; then
  echo "❌  curl a échoué (code $CURL_CODE)"
  exit "$CURL_CODE"
fi
exit 0

