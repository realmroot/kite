# Realmroot OIDC demo

This demo creates a disposable kind cluster whose API server trusts Realmroot
directly. Kite stores only the API server URL and CA bundle. Every request from
Kite uses the signed-in user's Realmroot ID token, so the bindings in
`rbac.yaml` are the only Kubernetes authorization policy.

The demo Application is a private Realmroot `confidential_web` client with the
callback `http://localhost:8080/api/auth/callback`. The cluster API server uses
the same client ID as its OIDC audience.

```bash
kind create cluster --config hack/demo/kind-realmroot.yaml
kubectl apply -f hack/demo/rbac.yaml
```

Start Kite with `REALMROOT_CLIENT_ID`, `REALMROOT_CLIENT_SECRET`,
`REALMROOT_ADMIN_GROUPS`, `KITE_ENCRYPT_KEY`, and `JWT_SECRET`, then add the
cluster in Settings using the server and CA from:

```bash
kubectl config view --raw --minify -o jsonpath='{.clusters[0].cluster.server}'
kubectl config view --raw --minify -o jsonpath='{.clusters[0].cluster.certificate-authority-data}'
```

`platform-admins` receives `cluster-admin`. `developers` receives `view` only
inside `realmroot-demo`. Change the bindings to match real Realmroot Team names.
