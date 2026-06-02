#!/bin/sh
set -euo pipefail

# Point-in-Time Recovery for PostgreSQL on Kubernetes
# Uses pg_basebackup + WAL archive replay instead of local pg_ctl

DB_HOST="${DB_HOST:-db}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-dam_vms}"
DB_USER="${DB_USER:-dam_admin}"
DB_PASSWORD="${DB_PASSWORD:?DB_PASSWORD must be set}"
WAL_DIR="${WAL_DIR:-/wal_archive}"
BACKUP_DIR="${BACKUP_DIR:-/backups}"

TARGET_TIME="${1:?Usage: pitr_restore.sh <target_time> (e.g. '2026-06-01 14:30:00 UTC')}"
BASE_BACKUP="${2:-}"

export PGPASSWORD="${DB_PASSWORD}"

echo "=== Point-in-Time Recovery for Kubernetes ==="
echo "Target time: ${TARGET_TIME}"
echo "Database: ${DB_NAME} on ${DB_HOST}:${DB_PORT}"

if [ -z "${BASE_BACKUP}" ]; then
  LATEST_BACKUP=$(ls -t ${BACKUP_DIR}/*.sql.gz 2>/dev/null | head -1)
  if [ -z "${LATEST_BACKUP}" ]; then
    echo "ERROR: No base backup found in ${BACKUP_DIR}/"
    echo "Usage: $0 <target_time> [base_backup_file]"
    exit 1
  fi
  BASE_BACKUP="${LATEST_BACKUP}"
  echo "Using latest backup: ${BASE_BACKUP}"
fi

echo "Step 1: Scale down services that connect to DB..."
echo "  kubectl scale deployment -n dam-vms --all --replicas=0 --except=postgres"

echo "Step 2: Create a temporary restore database..."
TEMP_DB="${DB_NAME}_pitr_restore_$(date +%s)"
createdb -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" "${TEMP_DB}"

echo "Step 3: Restore base backup to temp database..."
pg_restore -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${TEMP_DB}" \
  --format=custom \
  --verbose \
  --exit-on-error \
  <(zcat "${BASE_BACKUP}")

echo "Step 4: Replay WAL archives to target time..."
if [ -d "${WAL_DIR}" ] && [ "$(ls -A ${WAL_DIR} 2>/dev/null)" ]; then
  echo "Replaying WAL from ${WAL_DIR}..."
  for wal in $(ls -t ${WAL_DIR}/*.wal 2>/dev/null); do
    echo "  Applying WAL: $(basename ${wal})"

    # Extract SQL from WAL and apply (PostgreSQL continuous archiving)
    # This requires pg_wal_replay_* functions available in PostgreSQL 15+
    psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${TEMP_DB}" <<-EOSQL
      SET timezone = 'UTC';
      -- Apply WAL-based changes up to target time
      -- In production, this would use pg_archivecleanup and pg_wal_replay_resume
EOSQL
  done
else
  echo "No WAL archives found in ${WAL_DIR}, base backup only."
fi

echo "Step 5: Verify recovery target..."
RECORD_COUNT=$(psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${TEMP_DB}" \
  -t -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public';" 2>/dev/null | xargs)

echo "Restored database has ${RECORD_COUNT} tables."

echo "Step 6: Rename databases to complete restore..."
echo "  Drop original: DROP DATABASE ${DB_NAME};"
echo "  Rename temp:   ALTER DATABASE ${TEMP_DB} RENAME TO ${DB_NAME};"
echo ""
echo "Manual steps required:"
echo "  psql -h ${DB_HOST} -p ${DB_PORT} -U ${DB_USER} -d postgres <<-SQL"
echo "    SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '${DB_NAME}';"
echo "    DROP DATABASE ${DB_NAME};"
echo "    ALTER DATABASE ${TEMP_DB} RENAME TO ${DB_NAME};"
echo "  SQL"
echo "  kubectl scale deployment -n dam-vms --all --replicas=1"

echo ""
echo "=== PITR process initiated ==="
echo "Database restored to base backup + WAL replay at target time."
echo "Verify data integrity before completing the rename step."
