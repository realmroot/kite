#!/usr/bin/env bash

set -euo pipefail

cd "$(dirname "$0")/.."

client_version="$(awk '$1 == "k8s.io/client-go" {print $2; exit}' go.mod)"
client_minor="$(printf '%s' "${client_version}" | sed -E 's/^v0\.([0-9]+)\..*$/\1/')"
if ! [[ "${client_minor}" =~ ^[0-9]+$ ]]; then
  echo "cannot determine Kubernetes minor from client-go ${client_version}" >&2
  exit 1
fi

for offset in 0 1 2; do
  minor="$((client_minor - offset))"
  if ! rg -q "kubernetes: \"1\.${minor}\"" .github/workflows/e2e.yml; then
    echo "Kubernetes 1.${minor} is missing from the E2E compatibility matrix" >&2
    exit 1
  fi
  if ! rg -q "\| 1\.${minor} \|" docs/architecture/kubernetes-compatibility.md; then
    echo "Kubernetes 1.${minor} is missing from the documented compatibility matrix" >&2
    exit 1
  fi
done

if ! rg -q "E2E_NODE_IMAGE: kindest/node:v1\.${client_minor}\." .github/workflows/release.yaml; then
  echo "the release gate is not pinned to the client-go Kubernetes minor" >&2
  exit 1
fi

if ! rg -q 'discoveryv1\.EndpointSlice' pkg/resources/related_resources.go; then
  echo "service relationship discovery is not using stable EndpointSlice" >&2
  exit 1
fi
