#!/bin/bash

SERVICES=("ingest" "recorder" "webrtc-relay" "ai-worker" "event-proc" "api-gateway" "auth")
INFRA=("postgres" "redis" "nats")

TIMEOUT=60
START_TIME=$(date +%s)

while true; do
    ALL_OK=true
    FAILING=()

    for svc in "${SERVICES[@]}"; do
        HEALTH=$(docker compose exec -T "$svc" wget -qO- http://localhost:8080/healthz 2>/dev/null)
        if [[ "$HEALTH" != *"\"status\":\"ok\""* ]]; then
            ALL_OK=false
            FAILING+=("$svc")
        fi
    done

    # Infra check via docker inspect or exec
    for svc in "${INFRA[@]}"; do
        STATUS=$(docker inspect --format='{{.State.Health.Status}}' $(docker compose ps -q "$svc"))
        if [[ "$STATUS" != "healthy" ]]; then
            ALL_OK=false
            FAILING+=("$svc")
        fi
    done

    if [ "$ALL_OK" = true ]; then
        echo "All 10 services are healthy."
        exit 0
    fi

    CURRENT_TIME=$(date +%s)
    ELAPSED=$((CURRENT_TIME - START_TIME))

    if [ $ELAPSED -ge $TIMEOUT ]; then
        echo "Timeout reached. Failing services: ${FAILING[*]}"
        exit 1
    fi

    echo "Waiting for services... (${FAILING[*]})"
    sleep 2
done
