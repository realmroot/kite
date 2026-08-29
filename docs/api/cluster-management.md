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
| `apiServerUrl` | HTTPS Kubernetes endpoint reachable from Lightkite |
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

The response includes the fields above plus `id`. It never returns Kubernetes
credentials.

Authenticated users list selectable names with:

```http
GET /api/v1/clusters
```

## Create

```http
POST /api/v1/admin/clusters/
Content-Type: application/json
```

```json
{
  "name": "production",
  "description": "Production cluster",
  "apiServerUrl": "https://api.production.example:6443",
  "caBundle": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
  "tlsServerName": "api.production.example",
  "prometheusURL": "http://prometheus.monitoring.svc.cluster.local:9090",
  "isDefault": true
}
```

`apiServerUrl` must be HTTPS and reachable from Lightkite. Private network routing,
DNS, and any tunnel are deployment infrastructure, not a Lightkite protocol.

## Update and delete

```http
PUT    /api/v1/admin/clusters/:id
DELETE /api/v1/admin/clusters/:id
```

Update accepts `name`, `description`, `apiServerUrl`, `caBundle`,
`tlsServerName`, `prometheusURL`, `isDefault`, and `enabled`.
The default entry cannot be deleted until another entry becomes default.
Deleting a non-default cluster releases the catalog name for reuse. Historical
audit metadata is retained under the stable cluster identity recorded when the
operation occurred.

If the catalog is managed by `KITE_CONFIG_FILE`, create, update, and delete
return 403 and changes must be made in that file.
