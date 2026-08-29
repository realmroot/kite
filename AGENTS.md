# AGENTS.md

Guidance for AI coding agents working in this repository.

## Project nature

Kite is a single-binary Kubernetes web console with a Go backend and a React
frontend. The backend serves API routes, embeds the built frontend from
`static/`, keeps product metadata through GORM, and talks to Kubernetes clusters
through controller-runtime/client-go clients. The frontend is a Vite React app
that renders resource lists, detail pages, settings, terminals, Helm views,
metrics, and global search. Kite must not embed an AI runtime or an OAuth Agent
Resource Server.

Keep changes narrow. Do not add tests, refactors, helpers, feature flags, broad
validation, compatibility shims, comments, or docs unless the current task
requires them. If a change is local to one handler or component, keep it there.

## Build

The root `Makefile` is the main command surface.

```bash
make deps       # pnpm install in ui/ and go mod download
make frontend   # build ui/ into static/
make backend    # build the Go binary
make build      # frontend + backend
make dev        # run Go backend and Vite dev server
make lint       # go vet, golangci-lint, frontend eslint
make format     # go fmt and frontend prettier
make pre-commit # required before committing
```

Backend-only checks can be run directly:

```bash
go test ./...
go test ./pkg/resources
go test ./pkg/cluster
```

Frontend commands live in `ui/`:

```bash
pnpm --dir ui run build
pnpm --dir ui run type-check
pnpm --dir ui run lint
pnpm --dir ui run test
```

End-to-end tests live in `e2e/` and use Playwright plus a local kind cluster:

```bash
make e2e-test
make e2e-run SPEC=specs/cluster-management.spec.ts
```

Run only the checks relevant to the files you changed unless the user asks for a
larger sweep. Before creating a commit, always run `make pre-commit` and use its
result as the commit gate.

## Architecture

Process startup is split across the root Go files:

- `main.go` handles flags, opt-in pprof, HTTP server startup, shutdown,
  and build-version logging.
- `app.go` validates OIDC and secret settings, initializes the DB, templates,
  credential-free catalog input, cluster manager, config watcher, and Gin
  middleware.
- `routes.go` registers public, auth, admin, protected, resource, Helm,
  terminal, metrics, and proxy routes.
- `static.go` embeds `static/`, serves hashed assets with cache middleware, and
  falls back to `static/index.html` for frontend routes.

The backend package layout is feature-oriented:

- `pkg/model` is the GORM layer. `InitDB` auto-migrates all models and supports
  sqlite, mysql, and postgres. Sensitive persisted strings use `SecretString`.
- `pkg/cluster` owns `ClusterManager`, Kubernetes client creation, Prometheus
  discovery, and optional standard Cluster Inventory discovery.
- `pkg/kube` wraps controller-runtime/client-go clients and owns the shared
  runtime scheme.
- `pkg/resources` owns Kubernetes resource APIs. Most resources use
  `GenericResourceHandler`; version-dependent APIs use
  `versionedResourceHandler`; CRDs use `CRHandler`.
- `pkg/common/resource.go` is the backend resource registry for kinds, aliases,
  scope, searchability, and related-resource support.
- `pkg/auth` owns the standard OIDC Authorization Code + PKCE flow, encrypted
  server-side sessions, and request identity. `pkg/users` owns presentation
  preferences only. Kubernetes is the resource authorizer.
- `pkg/helm` and `pkg/helmutil` own chart repositories, chart content, and Helm
  release actions.
- `pkg/terminal`, `pkg/kube`, and `pkg/resources/logs_handler.go` own websocket
  terminals, exec, and log streaming.

## Request flow

Most protected API calls go through:

1. `authHandler.RequireAuth()`
2. `middleware.ClusterMiddleware(cm)`
3. feature-specific handlers, using the per-request client carrying the
   current user's OIDC ID token

The selected Kubernetes API server performs native authentication and RBAC.
Kite does not evaluate a parallel resource permission model.

When Cluster Inventory is enabled, Kite runs one shared informer against the
configured Inventory Kubernetes API and turns each `ClusterProfile` access
provider into a credential-free cluster context. It does not copy those entries
into Kite's database. The session ID Token is attached only to the current
request sent to the selected access provider. Agent Resource Server discovery,
DPoP verification, execution, and Agent audit belong to the access provider,
not Kite.

The current cluster is passed as `x-cluster-name`. The frontend writes it to
localStorage and a cookie in `ui/src/lib/current-cluster.ts`, and the API client
adds the encoded header in `ui/src/lib/api-client.ts`. The backend middleware
also accepts the same value from query params or cookies for websocket and other
non-fetch flows.

## Frontend

Frontend entry points:

- `ui/src/main.tsx` wires top-level providers.
- `ui/src/App.tsx` owns the app shell, cluster gate, search, and terminal
  surfaces.
- `ui/src/routes.tsx` owns routing.
- `ui/src/lib/api/` owns API functions, re-exported by `ui/src/lib/api.ts`.

UI rules:

- Keep UI simple and consistent with existing visual and interaction patterns.
- Reuse existing components before creating new ones.
- Treat `ui/src/components/ui` as shadcn-managed primitives; edit them only when
  the primitive itself must change.
- Put reusable feature-level components under `ui/src/components`.

When adding or renaming a resource surface, keep these in sync:

- backend constants and registry in `pkg/common/resource.go`
- backend handler registration in `pkg/resources/handler.go`
- frontend resource catalog in `ui/src/lib/resource-catalog.ts`
- route/page/component files under `ui/src/pages` and `ui/src/components`
- translation keys in `ui/src/i18n/locales/en.json` and `zh.json`

Frontend formatting is Prettier-driven: no semicolons, single quotes, 2-space
indentation, sorted imports. Let the configured formatter handle import order.

## Configuration and deployment

Runtime settings are loaded in `pkg/common/common.go` from environment variables
such as the `OIDC_*` claim/client settings, `PLATFORM_ADMIN_GROUPS`, `HOST`,
`PORT`, `KITE_ENCRYPT_KEY`, `DB_TYPE`, `DB_DSN`, `KITE_BASE`,
`KITE_CONFIG_FILE`, `CLUSTER_INVENTORY_*`, and CORS settings.

External config files are parsed in `internal/config.go`. Only the
credential-free `clusters` section is accepted and becomes read-only in the UI.
The config watcher reloads it at runtime.

The Helm chart lives under `charts/kite`. Chart templates wire environment,
secrets, sqlite persistence, config file mounts, an unprivileged ServiceAccount, ingress,
gateway, probes, and deployment strategy. If changing runtime env behavior,
check both `pkg/common/common.go` and the chart template/value path that sets
the same variable.

## Do not edit generated outputs

Do not edit these files by hand:

- `static/`: generated by `pnpm --dir ui run build` or `make frontend`
- `kite` and `bin/`: generated binaries
- `ui/node_modules/`, `e2e/node_modules/`, and Vite cache directories

Only update lockfiles (`go.sum`, `ui/pnpm-lock.yaml`, `e2e/pnpm-lock.yaml`) when
dependency changes require it.

## Coding conventions

- Use `go fmt`. Keep handlers direct and feature-local.
- Follow existing Gin response, klog, and request-context patterns.
- For Kubernetes resources, use the existing registry, handlers, and clients;
  preserve cluster scope, namespace scope, and `_all` behavior.
- Do not bypass authentication, cluster selection, or Kubernetes authorization
  paths.
- Store sensitive persisted values with `model.SecretString`; never log secrets.
- For user-visible frontend text, use i18n keys and keep `en.json` and `zh.json`
  in sync.
