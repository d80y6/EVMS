#!/bin/bash
# Initialize Vault with secrets from environment
set -euo pipefail

VAULT_ADDR="${VAULT_ADDR:-http://vault:8200}"
VAULT_TOKEN="${VAULT_TOKEN:-root}"

export VAULT_ADDR VAULT_TOKEN

# Enable KV secrets engine
vault secrets enable -path=evms kv-v2

# Store secrets
vault kv put evms/database DB_URL="${DB_URL}" DB_USER="${DB_USER}" DB_PASSWORD="${DB_PASSWORD}"
vault kv put evms/jwt JWT_SECRET="${JWT_SECRET}"
vault kv put evms/nats NATS_URL="${NATS_URL}"
vault kv put evms/aws AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID}" AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY}"

echo "Vault initialized successfully"
