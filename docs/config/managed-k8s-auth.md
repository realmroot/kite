# Managed Kubernetes authentication

Lightkite does not import the provider CLI kubeconfig used by `kubectl`. A managed
cluster is compatible only when its API server can authenticate the same
external OIDC issuer and audience used by Lightkite.

## Compatibility checklist

Confirm that the managed service lets you:

1. trust an external OIDC issuer and its signing keys;
2. accept Lightkite's OIDC client ID as the token audience;
3. configure or predict the username and groups claims;
4. create native RoleBindings/ClusterRoleBindings for those identities; and
5. expose an HTTPS API endpoint reachable directly from Lightkite or through the
   operator-managed private network path.

Add the cluster using only its API URL, CA bundle, optional TLS server name, and
connection mode.

Provider IAM authenticators based on short-lived `exec` plugins are not the
same protocol as direct OIDC authentication. Lightkite will not execute cloud CLIs,
store their tokens, create a privileged ServiceAccount, or impersonate users to
bridge that difference.

If the service cannot trust the external issuer, the cluster is not compatible
with direct user-token propagation as-is. A future provider-specific bridge
would need an independently reviewed workload-identity and token-exchange
design; do not solve this by giving Lightkite `cluster-admin`.

An HTTP 401 from Kubernetes normally means issuer/audience/signature/claim
authentication is misaligned. An HTTP 403 means authentication succeeded and
the exact user or group needs an appropriate Kubernetes RBAC binding.
