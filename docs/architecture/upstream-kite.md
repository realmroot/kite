# Lightkite and upstream Kite

Lightkite is an independent, standards-oriented fork of
[Kite](https://github.com/kite-org/kite). The fork started from Kite `v0.15.0`
at commit `deb3503f2b9e73d90541ee4212468bd8788ae56c`.

We thank Kite's maintainers and contributors for the Kubernetes dashboard,
resource views, Helm workflows, search, metrics, and terminal experience on
which Lightkite is built. Lightkite is not affiliated with or endorsed by the
upstream project. Its architecture intentionally diverges, so there is no plan
to merge this fork back into Kite as a whole.

## Product boundary

Lightkite is a focused Kubernetes resource dashboard. It follows Kubernetes
releases and APIs, preserves operational features that belong in a cluster
dashboard, and avoids building a separate identity system, authorization model,
AI agent runtime, or proprietary cluster transport into the product.

## Architectural differences

| Area | Upstream Kite | Lightkite |
| --- | --- | --- |
| User authentication | Application-managed authentication and users | Provider-neutral OpenID Connect Authorization Code with PKCE |
| Resource authorization | Application policy around shared cluster credentials | The signed-in user's OIDC identity is authorized directly by Kubernetes RBAC |
| Cluster credentials | Kubeconfig and ServiceAccount credential workflows | Credential-free cluster endpoints; no stored kubeconfig, bearer token, client certificate, or privileged ServiceAccount |
| Cluster catalog | Application-owned cluster records | Local credential-free records and the standard Cluster Inventory API |
| Kubernetes access | Product handlers plus Kubernetes clients | Canonical Kubernetes API gateway for general resource access; narrow product handlers only where Kubernetes has no equivalent dashboard API |
| Agent/AI features | Product extension | Removed; external agents use the Kubernetes or Resource Server APIs under their own identity |
| Automatic updates | Product-managed update path | Removed; releases are deployed through normal image and Helm workflows |
| Authorization audit | Application-defined authorization decisions | Kubernetes remains the authorization authority; Lightkite records attributable proxy activity |

## Features retained

Lightkite retains the dashboard capabilities that are useful under the new
security model, including resource browsing and editing, multi-cluster
selection, search, Helm, metrics, logs, exec and node terminals, CRDs, resource
relationships, custom navigation, and Kubernetes service and pod proxying.
Every operation remains subject to the selected cluster's Kubernetes RBAC.

## Compatibility identifiers

The product name, Go module, executable, container image, Helm chart, and new
Kubernetes deployment resources use `Lightkite` or `lightkite`.

Existing configuration values and stored data formats do not require a migration
solely due to the rename. In particular, the `KITE_*` environment variables, session
cookie name, browser storage keys, database filename, base-path placeholder,
and historical migration values remain unchanged. They are compatibility
interfaces, not current product branding.

Fresh deployments use `lightkite` Kubernetes resource names. When upgrading an
existing Helm release that used Kite's generated names, set
`fullnameOverride: kite` and keep the existing namespace and release name for
the first Lightkite rollout. This keeps the existing Secret, Service, and PVC
names while the executable and image change to Lightkite. Verify the rendered
manifest with `helm template` before applying it. After the rollout, resource
names can be migrated separately during a planned maintenance window.

## Upstream relationship

Lightkite follows upstream Kite for relevant user-interface fixes where doing
so remains practical, while maintaining its own security and backend
architecture. Small, generally useful fixes may be contributed upstream, but
Lightkite-specific identity and cluster-access changes belong in this project.
