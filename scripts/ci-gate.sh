#!/bin/bash

set -e

echo "Starting infrastructure..."
docker compose up -d --build

echo "Waiting for all services to be healthy..."
if ! ./scripts/health-check.sh; then
    echo "FAIL: Health check timed out."
    docker compose logs
    exit 1
fi

echo "Running final assertions..."
SERVICES=("ingest" "recorder" "event-proc" "auth")
for svc in "${SERVICES[@]}"; do
    echo "Asserting $svc /healthz..."
    HEALTH=$(docker compose exec -T "$svc" wget -qO- http://localhost:8080/healthz)
    if [[ "$HEALTH" != *"\"status\":\"ok\""* ]]; then
        echo "FAIL: $svc health check failed."
        exit 1
    fi
done

echo "Asserting NATS /healthz (port 8222)..."
docker compose exec -T nats wget -q --spider http://localhost:8222/healthz

echo "Asserting Postgres pg_isready..."
docker compose exec -T postgres pg_isready -U vms_user -d vms_db

echo "Asserting Redis ping..."
docker compose exec -T redis redis-cli ping | grep PONG

echo "PASS"
exit 0
