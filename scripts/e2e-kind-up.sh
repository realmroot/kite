#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
KIND_NAME="${E2E_KIND_NAME:-lightkite-e2e}"
KUBECONFIG_PATH="${E2E_KUBECONFIG:-${TMPDIR:-/tmp}/lightkite-e2e.kubeconfig}"
OIDC_CERT_DIR="${KITE_E2E_OIDC_CERT_DIR:-${TMPDIR:-/tmp}/lightkite-e2e-oidc}"
OIDC_CA_FILE="${OIDC_CERT_DIR}/ca.crt"
NODE_IMAGE="${E2E_NODE_IMAGE:-}"
CONTROL_PLANE="${KIND_NAME}-control-plane"
FORWARDER="${KIND_NAME}-oidc-forwarder"
RUNTIME_CONFIG="$(mktemp "${TMPDIR:-/tmp}/lightkite-kind.XXXXXX.yaml")"

cleanup() {
  rm -f "${RUNTIME_CONFIG}"
}
trap cleanup EXIT

"${ROOT_DIR}/scripts/e2e-generate-oidc-certs.sh" >/dev/null
sed "s|__OIDC_CA_FILE__|${OIDC_CA_FILE}|g" \
  "${ROOT_DIR}/e2e/fixtures/kind/config.yaml" >"${RUNTIME_CONFIG}"

start_forwarder() {
  docker rm -f "${FORWARDER}" >/dev/null 2>&1 || true
  docker run -d --name "${FORWARDER}" \
    --network "container:${CONTROL_PLANE}" \
    alpine/socat:1.8.0.3 \
    TCP-LISTEN:5556,fork,reuseaddr TCP:host.docker.internal:5556 >/dev/null
}

install_metrics_server() {
  kubectl --kubeconfig "${KUBECONFIG_PATH}" apply -f \
    "${ROOT_DIR}/e2e/fixtures/kind/metrics-server.yaml" >/dev/null
  kubectl --kubeconfig "${KUBECONFIG_PATH}" -n kube-system rollout status \
    deployment/metrics-server --timeout=2m >/dev/null
  for _ in $(seq 1 30); do
    if kubectl --kubeconfig "${KUBECONFIG_PATH}" get --raw \
      /apis/metrics.k8s.io/v1beta1/nodes >/dev/null 2>&1; then
      return
    fi
    sleep 2
  done
  printf 'metrics-server API did not become available\n' >&2
  return 1
}

install_prometheus_api_fixture() {
  kubectl --kubeconfig "${KUBECONFIG_PATH}" apply -f \
    "${ROOT_DIR}/e2e/fixtures/kind/prometheus-api-fixture.yaml" >/dev/null
  kubectl --kubeconfig "${KUBECONFIG_PATH}" -n monitoring rollout status \
    deployment/lightkite-prometheus-api --timeout=2m >/dev/null
}

if kind get clusters | grep -qx "${KIND_NAME}"; then
  CURRENT_NODE_IMAGE="$(docker inspect "${CONTROL_PLANE}" --format '{{.Config.Image}}' 2>/dev/null || true)"
  if docker exec "${CONTROL_PLANE}" \
    grep -q -- '--oidc-issuer-url=https://localhost:5556' /etc/kubernetes/manifests/kube-apiserver.yaml && \
    { [ -z "${NODE_IMAGE}" ] || [ "${CURRENT_NODE_IMAGE}" = "${NODE_IMAGE}" ]; }; then
    printf '☸️ Reusing OIDC-enabled kind cluster %s...\n' "${KIND_NAME}"
    start_forwarder
    kind export kubeconfig --name "${KIND_NAME}" --kubeconfig "${KUBECONFIG_PATH}"
    install_metrics_server
    install_prometheus_api_fixture
    exit 0
  fi
  printf '♻️ Replacing stale kind cluster without the current OIDC configuration...\n'
  docker rm -f "${FORWARDER}" >/dev/null 2>&1 || true
  kind delete cluster --name "${KIND_NAME}"
fi

printf '☸️ Creating OIDC-enabled kind cluster %s...\n' "${KIND_NAME}"
KIND_CREATE_ARGS=(
  --name "${KIND_NAME}"
  --config "${RUNTIME_CONFIG}"
  --wait 2m
  --kubeconfig "${KUBECONFIG_PATH}"
)
if [ -n "${NODE_IMAGE}" ]; then
  KIND_CREATE_ARGS+=(--image "${NODE_IMAGE}")
fi
kind create cluster "${KIND_CREATE_ARGS[@]}" &
KIND_PID=$!

for _ in $(seq 1 120); do
  if docker inspect "${CONTROL_PLANE}" >/dev/null 2>&1; then
    start_forwarder
    break
  fi
  sleep 1
done

wait "${KIND_PID}"
install_metrics_server
install_prometheus_api_fixture
