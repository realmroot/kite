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
export JWT_SECRET="${JWT_SECRET:-kite-e2e-jwt-secret}"
export KITE_ENCRYPT_KEY="${KITE_ENCRYPT_KEY:-kite-e2e-encryption-key}"
export OIDC_ISSUER="${OIDC_ISSUER:-https://localhost:5556}"
export OIDC_CA_FILE="${OIDC_CA_FILE:-${KITE_E2E_OIDC_CERT_DIR:-${TMPDIR:-/tmp}/kite-e2e-oidc}/ca.crt}"
export OIDC_CLIENT_ID="${OIDC_CLIENT_ID:-kite-e2e}"
export OIDC_CLIENT_SECRET="${OIDC_CLIENT_SECRET:-kite-e2e-secret}"
export OIDC_PROVIDER_NAME="${OIDC_PROVIDER_NAME:-Kite E2E Identity}"
export OIDC_SCOPES="${OIDC_SCOPES:-openid profile email offline_access}"
export OIDC_USERNAME_CLAIM="${OIDC_USERNAME_CLAIM:-email}"
export OIDC_GROUPS_CLAIM="${OIDC_GROUPS_CLAIM:-groups}"
export PLATFORM_ADMIN_SUBJECTS="${PLATFORM_ADMIN_SUBJECTS:-CiQxMTExMTExMS0xMTExLTQxMTEtODExMS0xMTExMTExMTExMTESBWxvY2Fs}"
export HOST="${HOST:-http://127.0.0.1:${PORT}}"
export PORT
KITE_E2E_HELM_DIR="${KITE_E2E_HELM_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/kite-e2e-helm.XXXXXX")}"
export HELM_CACHE_HOME="${KITE_E2E_HELM_DIR}/cache"
export HELM_CONFIG_HOME="${KITE_E2E_HELM_DIR}/config"
export HELM_DATA_HOME="${KITE_E2E_HELM_DIR}/data"
unset KITE_USERNAME
unset KITE_PASSWORD
unset KUBECONFIG

exec ./kite -v 3
