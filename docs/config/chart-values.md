# Helm chart values

The chart deploys Lightkite without a mounted ServiceAccount token and does not
create Kubernetes RBAC grants for dashboard resource access. Configure OIDC and
bind users/groups in every target cluster separately.

## Required identity values

| Value | Description |
| --- | --- |
| `image.repository` | Container repository; required when rendering the chart from source |
| `oidc.issuer` | Standard OpenID Connect issuer URL |
| `oidc.clientId` | Public PKCE or confidential client ID |
| `oidc.clientSecret` | Optional confidential-client secret; empty for a public PKCE client |
| `platformAdminGroups` | Groups allowed to manage Lightkite-owned shared metadata; use a JSON string array when a group contains spaces, commas, or other punctuation |
| `platformAdminSubjects` | Exact OIDC `sub` values with the same platform access; accepts the same JSON string-array form |
| `encryptKey` | Random key used to encrypt server-side provider tokens |

Optional OIDC mappings are `oidc.providerName`, `oidc.scopes`,
`oidc.usernameClaim`, `oidc.groupsClaim`, `oidc.nameClaim`, and
`oidc.pictureClaim`. None is provider-specific.

For a private issuer CA, create a Secret containing the PEM certificate and set
`oidc.ca.existingSecret`. `oidc.ca.key` defaults to `ca.crt`; the chart mounts it
read-only and configures `OIDC_CA_FILE`.

For production, prefer `secret.create=false` and `secret.existingSecret`.
The existing Secret must contain `OIDC_CLIENT_ID`, `KITE_ENCRYPT_KEY`,
and database keys when applicable. Add `OIDC_CLIENT_SECRET` only
for a confidential client.

## Runtime and exposure

| Value | Default | Description |
| --- | --- | --- |
| `replicaCount` | `1` | Application replicas |
| `image.repository` | empty in source | Container repository; release-packaged charts set their publisher repository |
| `image.tag` | Chart appVersion | Container tag |
| `deploymentStrategy.type` | `Recreate` | Safe default for one-replica SQLite; external databases may use `RollingUpdate` |
| `host` | required | Public HTTPS origin used for the OIDC callback; paths are rejected |
| `basePath` | empty | Optional URL path prefix |
| `service.type` | `ClusterIP` | Service type |
| `service.port` | `8080` | HTTP port |
| `ingress.enabled` | `false` | Create an Ingress |
| `gateway.enabled` | `false` | Create Gateway API resources |
| `debug` | `false` | Enable verbose application logging |
| `terminalImages.kubectl` | `alpine/kubectl:1.36.3` | Versioned shell + kubectl terminal image |
| `terminalImages.node` | `busybox:1.37.0` | Versioned node terminal image |
| `imageRegistryHosts` | empty | Additional comma-separated registry `host[:port]` values allowed for image-tag lookup |
| `releaseAPIURL` | empty | Optional GitHub-compatible update API; empty disables outbound checks |
| `analytics.enabled` | `false` | Load the configured operator-owned analytics script |
| `analytics.scriptURL` | empty | HTTPS Umami-compatible script URL; configure together with `analytics.websiteID` |
| `analytics.websiteID` | empty | Operator-owned analytics website ID |

Ingress/Gateway TLS termination must preserve the public host and protocol.
Set `host` explicitly in production.

## Database

| Value | Default | Description |
| --- | --- | --- |
| `db.type` | `sqlite` | `sqlite`, `postgres`, or `mysql` |
| `db.dsn` | empty | Required external database DSN |
| `db.sqlite.persistence.pvc.enabled` | `true` | Persist SQLite on a PVC |
| `db.sqlite.persistence.pvc.existingClaim` | empty | Reuse a PVC |
| `db.sqlite.persistence.pvc.storageClass` | empty | Requested StorageClass |
| `db.sqlite.persistence.pvc.size` | `1Gi` | Requested size |
| `db.sqlite.persistence.mountPath` | `/data` | SQLite mount path |
| `db.sqlite.persistence.filename` | `lightkite.db` | SQLite filename |

Production multi-replica deployments require PostgreSQL or MySQL. The chart
rejects multiple SQLite replicas, a rolling SQLite PVC deployment, conflicting
PVC/hostPath storage, unsupported database types, and a generated external-
database Secret without a DSN.
SQLite runs through one application connection with foreign keys, a busy
timeout, and WAL enabled; this avoids lock races in OIDC session transactions.

## Credential-free cluster catalog

`config.enabled` mounts a declarative catalog. Use
`config.existingSecret` for a Secret containing `config.yaml`, or define
`config.clusters` inline. Only the fields documented in
[Configuration file](./config-file.md) are accepted; kubeconfigs and identity
policy are rejected.

## Pod identity and security

| Value | Default | Description |
| --- | --- | --- |
| `serviceAccount.create` | `true` | Create an identity for the Lightkite Pod |
| `serviceAccount.automount` | `false` | Mount a Kubernetes API token; keep disabled |
| `podSecurityContext` | non-root UID/GID 65532, RuntimeDefault seccomp | Pod security context |
| `securityContext` | no privilege escalation, read-only root, all capabilities dropped | Container security context |
| `resources` | `{}` | Requests and limits |
| `nodeSelector`, `affinity`, `tolerations` | empty | Scheduling controls |
| `extraEnvs` | empty | Additional application environment variables |
| `volumes`, `volumeMounts` | empty | Additional mounts |

The authoritative exhaustive defaults, including Gateway API and probe
structures, are in `charts/lightkite/values.yaml`.
