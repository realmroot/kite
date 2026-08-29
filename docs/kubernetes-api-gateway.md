# Kubernetes API gateway

Lightkite exposes the selected cluster's standard Kubernetes HTTP API below a
browser-session boundary:

```text
/api/v1/kubernetes/<kubernetes-api-path>
/api/v1/_clusters/<cluster>/kubernetes/<kubernetes-api-path>
```

For example, listing Deployments in `default` uses:

```text
GET /api/v1/kubernetes/apis/apps/v1/namespaces/default/deployments?limit=100
```

The gateway preserves the HTTP method, query, request body, response status,
response headers, Kubernetes `Status` body, and streaming behavior. The path
after `/kubernetes` is the canonical upstream Kubernetes resource path; Lightkite
does not invent action routes or reinterpret Kubernetes objects.

The browser's Cookie and Authorization headers are removed before forwarding.
The server then injects the current OIDC session's validated ID token. The
target API server is therefore the only component that decides whether a user
may get, list, watch, create, update, patch, delete, exec, or read logs.

Lightkite maintains one credential-free connection pool per cluster. A lightweight
per-request wrapper supplies the user's token, so the number of TLS connection
pools and discovery transports scales with clusters rather than users. Lightkite
does not create per-user informers. Long-running watches remain ordinary
Kubernetes watch requests and clients should reconnect after the API server's
configured timeout.
