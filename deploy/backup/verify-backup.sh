#!/bin/bash
# Usage: ./verify-backup.sh [backup_file_or_dir]
# Exits 0 on success, 1 on failure

set -euo pipefail

ARG="${1:-/backups}"

if [ -f "$ARG" ]; then
  BACKUP_FILE="$ARG"
elif [ -d "$ARG" ]; then
  BACKUP_DIR="$ARG"
  LATEST=$(ls -t "$BACKUP_DIR"/*.sql.gz 2>/dev/null | head -1)
  if [ -z "$LATEST" ]; then
    echo "ERROR: No backup found in $BACKUP_DIR"
    exit 1
  fi
  BACKUP_FILE="$LATEST"
else
  echo "ERROR: Not a file or directory: $ARG"
  exit 1
fi

echo "Verifying: $BACKUP_FILE"

if ! pg_restore -l "$BACKUP_FILE" > /dev/null 2>&1; then
  echo "FAILED: pg_restore -l validation error"
  exit 1
fi

ENTRIES=$(pg_restore -l "$BACKUP_FILE" 2>/dev/null | wc -l)
echo "OK - $ENTRIES objects verified"
exit 0
