#!/bin/sh
set -euo pipefail

DB_HOST="${DB_HOST:-db}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-dam_vms}"
DB_USER="${DB_USER:-dam_admin}"
DB_PASSWORD="${DB_PASSWORD:?DB_PASSWORD must be set}"

RESTORE_FILE="${1:?Usage: pg_restore.sh <backup_file.sql.gz>}"
RESTORE_MODE="${RESTORE_MODE:-full}"
DROP_BEFORE="${DROP_BEFORE:-false}"

export PGPASSWORD="${DB_PASSWORD}"

if [ ! -f "${RESTORE_FILE}" ]; then
  echo "ERROR: Backup file not found: ${RESTORE_FILE}"
  exit 1
fi

if [ "${DROP_BEFORE}" = "true" ]; then
  echo "Dropping existing connections to ${DB_NAME}..."
  psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d postgres -c "
    SELECT pg_terminate_backend(pg_stat_activity.pid)
    FROM pg_stat_activity
    WHERE pg_stat_activity.datname = '${DB_NAME}'
      AND pid <> pg_backend_pid();"

  echo "Dropping and recreating database ${DB_NAME}..."
  psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d postgres -c "DROP DATABASE IF EXISTS ${DB_NAME};"
  createdb -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" "${DB_NAME}"
fi

echo "Starting restore from: ${RESTORE_FILE}"

if echo "${RESTORE_FILE}" | grep -q '\.gz$'; then
  pg_restore -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" \
    --format=custom \
    --verbose \
    --exit-on-error \
    <(zcat "${RESTORE_FILE}")
else
  pg_restore -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" \
    --format=custom \
    --verbose \
    --exit-on-error \
    "${RESTORE_FILE}"
fi

echo "Restore complete. Running ANALYZE..."
psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" -c "ANALYZE;"

echo "Restore completed successfully from: ${RESTORE_FILE}"
