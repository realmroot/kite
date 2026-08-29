# Lightkite — A standards-based Kubernetes dashboard

<div align="center">

<img src="./docs/assets/logo.svg" alt="Lightkite logo" width="128" height="128">

_A lightweight Kubernetes dashboard where the user's identity reaches the API server_

[![License](https://img.shields.io/badge/License-Apache-green.svg)](LICENSE)

[**Documentation**](docs/index.md) | [**Architecture**](docs/oidc-kubernetes.md) | [**Differences from Kite**](docs/architecture/upstream-kite.md)
<br>
**English** | [中文](./README_zh.md)

</div>

Lightkite is an independent fork of [Kite](https://github.com/kite-org/kite).
It keeps Kite's Kubernetes UI and replaces its local identity, local
authorization, and shared privileged kubeconfigs with standard OpenID Connect
and Kubernetes-native RBAC. The browser signs in through the configured OIDC
provider; Lightkite sends that user's validated ID token to the selected Kubernetes
API server. Configured group claims therefore map directly to Kubernetes
RoleBindings and ClusterRoleBindings.

See the [OIDC Kubernetes architecture](docs/oidc-kubernetes.md) and the detailed
[comparison with upstream Kite](docs/architecture/upstream-kite.md). Provider-specific
configuration belongs under `examples/` and never enters the core runtime.
The backend also provides a transparent [Kubernetes API gateway](docs/kubernetes-api-gateway.md)
for moving UI data access onto canonical Kubernetes resource APIs.

<img width="1586" height="1167" alt="image" src="https://github.com/user-attachments/assets/5710204d-5d34-44af-85dc-3b436e205c12" />

## ✨ Features

### User Interface

- Dark/light/color themes with system preference detection
- Global search across all resources
- Responsive design for desktop, tablet, and mobile
- i18n support (English and Chinese)

### Multi-Cluster Management

- Switch between multiple Kubernetes clusters
- Kubernetes-authorized in-cluster Prometheus service proxy per cluster
- Credential-free HTTPS Kubernetes endpoint connectivity
- Add, edit, switch, and remove credential-free cluster catalog entries

### Resource Management

- Full coverage: Pods, Deployments, Services, ConfigMaps, Secrets, PVs, PVCs, Nodes, and more
- Live YAML editing with Monaco editor (syntax highlighting and validation)
- Detailed views with containers, volumes, events, and conditions
- Resource relationships (e.g., Deployment → Pods)
- Create, update, delete, scale, and restart operations
- Custom Resource Definitions (CRDs) support
- Quick image tag selector using Docker and container registry APIs
- Helm chart discovery, install, upgrade, rollback, and release management
- Customizable sidebar with CRD shortcuts
- Kube proxy for direct pod/service access (no more `kubectl port-forward`)

### Monitoring & Observability

- Real-time CPU, memory, and network charts (Prometheus)
- Live pod logs with filtering and search
- Pod logs and workload metrics, subject to Kubernetes RBAC

### Security

- Provider-neutral OIDC Authorization Code + PKCE login
- Server-side encrypted OIDC sessions; no tokens exposed to browser JavaScript
- Configurable group claims mapped directly by the Kubernetes API server
- Kubernetes-native RBAC as the sole resource authorization policy
- No stored kubeconfig, bearer token, client certificate, or privileged ServiceAccount

---

## 🚀 Quick Start

For detailed instructions, see the [installation guide](docs/guide/installation.md).

### Docker

Configure the required OIDC and secret values described in
[docs/oidc-kubernetes.md](docs/oidc-kubernetes.md). Startup fails
closed if they are absent or use the upstream development defaults.

### Deploy in Kubernetes

#### Using Helm (Recommended)

1. **Install the versioned OCI chart published by Lightkite**

   ```bash
   helm install lightkite oci://ghcr.io/realmroot/charts/lightkite \
     --version <version> -n lightkite-system --create-namespace -f values.yaml
   ```

2. **Or install from Helm repository**

   ```bash
   helm repo add lightkite https://realmroot.github.io/lightkite/
   helm repo update
   helm install lightkite lightkite/lightkite --version <version> \
     -n lightkite-system --create-namespace -f values.yaml
   ```

#### Using kubectl

1. **Apply deployment manifests**

   ```bash
   kubectl apply -f deploy/install.yaml
   # Release assets contain the same manifest with an immutable image tag.
   curl -fLO https://github.com/realmroot/lightkite/releases/download/vX.Y.Z/install.yaml
   $EDITOR install.yaml
   kubectl apply -f install.yaml
   ```

2. **Access via port-forward**

   ```bash
   kubectl port-forward -n lightkite-system svc/lightkite 8080:8080
   ```

### Build from Source

1. **Clone the repository**

   ```bash
   git clone https://github.com/realmroot/lightkite.git
   cd lightkite
   ```

2. **Build the project**

   ```bash
   make deps
   make build
   ```

3. **Run the server**

   ```bash
   make run
   ```

---

## 🔍 Troubleshooting

For troubleshooting, see the local [FAQ](docs/faq.md) and configuration guides.

## 🤝 Contributing

We welcome contributions! Please see our [contributing guidelines](./CONTRIBUTING.md) for details on how to get involved.

## 🙏 Upstream Project

Lightkite is based on Kite and remains grateful to its maintainers and
contributors for the dashboard, resource views, and interaction model that made
this fork possible. Lightkite is independently maintained, is not affiliated
with or endorsed by the upstream project, and does not plan to merge this
architecture fork back into Kite as a whole. Focused fixes may still be shared
with upstream when they are generally useful.

## 📄 License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
