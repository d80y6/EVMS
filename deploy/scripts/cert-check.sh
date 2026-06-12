#!/bin/bash
# Check certificate expiration and alert if renewal needed
set -euo pipefail

NAMESPACE="${1:-default}"
DAYS_WARN="${2:-30}"
DAYS_CRIT="${2:-7}"

for secret in $(kubectl get certificate -n "$NAMESPACE" -o name); do
    echo "Checking $secret..."
    expiry=$(kubectl get "$secret" -n "$NAMESPACE" -o jsonpath='{.status.notAfter}')
    if [ -z "$expiry" ]; then
        echo "WARNING: $secret has no expiry date"
        continue
    fi
    expires=$(date -d "$expiry" +%s)
    now=$(date +%s)
    days_left=$(( (expires - now) / 86400 ))
    if [ "$days_left" -lt "$DAYS_CRIT" ]; then
        echo "CRITICAL: $secret expires in $days_left days"
    elif [ "$days_left" -lt "$DAYS_WARN" ]; then
        echo "WARNING: $secret expires in $days_left days"
    else
        echo "OK: $secret expires in $days_left days"
    fi
done
