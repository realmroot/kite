# External tool access

Kite can expose a standards-based OAuth Resource Server without embedding an
Agent runtime. It is disabled unless `RESOURCE_SERVER_URL` is configured.

```text
RESOURCE_SERVER_URL=https://kite.example.com/api/agent/v1
RESOURCE_SERVER_ISSUER=https://identity.example.com
RESOURCE_SERVER_AUTHORIZED_CLIENT_IDS=agent-protocol-client
RESOURCE_SERVER_JWT_ALGORITHMS=RS256
```

The exact Resource URL returns an RFC 8631 `service-desc` link. RFC 9728
metadata is available at
`/.well-known/oauth-protected-resource/api/agent/v1`, and the linked OpenAPI
contract publishes these scopes:

- `clusters:read` lists Kite's credential-free cluster catalog.
- `kubernetes:read` permits GET and HEAD requests to a selected Kubernetes API.
- `kubernetes:write` permits POST, PUT, PATCH, and DELETE requests.

Scopes are only an outer delegation ceiling. Kite forwards the signed,
audience-bound subject token to the selected Kubernetes API server, and
Kubernetes RBAC remains authoritative. A scope can never override a Kubernetes
denial.

Every protected request requires an `at+jwt` access token using the `DPoP`
authorization scheme and a fresh RFC 9449 ES256 proof. Kite validates issuer,
exact audience, expiry, client ID, scope, actor attribution, key binding,
method, target URI, token hash, and persisted replay state. There is no Bearer
fallback. The controlling subject and stable actor are recorded in the
Resource access audit table.

The interactive browser token and Resource access token use different
audiences. When both tokens have the same issuer, Kubernetes requires one
structured JWT authenticator with both audiences and `MatchAny` (issuer URLs
must be unique within `AuthenticationConfiguration`):

```yaml
apiVersion: apiserver.config.k8s.io/v1
kind: AuthenticationConfiguration
jwt:
  - issuer:
      url: https://identity.example.com
      audiences:
        - kite-browser-client
        - https://kite.example.com/api/agent/v1
      audienceMatchPolicy: MatchAny
    claimMappings:
      username:
        expression: claims.sub
      groups:
        expression: claims.groups
```

Pass this file with `kube-apiserver --authentication-config`. Kubernetes
1.30–1.33 use `apiserver.config.k8s.io/v1beta1`; Kubernetes 1.34 and later use
`v1`. Do not combine `--authentication-config` with `--oidc-*` flags. Bind the
resulting subjects or groups to Kubernetes Roles using normal RBAC.
