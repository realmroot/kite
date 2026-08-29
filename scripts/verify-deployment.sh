#!/usr/bin/env bash

set -euo pipefail

cd "$(dirname "$0")/.."

chart=charts/lightkite
common_values=(
  --set image.repository=example.invalid/lightkite
  --set host=https://lightkite.example.com
  --set oidc.issuer=https://identity.example.com
  --set oidc.clientId=test-client
  --set oidc.clientSecret=test-secret
  --set platformAdminGroups=platform-admins
  --set encryptKey=test-encryption-key
)

helm lint "$chart" "${common_values[@]}"

rendered="$(mktemp)"
trap 'rm -f "$rendered"' EXIT
helm template lightkite "$chart" --namespace lightkite-system "${common_values[@]}" >"$rendered"

required_patterns=(
  'kind: PersistentVolumeClaim'
  'type: Recreate'
  'automountServiceAccountToken: false'
  'runAsNonRoot: true'
  'readOnlyRootFilesystem: true'
  'allowPrivilegeEscalation: false'
  'mountPath: /tmp'
  'mountPath: /data'
  '_pragma=foreign_keys\(1\)&_pragma=busy_timeout\(5000\)&_pragma=journal_mode\(WAL\)'
  'value: "alpine/kubectl:1.36.3"'
  'value: "busybox:1.37.0"'
)
for pattern in "${required_patterns[@]}"; do
  if ! rg -q "$pattern" "$rendered"; then
    echo "rendered chart is missing production invariant: $pattern" >&2
    exit 1
  fi
done

expect_failure() {
  local description="$1"
  shift
  if helm template lightkite "$chart" --namespace lightkite-system "${common_values[@]}" "$@" >/dev/null 2>&1; then
    echo "chart accepted invalid configuration: $description" >&2
    exit 1
  fi
}

expect_failure "missing image repository" --set image.repository=
expect_failure "multiple SQLite replicas" --set replicaCount=2
expect_failure "rolling SQLite PVC deployment" --set deploymentStrategy.type=RollingUpdate
expect_failure "simultaneous PVC and hostPath storage" --set db.sqlite.persistence.hostPath.enabled=true
expect_failure "external database without DSN" --set db.type=postgres
expect_failure "unsupported database type" --set db.type=oracle
expect_failure "analytics enabled without destination" --set analytics.enabled=true
expect_failure "partial analytics configuration" --set analytics.scriptURL=https://analytics.example.com/script.js

helm template lightkite "$chart" --namespace lightkite-system "${common_values[@]}" \
  --set analytics.enabled=true \
  --set analytics.scriptURL=https://analytics.example.com/script.js \
  --set analytics.websiteID=lightkite-site >"$rendered"
for pattern in 'name: ENABLE_ANALYTICS' 'name: ANALYTICS_SCRIPT_URL' 'name: ANALYTICS_WEBSITE_ID'; do
  if ! rg -q "$pattern" "$rendered"; then
    echo "operator-owned analytics wiring is missing: $pattern" >&2
    exit 1
  fi
done

helm template lightkite "$chart" --namespace lightkite-system "${common_values[@]}" \
  --set oidc.ca.existingSecret=private-issuer-ca >"$rendered"
for pattern in 'name: OIDC_CA_FILE' 'secretName: private-issuer-ca' 'mountPath: /etc/lightkite/oidc'; do
  if ! rg -q "$pattern" "$rendered"; then
    echo "private OIDC CA wiring is missing: $pattern" >&2
    exit 1
  fi
done

public_values=("${common_values[@]}")
public_values+=(--set oidc.clientSecret=)
helm template lightkite "$chart" --namespace lightkite-system "${public_values[@]}" >"$rendered"
if rg -q 'OIDC_CLIENT_SECRET' "$rendered"; then
  echo "public PKCE client unexpectedly rendered OIDC_CLIENT_SECRET" >&2
  exit 1
fi
for pattern in 'name: OIDC_ISSUER' 'OIDC_CLIENT_ID:'; do
  if ! rg -q "$pattern" "$rendered"; then
    echo "public PKCE client is missing required OIDC wiring: $pattern" >&2
    exit 1
  fi
done

helm template lightkite "$chart" --namespace lightkite-system "${common_values[@]}" \
  --set secret.create=false \
  --set secret.existingSecret=lightkite-runtime \
  --set db.type=postgres >"$rendered"
if rg -q 'kind: PersistentVolumeClaim' "$rendered"; then
  echo "external database deployment unexpectedly rendered a SQLite PVC" >&2
  exit 1
fi
if ! rg -q 'value: "postgres"' "$rendered"; then
  echo "external database type is not wired into the deployment" >&2
  exit 1
fi
