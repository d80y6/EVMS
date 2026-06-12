#!/bin/bash
# EVMS Backup Verification Script
set -euo pipefail

BACKUP_DIR="${1:-/backups}"
FRESHNESS_HOURS="${FRESHNESS_HOURS:-48}"

echo "=== EVMS Backup Verification ==="
echo "Backup directory: $BACKUP_DIR"
echo ""

# Find latest backup
LATEST=$(ls -t "$BACKUP_DIR"/*.sql.gz 2>/dev/null | head -1)
if [ -z "$LATEST" ]; then
    LATEST=$(ls -t "$BACKUP_DIR"/*.dump 2>/dev/null | head -1)
fi
if [ -z "$LATEST" ]; then
    echo "ERROR: No backup found in $BACKUP_DIR"
    exit 1
fi

echo "Latest backup: $(basename "$LATEST")"

# Check freshness
if [ -f "$LATEST" ]; then
    FILE_AGE=$(stat -c %Y "$LATEST")
    NOW=$(date +%s)
    AGE_HOURS=$(( (NOW - FILE_AGE) / 3600 ))
    echo "Age: ${AGE_HOURS}h (max: ${FRESHNESS_HOURS}h)"

    if [ "$AGE_HOURS" -gt "$FRESHNESS_HOURS" ]; then
        echo "WARNING: Backup is stale! No fresh backup in ${FRESHNESS_HOURS}h"
    fi
fi

# Verify backup integrity
echo ""
echo "--- Integrity Check ---"
if [[ "$LATEST" == *.sql.gz ]]; then
    GZIP_STATUS=$(gunzip -t "$LATEST" 2>&1 && echo "ok" || echo "fail")
    echo "GZip integrity: $GZIP_STATUS"
    if [ "$GZIP_STATUS" != "ok" ]; then
        echo "FAILED: Backup file is corrupted (gzip check)"
        exit 1
    fi

    # Decompress to temp and check with pg_restore
    TEMP_FILE=$(mktemp)
    gunzip -c "$LATEST" > "$TEMP_FILE"
    if pg_restore -l "$TEMP_FILE" > /dev/null 2>&1; then
        ENTRIES=$(pg_restore -l "$TEMP_FILE" 2>/dev/null | wc -l)
        echo "pg_restore integrity: OK ($ENTRIES objects)"
    else
        echo "pg_restore integrity: FAILED"
        rm -f "$TEMP_FILE"
        exit 1
    fi
    rm -f "$TEMP_FILE"
elif [[ "$LATEST" == *.dump ]]; then
    if pg_restore -l "$LATEST" > /dev/null 2>&1; then
        ENTRIES=$(pg_restore -l "$LATEST" 2>/dev/null | wc -l)
        echo "pg_restore integrity: OK ($ENTRIES objects)"
    else
        echo "pg_restore integrity: FAILED"
        exit 1
    fi
fi

echo ""
echo "=== Backup Verification PASSED ==="
exit 0