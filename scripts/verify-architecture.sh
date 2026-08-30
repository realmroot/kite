#!/usr/bin/env bash

set -euo pipefail

cd "$(dirname "$0")/.."

./scripts/verify-no-embedded-ai.sh

for path in pkg/helm pkg/helmutil pkg/metrics pkg/prometheus pkg/search ui/src/pages/helmrelease-list-page.tsx ui/src/components/global-search.tsx; do
  if [ ! -e "$path" ]; then
    echo "explicitly retained product capability is missing: $path" >&2
    exit 1
  fi
done

for pattern in \
  'api.GET("/prometheus/resource-usage-history"' \
  'api.GET("/search"' \
  'resources.RegisterRoutes(api)' \
  'registerHelmReleaseRoutes'; do
  if ! rg -F -q "$pattern" routes.go pkg/resources/handler.go; then
    echo "explicitly retained Helm, Metrics, Search, or resource-dashboard route is missing: $pattern" >&2
    exit 1
  fi
done

for path in \
  pkg/scheduler \
  pkg/model/scheduled_task.go \
  pkg/proxy/handler.go \
  pkg/clusteragent \
  pkg/kube/proxy.go \
  pkg/resources/event_handler.go \
  pkg/resources/generic_resource_handler_write.go \
  ui/src/pages/helmrelease-auto-upgrade-dialog.tsx; do
  if [ -e "$path" ]; then
    echo "removed Helm auto-upgrade implementation remains: $path" >&2
    exit 1
  fi
done

if rg -n 'api\.(GET|POST|PUT|PATCH|DELETE)\("/(pods|nodes|namespaces|services|deployments|statefulsets|daemonsets|configmaps|secrets|events)(/|"|:)' \
  --glob '*.go' \
  --glob '!**/*_test.go' \
  .; then
  echo "legacy ordinary Kubernetes resource route remains" >&2
  exit 1
fi

if rg -n -i 'auto[- ]upgrade|HelmReleaseAutoUpgrade|helm_release_auto_upgrade|Automatic Upgrades' \
  --glob '!scripts/verify-architecture.sh' \
  --glob '!docs/architecture/product-decisions.md' \
  .; then
  echo "removed Helm auto-upgrade contract remains" >&2
  exit 1
fi

for path in \
  pkg/ai \
  pkg/mfa \
  pkg/passkey \
  pkg/rbac \
  pkg/auth/ldap.go \
  pkg/auth/ldap_setting_handler.go \
  pkg/auth/oauth_manager.go \
  pkg/auth/oauth_provider.go \
  pkg/auth/oauth_provider_handler.go \
  pkg/model/ldap_setting.go \
  pkg/model/oauth.go \
  pkg/model/passkey.go \
  pkg/model/pending_session.go \
  pkg/model/rbac.go \
  ui/src/components/init-check-route.tsx \
  ui/src/components/account-settings-dialog.tsx \
  ui/src/components/settings/apikey-management.tsx \
  ui/src/components/settings/authentication-management.tsx \
  ui/src/components/settings/oauth-provider-management.tsx \
  ui/src/components/settings/rbac-management.tsx \
  ui/src/components/settings/user-management.tsx \
  ui/src/lib/webauthn.ts \
  docs/screenshots/ai-chat.png \
  docs/screenshots/ai-form.png \
  docs/screenshots/ai-permission.png \
  docs/screenshots/assign-role.png \
  docs/screenshots/assign-role2.png \
  docs/screenshots/assign-role3.png \
  docs/screenshots/oauth.png \
  docs/screenshots/rbac.png \
  docs/screenshots/setup.png \
  docs/screenshots/setup2.png \
  docs/screenshots/user-m.png \
  e2e/fixtures/openldap \
  scripts/generate-kite-kubeconfig.sh; do
  if [ -e "$path" ]; then
    echo "removed architecture path still exists: $path" >&2
    exit 1
  fi
done

if rg -n '(clusterAgent|ClusterAgent|connectionMode|ConnectionMode|JWT_SECRET|JwtSecret|remotedialer)' \
  --glob '!docs/architecture/product-decisions.md' \
  --glob '!scripts/verify-architecture.sh' \
  --glob '!**/.git/**' \
  .; then
  echo "removed private tunnel contract remains" >&2
  exit 1
fi

if rg -n -i \
  '(github\.com/(casbin|go-ldap|pquerna/otp)|duo-labs/webauthn|golang\.org/x/crypto/bcrypt)' \
  go.mod go.sum ui/package.json ui/pnpm-lock.yaml; then
  echo "removed identity or authorization dependency remains" >&2
  exit 1
fi

if rg -n \
  '(cloud\.umami\.is|c3d8a914-abbc-4eed-9699-a9192c4bef9e)' \
  --hidden \
  --glob '!scripts/verify-architecture.sh' \
  --glob '!**/.git/**' \
  .; then
  echo "upstream-owned analytics destination remains" >&2
  exit 1
fi

if rg -n \
  '\b(HashPassword|CheckPasswordHash|LDAPSetting|PasskeyCredential|RoleAssignment|PendingSession|APIKeyLogin|RequirePermission)\b' \
  --hidden \
  --glob '*.go' \
  --glob '!**/*_test.go' \
  .; then
  echo "removed identity or authorization implementation remains" >&2
  exit 1
fi

if rg -n \
  '(k8s\.io/api/(batch|discovery|extensions|flowcontrol|networking|policy|storage)/v1beta|flowcontrol\.apiserver\.k8s\.io/v1beta|extensions/v1beta1)' \
  --glob '*.go' \
  --glob '!**/*_test.go' \
  .; then
  echo "Kubernetes API compatibility branch older than the supported minor window remains" >&2
  exit 1
fi

if rg -n \
  '(oauthProviders|resetPassword|assignRoles|allowedGroups|allRoles|allUsers|searchUsers|selectUser|authUrl|tokenUrl|userInfoUrl)' \
  ui/src \
  --hidden \
  --glob '*.ts' \
  --glob '*.tsx' \
  --glob '*.json'; then
  echo "removed setup, identity, or RBAC frontend contract remains" >&2
  exit 1
fi

if rg -n '(cachedSearchResults|cachedSearchResultsByQuery|cacheSearchResults)' ui/src --glob '*.ts' --glob '*.tsx'; then
  echo "frontend Kubernetes search result cache bypasses current RBAC evaluation" >&2
  exit 1
fi
