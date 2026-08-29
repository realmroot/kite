# Authentication API

Lightkite has one standards-compliant OpenID Connect provider configured by the
operator. It does not expose password, LDAP, passkey, MFA-provider management,
OAuth-provider CRUD, or API-key authentication endpoints.

## Browser flow

`GET /api/auth/login` creates an Authorization Code + PKCE transaction and
returns the provider authorization URL:

```json
{
  "auth_url": "https://identity.example.com/authorize?...",
  "provider": "OpenID Connect"
}
```

The browser navigates to `auth_url`. The provider redirects to
`GET /api/auth/callback`; Lightkite validates state, nonce, issuer, signature,
audience, and token lifetime before creating the session.

The browser receives only an opaque `HttpOnly`, `SameSite=Lax` cookie. ID,
access, and refresh tokens remain encrypted in the server-side session. HTTPS
deployments also set the cookie's `Secure` attribute.

## Session endpoints

```text
GET  /api/auth/user
POST /api/auth/refresh
POST /api/auth/logout
GET  /api/v1/bootstrap
```

`GET /api/auth/user` returns the local presentation profile, whether the user
may administer Lightkite-owned shared metadata, and effective sidebar preferences.
It does not return provider tokens.

`POST /api/auth/refresh` refreshes and revalidates the upstream OIDC session.
`POST /api/auth/logout` deletes the server-side session and expires the cookie.

## Authorization boundary

The OIDC ID token from the current session is attached to that user's
Kubernetes request. Kubernetes authenticates the token and authorizes the user
or groups with native RBAC. Lightkite roles and API keys are not part of this path.

`PLATFORM_ADMIN_GROUPS` is a separate policy for Lightkite-owned shared metadata,
such as the cluster catalog and global preferences. It cannot grant any
Kubernetes permission.

See [OIDC-native Kubernetes architecture](../oidc-kubernetes.md) for deployment
and claim-mapping requirements.
