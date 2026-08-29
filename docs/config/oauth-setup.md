# OpenID Connect setup

Kite uses OpenID Connect Authorization Code with PKCE and supports both public
and confidential clients. Configure one issuer per deployment; there is no
provider CRUD in the dashboard.

## Register the client

Prefer a public client that requires PKCE at any standards-compliant provider,
then register this exact redirect URI:

```text
https://kite.example.com/api/auth/callback
```

Set `HOST=https://kite.example.com` so callback and secure-cookie behavior do
not depend on forwarded headers.

## Configure Kite

```text
OIDC_ISSUER=https://identity.example.com
OIDC_CLIENT_ID=kite
# OIDC_CLIENT_SECRET is omitted for a public PKCE client.
OIDC_PROVIDER_NAME=Corporate Identity
OIDC_SCOPES=openid profile email groups offline_access
OIDC_USERNAME_CLAIM=email
OIDC_GROUPS_CLAIM=groups
OIDC_NAME_CLAIM=name
OIDC_PICTURE_CLAIM=picture
PLATFORM_ADMIN_GROUPS=kite-platform-admins
HOST=https://kite.example.com
KITE_ENCRYPT_KEY=<independent random secret>
```

The issuer must expose standard discovery metadata. `OIDC_SCOPES` must include
`openid` and `offline_access`; browser session renewal requires the user's
refresh grant. `HOST` is a required HTTPS origin and is never inferred from
forwarded request headers. The configured claim names are ordinary top-level
ID-token claims; Kite contains no provider-specific claim logic.

Set `OIDC_CLIENT_SECRET` only when the registered application is a confidential
client. PKCE, state, and nonce are enforced in either mode.

`PLATFORM_ADMIN_GROUPS` controls only Kite-owned shared metadata. Login is not
restricted to those groups, and membership does not grant Kubernetes access.

## Align Kubernetes

Configure each API server to trust the same issuer and audience, then align its
username and groups claim settings with the provider. Create Kubernetes
RoleBindings or ClusterRoleBindings for exact users or groups.

Kite forwards the original ID token. It cannot compensate for a managed
Kubernetes service that does not accept the external issuer or audience.

## Troubleshooting

- Discovery failure: verify the issuer's `/.well-known/openid-configuration`.
- Callback rejection: verify the exact HTTPS callback and `HOST`.
- Login succeeds but Kubernetes returns 401: align issuer, audience, signing
  keys, and API-server OIDC flags.
- Kubernetes returns 403: the identity authenticated successfully but lacks the
  required native RBAC binding.
