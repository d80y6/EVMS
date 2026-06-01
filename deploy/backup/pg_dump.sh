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
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="${BACKUP_DIR}/${DB_NAME}_${TIMESTAMP}.sql.gz"
WAL_DIR="${WAL_DIR:-/wal_archive}"

export PGPASSWORD="${DB_PASSWORD}"

mkdir -p "${BACKUP_DIR}"

pg_dump -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" \
  --format=custom \
  --compress=9 \
  --file="${BACKUP_FILE}"

echo "Backup created: ${BACKUP_FILE}"

find "${BACKUP_DIR}" -name "${DB_NAME}_*.sql.gz" -mtime +"${RETENTION_DAYS}" -delete

if [ -n "${S3_BUCKET}" ]; then
  aws s3 cp "${BACKUP_FILE}" "s3://${S3_BUCKET}/database/${TIMESTAMP}.sql.gz" \
    --region "${S3_REGION}"
  echo "Backup uploaded to s3://${S3_BUCKET}/database/${TIMESTAMP}.sql.gz"

  aws s3 sync "${WAL_DIR}" "s3://${S3_BUCKET}/wal/" \
    --region "${S3_REGION}" \
    --exclude "*" \
    --include "*.wal"
fi

echo "Backup complete"
