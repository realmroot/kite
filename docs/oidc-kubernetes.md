# OIDC-native Kubernetes architecture

## Security model

Lightkite is an OIDC relying party and a browser-facing backend-for-frontend. It is
not a Kubernetes identity provider or authorization proxy.

1. The browser starts the OIDC Authorization Code flow with PKCE, state, and
   nonce.
2. Lightkite validates issuer, signature, audience, lifetime, and nonce, then keeps
   ID, access, and refresh tokens in an encrypted server-side session. While
   the signed-in browser remains active, Lightkite renews the opaque HttpOnly
   session cookie and server-side activity timestamp; provider token refresh
   remains entirely server-side.
3. The browser receives only an opaque, `HttpOnly`, `SameSite=Lax` session
   cookie.
4. Lightkite keeps one credential-free connection runtime and HTTP transport per
   cluster. Each request wraps that shared transport with the signed-in user's
   OIDC ID token; tokens and Kubernetes clients are never shared between users.
   Lightkite starts no informer cache.
5. The Kubernetes API server authenticates the token and authorizes the
   configured username and group claims through native RBAC.

When Cluster Inventory is enabled, Lightkite watches `ClusterProfile` resources in
one Inventory Kubernetes API. Each profile supplies a credential-free access
provider endpoint. The ID Token remains the only user credential sent through
that Kubernetes-compatible endpoint and stays inside the encrypted server-side
session. Lightkite requests no provider-specific catalog scope.

`PLATFORM_ADMIN_GROUPS` controls only who may maintain Lightkite's shared cluster
catalog, templates, and audit view. It does not grant access to Kubernetes
resources. Kubernetes RoleBindings and ClusterRoleBindings remain authoritative.

Embedded AI/Agent execution, local password and passkey login, LDAP, local
OAuth-provider management, Lightkite RBAC, API keys, and credential-bearing
kubeconfig import are intentionally unavailable in this fork. Browser kubectl,
node terminal, and other inherited Kubernetes operations remain available while
they undergo explicit product and security review; they must use the current
user's Kubernetes identity and do not receive a shared ServiceAccount.

Ordinary Kubernetes API calls are also available through the transparent
[Kubernetes API gateway](kubernetes-api-gateway.md). Compatibility endpoints
used by the existing UI follow the same per-request identity rule.

## Required Lightkite configuration

```text
OIDC_ISSUER=https://identity.example.com
OIDC_CLIENT_ID=<public PKCE or confidential client ID>
# OIDC_CLIENT_SECRET=<optional confidential-client secret>
OIDC_PROVIDER_NAME=Corporate Identity
OIDC_SCOPES=openid profile email groups offline_access
OIDC_USERNAME_CLAIM=email
OIDC_GROUPS_CLAIM=groups
OIDC_NAME_CLAIM=name
OIDC_PICTURE_CLAIM=picture
PLATFORM_ADMIN_GROUPS=platform-admins
KITE_ENCRYPT_KEY=<random secret used for encrypted server sessions>
HOST=https://lightkite.example.com
```

Register `${HOST}/api/auth/callback` as the OIDC callback. The configured scopes
must contain `openid`. Claim names are top-level ID-token claims and are not
hard-coded; common group alternatives include `groups`, `roles`, and `memberOf`.

Lightkite does not rewrite the ID token. `OIDC_USERNAME_CLAIM` selects Lightkite's local
display identity and `OIDC_GROUPS_CLAIM` selects catalog-administrator groups;
the API server's `--oidc-username-claim` and `--oidc-groups-claim` independently
control Kubernetes identity. Configure both sides consistently to avoid an
operator seeing a different username or group interpretation in each layer.

## Kubernetes API server OIDC configuration

For a self-managed API server, configure equivalent flags:

```text
--oidc-issuer-url=https://identity.example.com
--oidc-client-id=<the same OIDC client ID>
--oidc-username-claim=email
--oidc-groups-claim=groups
--authorization-mode=Node,RBAC
```

Then bind provider users or groups with normal Kubernetes RBAC. For example:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: oidc-platform-admins
subjects:
  - kind: Group
    name: platform-admins
    apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: cluster-admin
  apiGroup: rbac.authorization.k8s.io
```

Managed Kubernetes offerings must support trusting the configured external OIDC
issuer and accept the configured audience. If a provider only supports its
own IAM authenticator, this direct-token architecture requires a provider-
specific authentication bridge and is not compatible as-is.

## Cluster catalog

Lightkite can combine two sources: its existing local credential-free cluster
configuration and standard Cluster Inventory profiles. Inventory profiles are
watched by one process-wide shared informer, are not copied into Lightkite's
database, and are managed at their publisher rather than through Lightkite settings.
Disabling Inventory leaves the standalone local source unchanged.

Each cluster row stores only:

- display name and description;
- HTTPS API server URL reachable from Lightkite;
- CA bundle and optional TLS server-name override;
- optional cluster-local Prometheus service URL;
- enabled/default flags.

It never accepts or returns a kubeconfig, bearer token, client certificate, or
client key. Existing upstream installations must re-register clusters using
transport metadata; privileged kubeconfigs are not migrated.

Lightkite requires network reachability to the API server. Private routing and any
tunnel are operator-owned deployment infrastructure; Lightkite does not implement
an enrollment protocol or in-cluster connector.

Prometheus is enabled only for `.svc`/`.svc.cluster.local` service URLs. Lightkite
reaches that service through the Kubernetes service proxy using the current
user token, so the API server must authorize `services/proxy`. Direct anonymous
or shared-credential access to an external Prometheus endpoint is deliberately
disabled because it cannot preserve Kubernetes RBAC boundaries.

Browser kubectl sessions require the user to create, get, and delete Pods and
Secrets in `kube-system`, and to use `pods/exec`. Their generated kubeconfig
contains only the current OIDC token and is deleted with the session. Node
terminal sessions require Node read access plus permission to create, get,
delete, and exec into a privileged Pod in `kube-system`. Admission policy may
deny that Pod. Node terminal is host-root access by design; grant these
permissions only to a dedicated operator group.

## Helm installation

Set the chart's `oidc` and secret values. With `clusterInventory.enabled=false`,
the Lightkite ServiceAccount token is not mounted. Enabling it mounts the token and
installs read-only get/list/watch permission for `ClusterProfile` resources in
the selected namespace. Apply provider-user and provider-group bindings for
target cluster resources separately.
