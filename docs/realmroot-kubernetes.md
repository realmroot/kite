# Realmroot-native Kubernetes architecture

## Security model

Kite is an OIDC relying party and a browser-facing backend-for-frontend. It is
not a Kubernetes identity provider or authorization proxy.

1. The browser starts Realmroot Authorization Code flow with PKCE, state, and
   nonce.
2. Kite validates issuer, signature, audience, lifetime, and nonce, then keeps
   ID, access, and refresh tokens in an encrypted server-side session.
3. The browser receives only an opaque, `HttpOnly`, `SameSite=Lax` session
   cookie.
4. For every Kubernetes request, Kite constructs a fresh client using the
   signed-in user's Realmroot ID token. No cross-user Kubernetes client or
   informer cache is shared.
5. The Kubernetes API server authenticates the token and authorizes its email
   and `groups` claims through native RBAC.

`REALMROOT_ADMIN_GROUPS` controls only who may maintain Kite's shared cluster
catalog, templates, and audit view. It does not grant access to Kubernetes
resources. Kubernetes RoleBindings and ClusterRoleBindings remain authoritative.

The node terminal, browser kubectl console, AI execution routes, local password
and passkey login, LDAP, local OAuth-provider management, Kite RBAC, API keys,
and kubeconfig import are intentionally unavailable in this fork.

## Required Kite configuration

```text
REALMROOT_ISSUER=https://id.realmroot.dev/api/auth
REALMROOT_CLIENT_ID=<confidential web application client ID>
REALMROOT_CLIENT_SECRET=<client secret>
REALMROOT_ADMIN_GROUPS=platform-admins
KITE_ENCRYPT_KEY=<random secret used for encrypted server sessions>
JWT_SECRET=<independent random secret used for tunnel enrollment grants>
HOST=https://kite.example.com
```

Register `${HOST}/api/auth/callback` as the Realmroot callback. Request the
`openid profile email groups offline_access` scopes. `groups` must be present in
the ID token.

## Kubernetes API server OIDC configuration

For a self-managed API server, configure equivalent flags:

```text
--oidc-issuer-url=https://id.realmroot.dev/api/auth
--oidc-client-id=<the same Realmroot client ID>
--oidc-username-claim=email
--oidc-groups-claim=groups
--authorization-mode=Node,RBAC
```

Then bind Realmroot groups with normal Kubernetes RBAC. For example:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: realmroot-platform-admins
subjects:
  - kind: Group
    name: platform-admins
    apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: cluster-admin
  apiGroup: rbac.authorization.k8s.io
```

Managed Kubernetes offerings must support trusting Realmroot as an external
OIDC issuer and accept the configured audience. If a provider only supports its
own IAM authenticator, this direct-token architecture requires a provider-
specific authentication bridge and is not compatible as-is.

## Cluster catalog

Each cluster row stores only:

- display name and description;
- connection mode: `direct` or `tunnel`;
- HTTPS API server URL for direct mode;
- CA bundle and optional TLS server-name override;
- optional Prometheus URL;
- enabled/default flags;
- tunnel enrollment keys and connection metadata for tunnel mode.

It never accepts or returns a kubeconfig, bearer token, client certificate, or
client key. Existing upstream installations must re-register clusters using
transport metadata; privileged kubeconfigs are not migrated.

Direct mode requires network reachability from Kite to the API server. Tunnel
mode deploys a transport-only agent in the cluster. Its manifest has no
ServiceAccount token, Role, ClusterRole, RoleBinding, or ClusterRoleBinding;
the end user's token traverses the tunnel to the API server unchanged.

## Helm installation

Set the chart's Realmroot values and secret values. The Kite ServiceAccount has
`automountServiceAccountToken: false`, and the chart does not install Kubernetes
RBAC for Kite. Apply your Realmroot-group bindings separately.
