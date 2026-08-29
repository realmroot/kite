# Kubernetes compatibility

Lightkite follows the three most recent Kubernetes minor releases and validates the
same browser and API end-to-end suite against each release before shipping.

| Kubernetes API server | Client libraries | Release status |
| --- | --- | --- |
| 1.36 | `k8s.io/*` 0.36 | Required release gate |
| 1.35 | `k8s.io/*` 0.36 | Required release gate |
| 1.34 | `k8s.io/*` 0.36 | Required release gate |

The CI matrix uses digest-pinned images published with kind 0.32.0. It covers
OIDC login, Kubernetes RBAC allow and deny behavior, discovery and generic
resources, metrics, Search, Helm, logs, exec, browser terminals, and direct and
reachable clusters. The release workflow repeats the suite on the newest minor.

Lightkite prefers stable Kubernetes APIs and API discovery. Specialized code must
move off deprecated APIs before the oldest supported minor removes them. For
example, Service-to-Pod relationships use `discovery.k8s.io/v1` EndpointSlice;
the legacy `v1 Endpoints` path is used only when EndpointSlice is unavailable.

When Kubernetes publishes a new minor release, update the `k8s.io/*` modules,
add the new digest-pinned kind image, remove the oldest matrix entry, run the
full matrix, and document any user-visible compatibility change. A version is
not listed as supported until its matrix job passes.
