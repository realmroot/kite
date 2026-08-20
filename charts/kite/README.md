# Kite Helm chart

This chart deploys the standards-based OIDC Kubernetes dashboard without a
mounted ServiceAccount token or dashboard-owned Kubernetes RBAC policy.

Install the immutable chart version published by your fork:

```bash
helm upgrade --install kite oci://ghcr.io/<owner>/charts/kite \
  --version <version> \
  --namespace kite-system \
  --create-namespace \
  --values values.yaml
```

The source chart intentionally has no default `image.repository`; release
packaging writes the publishing repository into the chart. Set it explicitly
when rendering directly from a checkout.

SQLite uses a one-replica `Recreate` deployment and a PVC by default. Use
PostgreSQL or MySQL before enabling multiple replicas or rolling updates.
The optional kubectl and node terminal images are versioned, and the generated
Cluster Agent manifest uses this release's own Kite image unless overridden.
Update checks make no outbound request unless `releaseAPIURL` is configured.
Set `imageRegistryHosts` when image-tag lookup must contact a private or
self-hosted registry; arbitrary registry hosts are denied by default.
Analytics has no built-in destination; configure `analytics.scriptURL` and
`analytics.websiteID` for an operator-owned endpoint before enabling it.

See [`docs/config/chart-values.md`](../../docs/config/chart-values.md) for the
identity, database, private-CA, ingress, and security values.

Uninstall the workload while retaining its PVC according to Kubernetes and
Helm retention behavior:

```bash
helm uninstall kite --namespace kite-system
```
