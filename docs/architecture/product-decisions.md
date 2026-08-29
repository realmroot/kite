# Lightkite product and architecture decisions

Status: normative for this fork.

This fork is a Kubernetes client. It does not own human identity, Kubernetes
authorization, an AI agent runtime, or an OAuth Resource Server. Cluster Access
Gateway owns the shared cluster directory, Agent Resource Server, and
Agent-attributed Kubernetes execution. Implementation, migrations, tests,
documentation, and shipped UI must follow the decisions below.

## Product scope

Lightkite's long-term direction is a professional multi-cluster Kubernetes resource
dashboard. New features should normally help discover, inspect, operate, or
troubleshoot Kubernetes resources. This direction is a product filter, not an
instruction to delete existing capabilities without an explicit decision.
Uncertain existing features remain supported until they are reviewed and
accepted for removal.

Kubernetes itself is the compatibility contract. Resource support is driven by
API discovery, REST mapping, OpenAPI, and unstructured objects wherever a
specialized UI is not justified. Lightkite follows supported Kubernetes releases
with an explicit compatibility matrix and tests API removal, feature-gate, and
version-skew boundaries. A Kubernetes release should normally require dependency
and compatibility validation, not a product rewrite or a new hard-coded model.

## Trust and authorization model

1. Human sign-in uses standard OAuth 2.1/OpenID Connect Authorization Code with
   PKCE. Issuer-specific claims or proprietary provider behavior are forbidden
   in the browser authentication path.
2. Lightkite keeps an opaque browser session and encrypted upstream tokens. It sends
   the signed-in user's ID token through the selected Gateway access URL to the
   Kubernetes API server. It never
   replaces that identity with a shared kubeconfig, bearer token, client
   certificate, ServiceAccount, or Kubernetes impersonation headers.
3. Kubernetes authenticates the OIDC token and is the sole authority for
   Kubernetes API permissions. A RoleBinding may target either an exact OIDC
   username/subject or a group; Lightkite does not require group-only authorization
   and does not reproduce Kubernetes RBAC in its database.
4. Lightkite-owned objects (cluster projection metadata, templates, Helm repository
   metadata, UI preferences, and audit records) are not Kubernetes objects.
   Their management policy is deliberately separate from Kubernetes resource
   authorization. Platform-management access is derived from standard OIDC
   claims configured by the operator and may never grant additional Kubernetes
   permissions.
5. Kube Cluster Hub owns connection metadata. Lightkite keeps
   a credential-free local projection only to preserve stable resource-history
   keys and UI state. The user's token remains the Kubernetes request
   credential.
6. Machine and Agent access uses the Gateway's standards-based OAuth Resource
   Server. Lightkite contains no DPoP verifier, replay store, Agent execution
   credential, API key, or embedded Agent loop.

## Explicitly removed now

The following capabilities must have no runtime route, handler, model, table
migration, setting, dependency, UI, translation, test fixture, or product
documentation, except for migration notes that explicitly state their removal:

- Built-in AI chat, LLM providers, prompts, tool execution, approval sessions,
  and every embedded Agent/AI loop.
- Local users, passwords, bootstrap/setup administrator, anonymous login, LDAP,
  WebAuthn/passkeys, and Lightkite-managed MFA.
- Stored OAuth provider records or provider-management UI. Deployment has one
  standards-compliant OIDC issuer configured outside the product database.
- Lightkite roles, role assignments, permission rules, Kubernetes-resource filters,
  and all other parallel application RBAC for Kubernetes operations.
- User and service API keys.
- Cluster kubeconfig import that retains credentials, shared bearer tokens,
  client certificates, privileged ServiceAccounts, and Kubernetes
  impersonation.
- Lightkite-local OAuth Resource Server routes, DPoP replay state, Agent access-token
  verification, and Agent Kubernetes execution.
- Scheduled Helm auto-upgrade, its scheduler, retained execution credentials,
  configuration UI, and persisted task table.
- Lightkite's private reverse-tunnel protocol, Cluster Agent binary, enrollment
  grants, signing secret, manifests, settings, and connection-mode fields.

No other original product capability belongs in this list by inference. Moving
an existing capability here requires an explicit product decision and a
replacement or migration assessment.

## Retained target capabilities

These are product capabilities, not compatibility stubs. Each must have a
coherent API, UI, authorization boundary, error behavior, tests, and current
documentation:

- OIDC login, callback, refresh, logout, session revocation, and current-user
  bootstrap.
- Multi-cluster catalog with create, edit, disable, delete, and credential-free
  HTTPS API endpoint connectivity. Private routing is deployment infrastructure.
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

External tools discover and call Kube Cluster Hub directly. Hub owns and
displays its Agent audit events. Lightkite neither consumes a private Hub audit API
nor terminates Agent OAuth tokens or proxies Agent traffic. The dashboards and
Agents share standard Cluster Inventory and canonical Kubernetes resource
identity without sharing an authentication credential.

## Preserve pending review

Existing capabilities not named in either of the two lists above remain in the
product while they are evaluated. This currently includes browser kubectl,
node-terminal workflows, image-registry tag lookup, analytics,
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
  absent. HTTP `User-Agent`, Kubernetes RBAC resource kinds, and
  contribution-policy references to coding assistants are not product AI and
  are explicitly outside that scan.
- Go unit/integration tests, race-sensitive authentication and transport tests,
  frontend typecheck/lint/tests/build, and database migration tests pass.
- A local end-to-end environment proves OIDC login, cluster selection, a
  user-specific allow and deny from Kubernetes RBAC, logs, exec,
  and representative retained operations. The API server audit identity must be
  the actual OIDC user, never Lightkite or a shared ServiceAccount.
- Documentation and deployment examples describe only the shipped architecture
  and do not instruct operators to provide Lightkite with cluster-admin credentials.

The system acceptance suite additionally verifies the Gateway's Resource Server
metadata, OpenAPI/scopes, token validation, DPoP replay rejection, actor
attribution, Kubernetes impersonation boundary, and negative authorization
paths.
