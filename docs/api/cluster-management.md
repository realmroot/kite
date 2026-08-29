# Cluster catalog API

The cluster catalog stores transport and presentation metadata only. Endpoints
under `/api/v1/admin/clusters/` require an authenticated OIDC session whose
groups match `PLATFORM_ADMIN_GROUPS`. This platform permission does not grant
Kubernetes resource access.

## Stored fields

| Field | Description |
| --- | --- |
| `name` | Unique display name |
| `description` | Optional context |
| `connectionMode` | `direct` or `tunnel` |
| `apiServerUrl` | HTTPS Kubernetes endpoint for direct mode |
| `caBundle` | Optional PEM or base64-encoded PEM trust bundle |
| `tlsServerName` | Optional TLS name override |
| `prometheusURL` | Optional Prometheus Service URL |
| `isDefault` | Default selection |
| `enabled` | Whether users may select the entry |

Kubeconfig, token, username/password, client-certificate, client-key, and
ServiceAccount fields are not accepted. Unknown JSON fields are rejected.

## List

```http
GET /api/v1/admin/clusters/
```

The response includes the fields above plus `id`, tunnel connection status,
and tunnel-agent version where applicable. It never returns enrollment secrets
or Kubernetes credentials.

Authenticated users list selectable names with:

```http
GET /api/v1/clusters
```

## Create

```http
POST /api/v1/admin/clusters/
Content-Type: application/json
```

Direct example:

```json
{
  "name": "production",
  "description": "Production cluster",
  "connectionMode": "direct",
  "apiServerUrl": "https://api.production.example:6443",
  "caBundle": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
  "tlsServerName": "api.production.example",
  "prometheusURL": "http://prometheus.monitoring.svc.cluster.local:9090",
  "isDefault": true
}
```

`apiServerUrl` must be HTTPS. The direct endpoint must be reachable from Kite.

Tunnel example:

```json
{
  "name": "private",
  "description": "Private network cluster",
  "connectionMode": "tunnel",
  "isDefault": false
}
```

The tunnel response contains a short-lived encrypted
`clusterAgentManifestURL`. Download and apply it promptly. The URL grant
expires and is not returned by later list calls. The generated agent is
transport-only and has no mounted ServiceAccount
token or Kubernetes RBAC grant.

## Update and delete

```http
PUT    /api/v1/admin/clusters/:id
DELETE /api/v1/admin/clusters/:id
```

Update accepts `name`, `description`, `apiServerUrl`, `caBundle`,
`tlsServerName`, `prometheusURL`, `isDefault`, and `enabled`. Connection
mode is immutable; replace the catalog entry to change direct versus tunnel.
The default entry cannot be deleted until another entry becomes default.
Deleting a non-default cluster releases the catalog name for reuse. Historical
audit metadata is retained under the stable cluster identity recorded when the
operation occurred.

If the catalog is managed by `KITE_CONFIG_FILE`, create, update, and delete
return 403 and changes must be made in that file.
