#!/bin/bash
set -e
HOST_IP=$(hostname -I | awk '{print $1}')
export HOST_IP
cd "$(dirname "$0")"
echo "Detected host IP: $HOST_IP"
docker compose --env-file ../../.env -f docker-compose.yml up -d "$@"
