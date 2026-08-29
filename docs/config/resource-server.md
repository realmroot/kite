# External tool access

Kite is a dashboard client, not an Agent Resource Server. Agent access is
provided by a Cluster Inventory access provider such as Kube Cluster Hub.

Kite discovers access providers from standard `ClusterProfile` resources. It
does not call a Hub-private catalog API and does not request Hub-specific OAuth
scopes. Human Kubernetes calls carry the current user's OIDC ID token; the
target kube-apiserver remains the resource authorizer.

Kite contains no Resource Server signing key, DPoP replay store, Agent token
validator, catalog credential, or privileged target-cluster credential.
