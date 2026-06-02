#!/bin/sh
set -euo pipefail

DB_HOST="${DB_HOST:-db}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-dam_vms}"
DB_USER="${DB_USER:-dam_admin}"
DB_PASSWORD="${DB_PASSWORD:?DB_PASSWORD must be set}"
BACKUP_DIR="${BACKUP_DIR:-/backups}"
RETENTION_DAYS="${RETENTION_DAYS:-30}"
S3_BUCKET="${S3_BUCKET:-}"
S3_REGION="${S3_REGION:-us-east-1}"
METRICS_FILE="${METRICS_FILE:-/backups/.metrics}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="${BACKUP_DIR}/${DB_NAME}_${TIMESTAMP}.sql.gz"
WAL_DIR="${WAL_DIR:-/wal_archive}"

export PGPASSWORD="${DB_PASSWORD}"

mkdir -p "${BACKUP_DIR}"

echo "Starting backup at $(date --iso-8601=seconds)"

# Step 1: Perform the backup
echo "Creating backup: ${BACKUP_FILE}"
pg_dump -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" \
  --format=custom \
  --compress=9 \
  --file="${BACKUP_FILE}"

BACKUP_SIZE=$(stat -c%s "${BACKUP_FILE}" 2>/dev/null || stat -f%z "${BACKUP_FILE}" 2>/dev/null || echo "0")
echo "Backup created: ${BACKUP_FILE} (${BACKUP_SIZE} bytes)"

# Step 2: Validate the backup by testing restore headers
echo "Validating backup..."
if pg_restore -l "${BACKUP_FILE}" > /dev/null 2>&1; then
  echo "Backup validation: PASSED"
  VALIDATION_STATUS="1"
else
  echo "Backup validation: FAILED" >&2
  VALIDATION_STATUS="0"
fi

# Step 3: Clean up old backups
echo "Cleaning backups older than ${RETENTION_DAYS} days..."
find "${BACKUP_DIR}" -name "${DB_NAME}_*.sql.gz" -mtime +"${RETENTION_DAYS}" -delete

# Step 4: Upload to S3 if configured
if [ -n "${S3_BUCKET}" ]; then
  echo "Uploading backup to s3://${S3_BUCKET}/database/${TIMESTAMP}.sql.gz"
  aws s3 cp "${BACKUP_FILE}" "s3://${S3_BUCKET}/database/${TIMESTAMP}.sql.gz" \
    --region "${S3_REGION}"
  echo "Backup uploaded."

  aws s3 sync "${WAL_DIR}" "s3://${S3_BUCKET}/wal/" \
    --region "${S3_REGION}" \
    --exclude "*" \
    --include "*.wal" || echo "WAL sync skipped (no changes)"

  # Upload validation metrics
  echo "vms_backup_timestamp_seconds $(date +%s)" > "${METRICS_FILE}"
  echo "vms_backup_size_bytes ${BACKUP_SIZE}" >> "${METRICS_FILE}"
  echo "vms_backup_validation_status ${VALIDATION_STATUS}" >> "${METRICS_FILE}"
  aws s3 cp "${METRICS_FILE}" "s3://${S3_BUCKET}/metrics/backup_${TIMESTAMP}.prom" \
    --region "${S3_REGION}" || true
fi

echo "Backup complete at $(date --iso-8601=seconds)"
echo "Status: $([ "${VALIDATION_STATUS}" = "1" ] && echo 'SUCCESS' || echo 'FAILED')"
