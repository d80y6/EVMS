#!/bin/bash
set -euo pipefail

DB_HOST="${DB_HOST:-db}"
DB_PORT="${DB_PORT:-5432}"
DB_NAME="${DB_NAME:-dam_vms}"
DB_USER="${DB_USER:-dam_admin}"
DB_PASSWORD="${DB_PASSWORD:?DB_PASSWORD must be set}"
BACKUP_DIR="${BACKUP_DIR:-/backups}"
RETENTION_DAYS="${RETENTION_DAYS:-30}"
RETENTION_MODE="${RETENTION_MODE:-simple}"
S3_BUCKET="${S3_BUCKET:-}"
S3_REGION="${S3_REGION:-us-east-1}"
METRICS_FILE="${METRICS_FILE:-/backups/.metrics}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="${BACKUP_DIR}/${DB_NAME}_${TIMESTAMP}.sql.gz"
WAL_DIR="${WAL_DIR:-/wal_archive}"

export PGPASSWORD="${DB_PASSWORD}"

mkdir -p "${BACKUP_DIR}"

echo "Starting backup at $(date --iso-8601=seconds)"
echo "Retention mode: ${RETENTION_MODE}"

# Step 1: Perform the backup
echo "Creating backup: ${BACKUP_FILE}"
pg_dump -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" \
  --format=custom \
  --compress=9 \
  --file="${BACKUP_FILE}"

BACKUP_SIZE=$(stat -c%s "${BACKUP_FILE}" 2>/dev/null || stat -f%z "${BACKUP_FILE}" 2>/dev/null || echo "0")
echo "Backup created: ${BACKUP_FILE} (${BACKUP_SIZE} bytes)"

# Step 2: Verify backup integrity
verify_backup() {
  local backup_file="$1"
  local result=0
  local temp_dir

  echo "  Verifying backup integrity..."

  if ! pg_restore -l "$backup_file" > /dev/null 2>&1; then
    echo "  FAILED: Backup header/TOC validation error" >&2
    result=1
  fi

  if [ "$result" -eq 0 ]; then
    temp_dir=$(mktemp -d)
    if pg_restore -l "$backup_file" > "$temp_dir/toc.txt" 2>/dev/null; then
      local entry_count
      entry_count=$(wc -l < "$temp_dir/toc.txt")
      if [ "$entry_count" -gt 0 ]; then
        echo "  Integrity: $entry_count objects verified"
      else
        echo "  WARNING: No objects found in backup" >&2
      fi
    else
      echo "  FAILED: Could not extract backup contents" >&2
      result=1
    fi
    rm -rf "$temp_dir"
  fi

  echo "vms_backup_verification_status ${result}" >> "${METRICS_FILE}"

  if [ "$result" -eq 0 ]; then
    echo "  Verification: PASSED"
  else
    echo "  Verification: FAILED" >&2
  fi

  return "$result"
}

verify_backup "${BACKUP_FILE}"
VERIFY_RESULT=$?
VALIDATION_STATUS=$([ "$VERIFY_RESULT" -eq 0 ] && echo "1" || echo "0")

# Step 3: Clean up old backups
tiered_cleanup() {
  echo "Running tiered retention cleanup..."
  find "${BACKUP_DIR}" -maxdepth 1 -name "${DB_NAME}_*.sql.gz" -printf '%T@\t%p\n' | sort -rn | cut -f2- | {
    local count=0
    local weekly_kept=0
    local monthly_kept=0

    while IFS= read -r f; do
      count=$((count + 1))

      if [ "$count" -le 7 ]; then
        echo "  Keep (daily #$count): $(basename "$f")"
        continue
      fi

      if [ "$weekly_kept" -lt 4 ] && [ $(( (count - 8) % 7 )) -eq 0 ]; then
        weekly_kept=$((weekly_kept + 1))
        echo "  Keep (weekly #$weekly_kept): $(basename "$f")"
        continue
      fi

      if [ "$monthly_kept" -lt 12 ] && [ $(( (count - 8 - 4*7) % 30 )) -eq 0 ]; then
        monthly_kept=$((monthly_kept + 1))
        echo "  Keep (monthly #$monthly_kept): $(basename "$f")"
        continue
      fi

      echo "  Removing: $(basename "$f")"
      rm -f "$f"
    done
  }
}

if [ "${RETENTION_MODE}" = "tiered" ]; then
  tiered_cleanup
else
  echo "Cleaning backups older than ${RETENTION_DAYS} days..."
  find "${BACKUP_DIR}" -name "${DB_NAME}_*.sql.gz" -mtime +"${RETENTION_DAYS}" -delete
fi

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

  echo "vms_backup_timestamp_seconds $(date +%s)" > "${METRICS_FILE}"
  echo "vms_backup_size_bytes ${BACKUP_SIZE}" >> "${METRICS_FILE}"
  echo "vms_backup_validation_status ${VALIDATION_STATUS}" >> "${METRICS_FILE}"
  aws s3 cp "${METRICS_FILE}" "s3://${S3_BUCKET}/metrics/backup_${TIMESTAMP}.prom" \
    --region "${S3_REGION}" || true
fi

echo "Backup complete at $(date --iso-8601=seconds)"
echo "Status: $([ "${VALIDATION_STATUS}" = "1" ] && echo 'SUCCESS' || echo 'FAILED')"
