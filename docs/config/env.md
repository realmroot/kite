# Environment Variables

Kite supports several environment variables by default to change the default values of some configuration items.

- **KITE_CONFIG_FILE**: Optional path to a credential-free cluster catalog configuration file.
- **REALMROOT_ISSUER**: Realmroot OIDC issuer. Defaults to `https://id.realmroot.dev/api/auth`.
- **REALMROOT_CLIENT_ID** / **REALMROOT_CLIENT_SECRET**: Required confidential web application credentials.
- **REALMROOT_ADMIN_GROUPS**: Required comma-separated Realmroot groups allowed to manage Kite's shared catalog. This does not grant Kubernetes access.

- **JWT_SECRET**: Secret key used for signing and verifying JWT
- **KITE_ENCRYPT_KEY**: Secret key used to encrypt server-side OIDC tokens.

- **HOST**: Used for generating OAuth 2.0 authorization callback addresses, default will be obtained from request headers. If you find the result not as expected, you can manually configure this environment variable.

- **TRUSTED_PROXIES**: Comma-separated list of reverse proxy, ingress, or load balancer IPs/CIDRs that Kite should trust when reading `X-Forwarded-For` / `X-Real-IP` to determine the client IP. By default, Kite trusts local/private network ranges (`127.0.0.0/8`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `::1`, `fc00::/7`) so common ingress deployments can report real client IPs. Set a narrower value such as `TRUSTED_PROXIES=10.42.0.0/16,192.168.1.10` for production, or `TRUSTED_PROXIES=none` to ignore all client-supplied forwarding headers.

- **CLUSTER_AGENT_IMAGE**: Docker image used when generating the Cluster Agent manifest for Cluster Agent clusters.

- **ENABLE_ANALYTICS**: Enable data analytics functionality, default value is `false`. When enabled, Kite will collect limited data to help improve the product.

- **PORT**: Port on which Kite runs, default value is `8080`.
