# Kubernetes resources

Kite's browser client uses the Kubernetes API itself. Kite does not publish or
maintain a second CRUD contract for built-in or custom resources.

## Gateway

An authenticated browser session can address any Kubernetes API path below:

```text
/api/v1/kubernetes/<kubernetes-api-path>
/api/v1/_clusters/<cluster>/kubernetes/<kubernetes-api-path>
```

The gateway preserves the method, query string, request body, response status,
Kubernetes `Status` object, streaming body, and content type. It removes browser
cookies and credentials before using the current user's cluster-bound OIDC
transport. Kubernetes remains the only resource authorizer.

Examples:

```text
GET    /api/v1/kubernetes/api/v1/namespaces/default/configmaps
GET    /api/v1/kubernetes/apis/apps/v1/namespaces/default/deployments/example
POST   /api/v1/kubernetes/api/v1/namespaces/default/configmaps
PUT    /api/v1/kubernetes/api/v1/namespaces/default/configmaps/example
PATCH  /api/v1/kubernetes/apis/apps/v1/namespaces/default/deployments/example
DELETE /api/v1/kubernetes/api/v1/namespaces/default/configmaps/example
```

Use Kubernetes-native media types and request shapes. For example, a merge
patch uses `Content-Type: application/merge-patch+json`; delete propagation and
grace periods use a Kubernetes `DeleteOptions` body. Lists use Kubernetes
`limit`, `continue`, `labelSelector`, `fieldSelector`, and `watch` parameters.

The frontend catalog maps built-in resources to their canonical group and
version. For a custom resource, it reads the CRD through
`apiextensions.k8s.io/v1`, selects its storage (or first served) version, and
uses the CRD's plural and scope. Adding a CRD therefore does not add a Kite
handler.

Pod and Node table metrics are client-side compositions of the standard core
resources and `metrics.k8s.io/v1beta1`. A missing or unauthorized Metrics API
does not prevent the underlying resources from being displayed.

## Kite-specific resource operations

Kite retains narrow APIs only when the capability is not one Kubernetes
resource operation:

- multi-document YAML apply and history rollback orchestration;
- resource history and audit presentation;
- `kubectl describe`-compatible aggregate output;
- related-resource aggregation;
- workload revision presentation and rollback orchestration;
- Node drain, which coordinates cordon and multiple Pod evictions;
- Pod file browsing and transfer over exec;
- Helm release operations, because a Helm release is not a Kubernetes API
  resource.

These APIs do not replace ordinary Kubernetes list/get/create/update/patch/delete
operations.

## Resource history

Built-in and custom-resource detail routes expose a paginated history collection
at `/<resource>/<namespace>/<name>/history` (use `_all` for a cluster-scoped
resource). Before reading Kite's history database, the backend submits a
Kubernetes `SelfSubjectAccessReview` for `get` on that exact resource group,
plural, namespace, and name with the current user's token. A denied review
returns `403`; a platform-management role does not bypass this check.

Mutations sent through the standard Kubernetes gateway are recorded at that
single boundary. History pages accept one-based `page` and `pageSize` values;
`pageSize` is limited to 100. Kubernetes Secret operations retain attribution
and success or failure metadata, but Kite does not persist Secret YAML bodies or
raw Secret error details. A rollback from resource history is a new apply
operation and is authorized again by Kubernetes.

History is bound to the immutable catalog cluster ID rather than only its
display name. Renaming a cluster preserves its history, while deleting and later
recreating the same name cannot attach old YAML to the new cluster.
