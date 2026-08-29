#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${KITE_E2E_PORT:-38080}"
if [ -n "${KITE_E2E_DB_PATH:-}" ]; then
  DB_PATH="${KITE_E2E_DB_PATH}"
else
  DB_PATH="$(mktemp "${TMPDIR:-/tmp}/kite-e2e.XXXXXX")"
fi

cd "${ROOT_DIR}"

rm -f "${DB_PATH}"

make build

export DB_TYPE=sqlite
export DB_DSN="${DB_PATH}?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
export DISABLE_VERSION_CHECK=true
export JWT_SECRET="${KITE_E2E_JWT_SECRET:-kite-e2e-jwt-secret}"
export KITE_ENCRYPT_KEY="${KITE_E2E_ENCRYPT_KEY:-kite-e2e-encryption-key}"
export OIDC_ISSUER="${KITE_E2E_OIDC_ISSUER:-https://localhost:5556}"
export OIDC_CA_FILE="${KITE_E2E_OIDC_CA_FILE:-${KITE_E2E_OIDC_CERT_DIR:-${TMPDIR:-/tmp}/kite-e2e-oidc}/ca.crt}"
export OIDC_CLIENT_ID="${KITE_E2E_OIDC_CLIENT_ID:-kite-e2e}"
export OIDC_CLIENT_SECRET="${KITE_E2E_OIDC_CLIENT_SECRET:-kite-e2e-secret}"
export OIDC_PROVIDER_NAME="${KITE_E2E_OIDC_PROVIDER_NAME:-Kite E2E Identity}"
export OIDC_SCOPES="${KITE_E2E_OIDC_SCOPES:-openid profile email offline_access}"
export OIDC_USERNAME_CLAIM="${KITE_E2E_OIDC_USERNAME_CLAIM:-email}"
export OIDC_GROUPS_CLAIM="${KITE_E2E_OIDC_GROUPS_CLAIM:-groups}"
export OIDC_NAME_CLAIM="${KITE_E2E_OIDC_NAME_CLAIM:-name}"
export OIDC_PICTURE_CLAIM="${KITE_E2E_OIDC_PICTURE_CLAIM:-picture}"
export PLATFORM_ADMIN_GROUPS="${KITE_E2E_PLATFORM_ADMIN_GROUPS:-}"
export PLATFORM_ADMIN_SUBJECTS="${KITE_E2E_PLATFORM_ADMIN_SUBJECTS:-CiQxMTExMTExMS0xMTExLTQxMTEtODExMS0xMTExMTExMTExMTESBWxvY2Fs}"
export HOST="${KITE_E2E_HOST:-http://127.0.0.1:${PORT}}"
export CLUSTER_INVENTORY_ENABLED=false
if [ -n "${KITE_E2E_CONFIG_FILE:-}" ]; then
  export KITE_CONFIG_FILE="${KITE_E2E_CONFIG_FILE}"
else
  E2E_CONFIG_PATH="$(mktemp "${TMPDIR:-/tmp}/kite-e2e-config.XXXXXX")"
  printf '{}\n' >"${E2E_CONFIG_PATH}"
  export KITE_CONFIG_FILE="${E2E_CONFIG_PATH}"
fi
export PORT
KITE_E2E_HELM_DIR="${KITE_E2E_HELM_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/kite-e2e-helm.XXXXXX")}"
export HELM_CACHE_HOME="${KITE_E2E_HELM_DIR}/cache"
export HELM_CONFIG_HOME="${KITE_E2E_HELM_DIR}/config"
export HELM_DATA_HOME="${KITE_E2E_HELM_DIR}/data"
unset KITE_USERNAME
unset KITE_PASSWORD
unset KUBECONFIG

exec ./kite -v 3
