# Installation Guide

This guide provides detailed instructions for installing Lightkite in a Kubernetes environment.

## Prerequisites

- `kubectl` with cluster administrator privileges
- Helm v4, or Helm v3.8+ for OCI chart installation
- MySQL/PostgreSQL database, or local storage for sqlite

## Installation Methods

### Method 1: Helm Chart (Recommended)

Install Lightkite into a new release and namespace. Do not apply this chart over
an existing Kite release.

Install the versioned OCI chart published by Lightkite (replace `<version>`):

```bash
helm install lightkite oci://ghcr.io/realmroot/charts/lightkite \
  --version <version> -n lightkite-system --create-namespace -f values.yaml
```

If the repository enables its optional GitHub Pages Helm index, you may instead
install that same version from the index:

```bash
helm repo add lightkite https://realmroot.github.io/lightkite/
helm repo update
helm install lightkite lightkite/lightkite --version <version> \
  -n lightkite-system --create-namespace -f values.yaml
```

#### Custom Installation

You can adjust installation parameters by customizing the values file:

For complete configuration, refer to [Chart Values](../config/chart-values).

Install with custom values:

```bash
helm install lightkite oci://ghcr.io/realmroot/charts/lightkite \
  --version <version> -n lightkite-system --create-namespace -f values.yaml

# Or use the Helm repository
helm install lightkite lightkite/lightkite --version <version> \
  -n lightkite-system --create-namespace -f values.yaml
```

### Method 2: YAML Manifest

Each release includes a versioned `install.yaml` with persistent SQLite storage
and a non-root security context. Download it, replace the OIDC/secret/host
placeholders, review it, and then apply it:

```bash
curl -fLO https://github.com/realmroot/lightkite/releases/download/vX.Y.Z/install.yaml
$EDITOR install.yaml
kubectl apply -f install.yaml
```

For external databases, private issuer CAs, ingress, or other advanced
configuration, use the Helm Chart.

## Accessing Lightkite

### Port Forwarding (Testing Environment)

During testing, you can quickly access Lightkite through port forwarding:

```bash
kubectl port-forward -n lightkite-system svc/lightkite 8080:8080
```

### LoadBalancer Service

If the cluster supports LoadBalancer, you can directly expose the Lightkite service:

```bash
kubectl patch svc lightkite -n lightkite-system -p '{"spec": {"type": "LoadBalancer"}}'
```

Get the assigned IP:

```bash
kubectl get svc lightkite -n lightkite-system
```

### Ingress (Recommended for Production)

For production environments, it's recommended to expose Lightkite through an Ingress controller with TLS enabled:

::: warning
Lightkite's log and web terminal features require websocket support.
Some Ingress controllers may require additional configuration to handle websockets correctly.
:::

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: lightkite
  namespace: lightkite-system
spec:
  ingressClassName: nginx
  rules:
    - host: lightkite.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: lightkite
                port:
                  number: 8080
  tls:
    - hosts:
        - lightkite.example.com
      secretName: lightkite-tls
```

## Serving under a subpath (basePath)

If you want to serve Lightkite under a subpath (for example `https://example.com/lightkite`), use the Helm chart `basePath` value.

How to set it:

- In `values.yaml`:

```yaml
basePath: "/lightkite"
```

- Or with Helm CLI:

```fish
helm install lightkite oci://ghcr.io/realmroot/charts/lightkite \
  --version <version> -n lightkite-system --create-namespace \
  -f values.yaml --set basePath="/lightkite"
```

Important notes:

- Ingress configuration: make sure your Ingress `paths` match the subpath and use a matching pathType (e.g., `Prefix`). Example:

```yaml
ingress:
  enabled: true
  hosts:
    - host: lightkite.example.com
      paths:
        - path: /lightkite
          pathType: Prefix
```

- OIDC callback: register the base path when used, for example
  `https://lightkite.example.com/lightkite/api/auth/callback`.
- Environment overrides: if you provide environment variables via `extraEnvs` or an existing secret, ensure `KITE_BASE` is set consistently with the `basePath` value (otherwise behavior may differ).

## Verifying Installation

After installation, open Lightkite and sign in through the configured OIDC provider.
The Overview page should load after the provider redirects back to Lightkite.

::: tip
If you need to configure Lightkite through environment variables, please refer to [Environment Variables](../config/env).
:::

### Add the first cluster

An operator in `PLATFORM_ADMIN_GROUPS` can open **Settings > Clusters** and add
a direct cluster using only its API server URL, CA bundle, and optional TLS
server name. Private APIs require operator-managed network connectivity.
Lightkite does not use its Pod ServiceAccount as a cluster credential. The
cluster API server must trust the same OIDC issuer, and Kubernetes RBAC must
bind the signed-in users or groups.

## Uninstalling Lightkite

### Helm Uninstall

```bash
helm uninstall lightkite -n lightkite-system
```

### YAML Uninstall

```bash
kubectl delete -f install.yaml
```

## Next Steps

After Lightkite installation is complete, you can continue with:

- [Adding Users](../config/user-management)
- [Configuring RBAC](../config/rbac-config)
- [Configuring OAuth Authentication](../config/oauth-setup)
- [Setting up Prometheus Monitoring](../config/prometheus-setup)
