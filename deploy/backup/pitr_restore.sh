#!/bin/sh
set -euo pipefail

DB_HOST="${DB_HOST:-db}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-dam_vms}"
DB_USER="${DB_USER:-dam_admin}"
DB_PASSWORD="${DB_PASSWORD:?DB_PASSWORD must be set}"
DATA_DIR="${DATA_DIR:-/var/lib/postgresql/data}"
WAL_DIR="${WAL_DIR:-/wal_archive}"

TARGET_TIME="${1:?Usage: pitr_restore.sh <target_time> (e.g. '2026-06-01 14:30:00 UTC')}"
BASE_BACKUP="${2:-}"

export PGPASSWORD="${DB_PASSWORD}"

echo "=== Point-in-Time Recovery ==="
echo "Target time: ${TARGET_TIME}"
echo "Data directory: ${DATA_DIR}"
echo "WAL archive: ${WAL_DIR}"

if [ -z "${BASE_BACKUP}" ]; then
  LATEST_BACKUP=$(ls -t /backups/*.sql.gz 2>/dev/null | head -1)
  if [ -z "${LATEST_BACKUP}" ]; then
    echo "ERROR: No base backup found in /backups/"
    exit 1
  fi
  BASE_BACKUP="${LATEST_BACKUP}"
  echo "Using latest backup: ${BASE_BACKUP}"
fi

echo "Step 1: Shutdown PostgreSQL (if running)..."
pg_ctl -D "${DATA_DIR}" stop 2>/dev/null || true

echo "Step 2: Restore base backup..."
pg_restore -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" \
  --format=custom \
  --verbose \
  --exit-on-error \
  <(zcat "${BASE_BACKUP}")

echo "Step 3: Configure recovery.conf for PITR..."
cat > "${DATA_DIR}/recovery.conf" << EOF
restore_command = 'cp ${WAL_DIR}/%f %p'
recovery_target_time = '${TARGET_TIME}'
recovery_target_timeline = 'latest'
pause_at_recovery_target = false
EOF

echo "Step 4: Start PostgreSQL in recovery mode..."
pg_ctl -D "${DATA_DIR}" start -w

echo "Step 5: Verify recovery completed..."
sleep 5
RECOVERY_STATUS=$(psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" \
  -t -c "SELECT pg_is_in_recovery();" 2>/dev/null | xargs)

if [ "${RECOVERY_STATUS}" = "f" ]; then
  echo "PITR complete. Database is accepting writes at target time: ${TARGET_TIME}"
else
  echo "WARNING: Database may still be in recovery mode."
  echo "Recovery status: ${RECOVERY_STATUS}"
fi

echo "Step 6: Run ANALYZE..."
psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" -c "ANALYZE;"

echo "Point-in-Time Recovery completed successfully."
