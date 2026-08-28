# External tool access

Kite is a dashboard client. Agent access is exposed by Cluster Access Gateway,
not by the Kite process.

Configure Kite with the Gateway root URL:

```text
CLUSTER_GATEWAY_URL=https://clusters.example.com
```

Kite uses the Gateway catalog as the cluster directory and sends the signed-in
user's Kubernetes OIDC ID token to the selected Gateway access URL. Kubernetes
RBAC remains authoritative. The Gateway independently publishes RFC 9728
metadata, OpenAPI, DPoP-protected Agent operations, and Agent audit events.

Kite contains no Resource Server signing-key, DPoP replay store, Agent token
validator, or privileged Kubernetes execution credential.
