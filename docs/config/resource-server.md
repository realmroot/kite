# External tool access

Kite is a dashboard client. Agent access is exposed by Kube Cluster Hub,
not by the Kite process.

Configure Kite with the Hub root URL:

```text
CLUSTER_GATEWAY_URL=https://clusters.example.com
```

Kite requests the catalog's RFC 8707 resource indicator during Authorization
Code + PKCE login. It uses the resulting Access Token for catalog calls and the
same session's ID Token for Kubernetes calls. Kubernetes RBAC remains
authoritative. The Hub independently publishes RFC 9728
metadata, OpenAPI, DPoP-protected Agent operations, and Agent audit events.

Kite contains no Resource Server signing key, DPoP replay store, Agent token
validator, or privileged Kubernetes execution credential.
