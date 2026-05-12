#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="/etc/festival/backup.env"

if [[ -f "$ENV_FILE" ]]; then
	set -a
	# shellcheck disable=SC1090
	source "$ENV_FILE"
	set +a
else
	echo "❌ Missing env file: $ENV_FILE"
	exit 1
fi

BACKUP_DIR="/opt/festival/backups"
REMOTE_USER="kreton"
REMOTE_HOST="kreton.duckdns.org"
REMOTE_DIR="/srv/backups/festival"

mkdir -p "$BACKUP_DIR"

# Local dump
DUMP_FILE="$BACKUP_DIR/backup_$(date +%Y%m%d_%H%M%S).dump"
./scripts/export_db_dump.sh -o "$DUMP_FILE"

# Send to remote
rsync -av --compress -e "ssh -p 7493" "$DUMP_FILE" "${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_DIR}/"

# (optional) prune local backups older than 14 days
find "$BACKUP_DIR" -type f -name "backup_*.dump" -mtime +14 -delete
