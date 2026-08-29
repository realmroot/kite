# Configuration file

The optional YAML configuration file manages the credential-free cluster
catalog declaratively. OIDC client settings and platform policy are deployment
environment settings, while Kubernetes authorization remains in Kubernetes.

Set `KITE_CONFIG_FILE=/etc/kite/config.yaml`. Unknown fields are rejected so a
legacy kubeconfig, identity-provider, or Kite-RBAC section cannot be accepted
silently.

## Schema

```yaml
clusters:
  - name: production
    description: Production cluster
    apiServerUrl: https://api.production.example:6443
    caBundle: |
      -----BEGIN CERTIFICATE-----
      ...
      -----END CERTIFICATE-----
    tlsServerName: api.production.example
    prometheusURL: http://prometheus.monitoring.svc.cluster.local:9090
    default: true
```

| Field | Required | Description |
| --- | --- | --- |
| `name` | Yes | Unique display name |
| `description` | No | Human-readable context |
| `apiServerUrl` | Yes | Credential-free Kubernetes HTTPS API endpoint |
| `caBundle` | No | PEM or base64-encoded PEM trust bundle |
| `tlsServerName` | No | TLS name override for private endpoints |
| `prometheusURL` | No | Prefer a cluster-local Service URL; see the Prometheus guide |
| `default` | No | Select this cluster by default |

The file must not contain kubeconfig data, bearer tokens, client certificates,
client keys, ServiceAccount tokens, OAuth client secrets, users, LDAP settings,
or role mappings.

When `clusters` is present, the file is authoritative for the cluster catalog
and the corresponding UI becomes read-only. Kite watches the file and applies
valid changes transactionally. Existing names are updated in place so their
stable cluster identity and resource history remain intact. Removing a cluster
deletes its catalog row, but does not change resources
inside that Kubernetes cluster. Invalid startup configuration prevents Kite
from becoming ready; an invalid hot reload leaves the last valid catalog and
connections active until the file is corrected.

## Helm chart

Use an existing Secret:

```bash
kubectl create secret generic kite-config \
  --from-file=config.yaml=./config.yaml
```

```yaml
config:
  enabled: true
  existingSecret: kite-config
```

Or place the same `clusters` array below `config` in chart values. Sensitive
OIDC and database values belong in the chart's application Secret, not this
catalog file.
