# Environment Variables

Kite supports several environment variables by default to change the default values of some configuration items.

- **KITE_CONFIG_FILE**: Optional path to a credential-free cluster catalog configuration file.
- **OIDC_ISSUER**: Required OpenID Connect issuer URL. Kite uses standard provider discovery.
- **OIDC_CLIENT_ID**: Required application client ID. Public Authorization Code + PKCE clients are supported.
- **OIDC_CLIENT_SECRET**: Optional confidential-client secret. Leave unset for a public PKCE client.
- **OIDC_PROVIDER_NAME**: Login-page display name. Defaults to `OpenID Connect`.
- **OIDC_SCOPES**: Space- or comma-separated scopes. Must contain `openid` and `offline_access` so browser sessions can refresh server-side.
- **OIDC_USERNAME_CLAIM** / **OIDC_GROUPS_CLAIM**: Claims mapped to the local display identity and Kubernetes groups. Defaults to `email` and `groups`.
- **OIDC_NAME_CLAIM** / **OIDC_PICTURE_CLAIM**: Optional profile claim names. Defaults to `name` and `picture`.
- **PLATFORM_ADMIN_GROUPS**: Groups allowed to manage Kite-owned shared metadata. Comma/space-separated values remain supported; use a JSON string array such as `["operators,west","platform admins"]` to preserve claim values containing separators. This does not grant Kubernetes access.
- **PLATFORM_ADMIN_SUBJECTS**: Exact OIDC `sub` values with the same platform access. It accepts the same legacy-list or JSON-array syntax and is useful when the issuer does not emit groups.
- **CLUSTER_INVENTORY_ENABLED**: Enable standard Cluster Inventory `ClusterProfile` discovery. Defaults to `false`; Kite remains usable with its local credential-free cluster configuration when disabled.
- **CLUSTER_INVENTORY_KUBECONFIG**: Optional kubeconfig file path for an external Inventory Kubernetes API. When empty, Kite uses its in-cluster ServiceAccount.
- **CLUSTER_INVENTORY_NAMESPACE**: Namespace watched for `ClusterProfile` resources. Defaults to `cluster-inventory`.
- **CLUSTER_INVENTORY_LABEL_SELECTOR**: Optional Kubernetes label selector limiting discovered profiles.

- **KITE_ENCRYPT_KEY**: Secret key used to encrypt server-side OIDC tokens.

- **HOST**: Required public HTTPS origin used for OAuth 2.0 callbacks and secure cookies. It must not contain credentials, a path, query, or fragment. Loopback HTTP is accepted only for local development; use `KITE_BASE` for a path prefix.

- **TRUSTED_PROXIES**: Comma-separated list of reverse proxy, ingress, or load balancer IPs/CIDRs that Kite should trust when reading `X-Forwarded-For` / `X-Real-IP` to determine the client IP. By default, Kite trusts local/private network ranges (`127.0.0.0/8`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `::1`, `fc00::/7`) so common ingress deployments can report real client IPs. Set a narrower value such as `TRUSTED_PROXIES=10.42.0.0/16,192.168.1.10` for production, or `TRUSTED_PROXIES=none` to ignore all client-supplied forwarding headers.

- **KUBECTL_TERMINAL_IMAGE** / **NODE_TERMINAL_IMAGE**: Versioned runtime images for the optional browser terminal workflows.
- **RELEASE_API_URL**: Optional GitHub-compatible latest-release API. No version-check request is made when it is empty.
- **KITE_IMAGE_REGISTRY_HOSTS**: Comma-separated additional image registry
  `host[:port]` values that Kite may contact for the optional image-tag lookup.
  Docker Hub, GHCR, Quay, `registry.k8s.io`, and GCR are enabled by default;
  private or self-hosted registries require an explicit entry. Kite never
  accepts registry credentials through this endpoint.

- **ENABLE_ANALYTICS**: Enable the optional operator-owned analytics integration. Defaults to `false`.
- **ANALYTICS_SCRIPT_URL** / **ANALYTICS_WEBSITE_ID**: Paired, operator-supplied Umami-compatible script URL and website ID. Kite has no built-in analytics destination. The script URL must use HTTPS except on a loopback development address, and neither value is sent anywhere while analytics is disabled.

- **PORT**: Port on which Kite runs, default value is `8080`.
- **PPROF_ADDRESS**: Optional Go diagnostics listener such as `127.0.0.1:6060`. It is disabled by default; do not expose it publicly.
