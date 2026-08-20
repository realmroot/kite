# Private-cluster tunnel

Tunnel mode connects a private Kubernetes API server to Kite without giving
Kite or the tunnel Agent a Kubernetes identity. The Agent is a transport
component, not a controller and not an authorization proxy.

## Data path

1. A platform administrator creates a cluster with `connectionMode: tunnel`.
2. Kite returns a short-lived encrypted manifest URL.
3. The generated Deployment opens an authenticated reverse tunnel to Kite and
   registers only API-server URL, CA data, TLS server name, and Agent version.
4. For each dashboard request, Kite sends the current user's original OIDC ID
   token through that tunnel.
5. The Kubernetes API server authenticates the user and applies native RBAC.

The Agent never sends a kubeconfig, bearer token, client certificate, client
key, or ServiceAccount token to Kite. The generated Pod sets
`automountServiceAccountToken: false`, has no Role/ClusterRole binding, and
reads only the namespace's public `kube-root-ca.crt` ConfigMap.

## Install

Create the tunnel entry in **Settings → Clusters**, then download and apply the
manifest immediately:

```bash
kubectl apply -f kite-cluster-agent.yaml
```

The manifest URL contains an encrypted grant that expires after ten minutes.
The durable Agent enrollment token is delivered inside the downloaded Secret,
stored only as a hash by Kite, and is used solely to authenticate the reverse
tunnel. It is not a Kubernetes credential.

The generated command is equivalent to:

```text
kite cluster-agent \
  --server=https://kite.example.com \
  --token=<tunnel-enrollment-token> \
  --public-key=<registration-encryption-public-key> \
  --api-server=https://kubernetes.default.svc \
  --ca-file=/var/run/kite/ca/ca.crt
```

`--api-server` is required and must be HTTPS. `--tls-server-name` is
available for endpoints whose network address differs from the certificate
name. There is no kubeconfig or automatic in-cluster credential mode.

## Permissions

Create normal Kubernetes RoleBindings/ClusterRoleBindings for the OIDC users or
groups that will operate this cluster. Do not bind the Kite or Agent
ServiceAccount. The API server audit identity must be the signed-in user for
direct and tunnel modes alike.

If the Agent disconnects, Kite reports the entry as disconnected and tunnel
requests fail; Kite does not fall back to a stored credential or another
network path.
