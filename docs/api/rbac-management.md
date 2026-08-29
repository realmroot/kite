# Authorization

Lightkite does not provide an application-RBAC management API. Kubernetes resource
authorization is configured with native `Role`, `ClusterRole`, `RoleBinding`,
and `ClusterRoleBinding` objects through Kubernetes itself.

Bindings may target an exact OIDC user or an OIDC group. The values must match
the API server's configured username and groups claims. Lightkite forwards the
current user's signed ID token and does not impersonate a different principal.

Lightkite-owned shared metadata has a separate, deliberately narrow policy:
members of `PLATFORM_ADMIN_GROUPS` may maintain the cluster catalog, shared
templates/preferences, Helm repository metadata, and audit views. This policy
never expands the user's Kubernetes permissions.

The platform audit view contains attribution and operation metadata only. It
does not return copied Kubernetes YAML. YAML history is available only from the
specific resource history route after Kubernetes authorizes `get` for that
resource, and Secret bodies are never retained there.

Use Kubernetes `SelfSubjectAccessReview` or `kubectl auth can-i` with the same
OIDC identity when diagnosing resource authorization.
