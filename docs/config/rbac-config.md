# Kubernetes RBAC configuration

Kubernetes is the sole authorizer for Kubernetes resources. Kite does not
define roles, resource filters, deny rules, or user-role mappings in its own
database.

For a namespace-scoped group:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: application-viewers
  namespace: application
subjects:
  - kind: Group
    name: application-viewers
    apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: view
  apiGroup: rbac.authorization.k8s.io
```

An exact user binding uses `kind: User` and the username emitted by the API
server's configured username claim. Group bindings use the API server's groups
claim. Both are standard Kubernetes RBAC subjects.

Subresources require their own permissions, for example `pods/log`,
`pods/exec`, `services/proxy` for cluster-local Prometheus, and the Helm
release's underlying Secrets plus managed resources. A UI action may therefore
be visible while Kubernetes correctly rejects it with 403.

`PLATFORM_ADMIN_GROUPS` is not Kubernetes RBAC. It controls only shared
Kite-owned metadata and cannot make a denied Kubernetes operation succeed.
