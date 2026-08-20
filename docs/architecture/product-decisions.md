# Kite product and architecture decisions

Status: normative for this fork.

This fork is a Kubernetes client. It does not own human identity, Kubernetes
authorization, or an AI agent runtime. A standards-based OAuth Resource Server
for external tools is a later architecture stage, not part of the current
browser-product contract. Implementation, migrations, tests, documentation,
and shipped UI must follow the decisions below.

## Product scope

Kite's long-term direction is a professional multi-cluster Kubernetes resource
dashboard. New features should normally help discover, inspect, operate, or
troubleshoot Kubernetes resources. This direction is a product filter, not an
instruction to delete existing capabilities without an explicit decision.
Uncertain existing features remain supported until they are reviewed and
accepted for removal.

Kubernetes itself is the compatibility contract. Resource support is driven by
API discovery, REST mapping, OpenAPI, and unstructured objects wherever a
specialized UI is not justified. Kite follows supported Kubernetes releases
with an explicit compatibility matrix and tests API removal, feature-gate, and
version-skew boundaries. A Kubernetes release should normally require dependency
and compatibility validation, not a product rewrite or a new hard-coded model.

## Trust and authorization model

1. Human sign-in uses standard OAuth 2.1/OpenID Connect Authorization Code with
   PKCE. Issuer-specific claims or proprietary provider behavior are forbidden
   in the browser authentication path.
2. Kite keeps an opaque browser session and encrypted upstream tokens. It sends
   the signed-in user's ID token to the selected Kubernetes API server. It never
   replaces that identity with a shared kubeconfig, bearer token, client
   certificate, ServiceAccount, or Kubernetes impersonation headers.
3. Kubernetes authenticates the OIDC token and is the sole authority for
   Kubernetes API permissions. A RoleBinding may target either an exact OIDC
   username/subject or a group; Kite does not require group-only authorization
   and does not reproduce Kubernetes RBAC in its database.
4. Kite-owned objects (cluster connection metadata, templates, Helm repository
   metadata, UI preferences, and audit records) are not Kubernetes objects.
   Their management policy is deliberately separate from Kubernetes resource
   authorization. Platform-management access is derived from standard OIDC
   claims configured by the operator and may never grant additional Kubernetes
   permissions.
5. Direct clusters store a display name, API server URL, optional CA bundle,
   optional Prometheus URL, connection mode, and presentation metadata only.
   Tunnel clusters additionally store transport registration material. They do
   not store Kubernetes credentials. The tunnel agent only transports bytes;
   the user's token remains the Kubernetes request credential.
6. Machine and Agent access uses a separate standards-based OAuth Resource
   Server surface with RFC 9728 metadata, OpenAPI, DPoP verification, RFC 8693
   actor attribution, and explicit operation scopes. Kite API keys and embedded
   agent loops remain forbidden.

## Explicitly removed now

The following capabilities must have no runtime route, handler, model, table
migration, setting, dependency, UI, translation, test fixture, or product
documentation, except for migration notes that explicitly state their removal:

- Built-in AI chat, LLM providers, prompts, tool execution, approval sessions,
  and every embedded Agent/AI loop.
- Local users, passwords, bootstrap/setup administrator, anonymous login, LDAP,
  WebAuthn/passkeys, and Kite-managed MFA.
- Stored OAuth provider records or provider-management UI. Deployment has one
  standards-compliant OIDC issuer configured outside the product database.
- Kite roles, role assignments, permission rules, Kubernetes-resource filters,
  and all other parallel application RBAC for Kubernetes operations.
- User and service API keys.
- Cluster kubeconfig import that retains credentials, shared bearer tokens,
  client certificates, privileged ServiceAccounts, and Kubernetes
  impersonation.

No other original product capability belongs in this list by inference. Moving
an existing capability here requires an explicit product decision and a
replacement or migration assessment.

## Retained target capabilities

These are product capabilities, not compatibility stubs. Each must have a
coherent API, UI, authorization boundary, error behavior, tests, and current
documentation:

- OIDC login, callback, refresh, logout, session revocation, and current-user
  bootstrap.
- Multi-cluster catalog with create, edit, disable, delete, direct connectivity,
  and credential-free reverse tunnel connectivity.
- Kubernetes discovery, browsing, YAML view/edit/apply, create/delete, scale,
  restart, rollback, custom resources, and raw API gateway.
- Pod logs and Pod exec, including streaming lifecycle and disconnect behavior.
- Kubernetes metrics, optional Prometheus queries, and cross-resource search.
- Manual Helm repository browsing, install, dry-run, upgrade, rollback, and
  release management under the interactive user's Kubernetes identity.
- Resource templates, sidebar/user preferences, and attributable
  resource-operation audit history.

Helm, Metrics (including the optional Prometheus integration), and Search are
explicitly retained. Their implementation may be made thinner or more
Kubernetes-native, but they must not be removed as architectural cleanup.

## External tools

Kite is a standards-based OAuth Resource Server so external tools can operate
Kubernetes resources without an embedded Agent loop. The approved contract is:

- the exact protected Resource is configured per deployment and ends at the
  versioned `/api/agent/v1` boundary;
- `clusters:read`, `kubernetes:read`, and `kubernetes:write` form the operation
  scope vocabulary and never grant authority denied by Kubernetes RBAC;
- the token `sub` is the controlling subject and RFC 8693 `act` is the stable
  Agent actor recorded by Kite;
- the exact Resource exposes RFC 9728 metadata and a linked OpenAPI contract.

The implementation must remain authorization-server-neutral. Provider names,
issuer-specific claims, or proprietary token behavior do not belong in Kite.

## Preserve pending review

Existing capabilities not named in either of the two lists above remain in the
product while they are evaluated. This currently includes browser kubectl,
node-terminal workflows, Helm automation, image-registry tag lookup, analytics,
version checks, and other extensions inherited from upstream. Preservation does
not certify their long-term product direction; each receives an explicit keep,
redesign, replace, or remove decision before destructive work begins. While a
capability is preserved, known security boundaries are still enforced: for
example, registry tag lookup uses an operator-controlled host allowlist and
bounded outbound requests.

## Rewrite rules

- Shared transports may be cached by cluster because they contain no user
  credential. Per-request clients and authorization data must not be cached
  across users. No informer may be created per user or per session.
- Product-specific aggregate endpoints are retained for overview, metrics,
  search, logs, exec, Helm, templates, catalog, and audit. Generic Kubernetes
  CRUD should converge on the raw Kubernetes API gateway rather than grow a
  second resource API indefinitely.
- User persistence is a local profile keyed by OIDC issuer plus subject. It may
  contain presentation preferences and last-login data only; it is not an
  identity authority and has no enabled/disabled, password, role, MFA, or API
  key state.
- Database changes require explicit forward migrations that remove sensitive
  legacy columns and obsolete tables. AutoMigrate alone is not an acceptable
  deletion strategy.
- Security failures are explicit and fail closed. Streaming and Kubernetes API
  errors preserve useful upstream status without leaking tokens or secrets.

## Release acceptance

A release is blocked until all of the following pass:

- Repository scans prove removed feature packages, routes, UI imports, settings,
  dependencies, database objects, translations, tests, and documentation are
  absent. Transport `Cluster Agent`, HTTP `User-Agent`, Kubernetes RBAC resource
  kinds, and contribution-policy references to coding assistants are not
  product AI and are explicitly outside that scan.
- Go unit/integration tests, race-sensitive authentication and transport tests,
  frontend typecheck/lint/tests/build, and database migration tests pass.
- A local end-to-end environment proves OIDC login, direct and tunneled cluster
  selection, a user-specific allow and deny from Kubernetes RBAC, logs, exec,
  and representative retained operations. The API server audit identity must be
  the actual OIDC user, never Kite or a shared ServiceAccount.
- Documentation and deployment examples describe only the shipped architecture
  and do not instruct operators to provide Kite with cluster-admin credentials.

When the external-tools milestone is approved for a release, its acceptance
gate additionally includes Resource Server metadata, OpenAPI/scopes, token
validation, DPoP and replay rejection, actor attribution, and negative
authorization conformance tests.
