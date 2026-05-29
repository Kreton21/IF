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
set -euo pipefail

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

for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    try:
        data = json.loads(line)
    except json.JSONDecodeError:
        continue

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

    status = ""
    if failed > 0:
        status = f"  ⚠️  {failed} échec(s)"

    if done:
        elapsed_str = f"{elapsed:.0f}s"
        print(f"\r  [{bar}] {sent+failed}/{total}  ✅ Terminé en {elapsed_str}{status}          ")
        # final summary
        print()
        print(f"  ✅  Envoyés  : {sent}")
        if failed:
            print(f"  ❌  Échoués  : {failed}")
        err = data.get("error")
        if err:
            print(f"  ⚠️  Erreur   : {err}")
        sys.exit(0 if not data.get("error") else 1)
    else:
        eta_str = f"  ETA {remaining:.0f}s" if remaining > 0 else ""
        line_out = f"\r  [{bar}] {sent+failed}/{total}  ({pct*100:.0f}%)  {rate:.1f}/s{eta_str}{status}   "
        print(line_out, end="", flush=True)

PYEOF
}

# ── Lancer le broadcast et streamer la progression ───────────
curl -sS -N --no-buffer \
  -X POST "$ENDPOINT" \
  -H "X-Broadcast-Key: $KEY" \
  -H "Content-Type: application/json" \
  -H "Accept: application/x-ndjson" \
  -d "$BODY" | draw_progress

EXIT_CODE=${PIPESTATUS[0]}
if [[ "$EXIT_CODE" -ne 0 ]]; then
  echo "❌  curl a échoué (code $EXIT_CODE) — vérifiez l'API et la clé."
  exit "$EXIT_CODE"
fi

