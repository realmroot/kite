#!/usr/bin/env bash
set -euo pipefail

OIDC_CERT_DIR="${KITE_E2E_OIDC_CERT_DIR:-${TMPDIR:-/tmp}/kite-e2e-oidc}"
OIDC_CA_KEY="${OIDC_CERT_DIR}/ca.key"
OIDC_CA_CERT="${OIDC_CERT_DIR}/ca.crt"
OIDC_SERVER_KEY="${OIDC_CERT_DIR}/tls.key"
OIDC_SERVER_CERT="${OIDC_CERT_DIR}/tls.crt"

mkdir -p "${OIDC_CERT_DIR}"

if [ -f "${OIDC_CA_KEY}" ] && [ -f "${OIDC_CA_CERT}" ] && \
   [ -f "${OIDC_SERVER_KEY}" ] && [ -f "${OIDC_SERVER_CERT}" ] && \
   openssl x509 -checkend 86400 -noout -in "${OIDC_SERVER_CERT}" >/dev/null; then
  printf '%s\n' "${OIDC_CERT_DIR}"
  exit 0
fi

rm -f "${OIDC_CA_KEY}" "${OIDC_CA_CERT}" "${OIDC_SERVER_KEY}" "${OIDC_SERVER_CERT}"

openssl req -x509 -newkey rsa:2048 -sha256 -nodes \
  -keyout "${OIDC_CA_KEY}" \
  -out "${OIDC_CA_CERT}" \
  -days 3650 \
  -subj "/CN=Kite E2E OIDC CA" >/dev/null 2>&1

openssl req -newkey rsa:2048 -sha256 -nodes \
  -keyout "${OIDC_SERVER_KEY}" \
  -out "${OIDC_CERT_DIR}/tls.csr" \
  -subj "/CN=localhost" >/dev/null 2>&1

openssl x509 -req -sha256 \
  -in "${OIDC_CERT_DIR}/tls.csr" \
  -CA "${OIDC_CA_CERT}" \
  -CAkey "${OIDC_CA_KEY}" \
  -CAcreateserial \
  -out "${OIDC_SERVER_CERT}" \
  -days 825 \
  -extfile <(printf '%s\n' 'subjectAltName=DNS:localhost,IP:127.0.0.1' 'extendedKeyUsage=serverAuth') >/dev/null 2>&1

rm -f "${OIDC_CERT_DIR}/tls.csr" "${OIDC_CERT_DIR}/ca.srl"
chmod 600 "${OIDC_CA_KEY}" "${OIDC_SERVER_KEY}"
printf '%s\n' "${OIDC_CERT_DIR}"
