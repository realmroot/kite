# What is Kite?

Kite is a lightweight, modern Kubernetes dashboard for real-time observability and multi-cluster resource management.

![Dashboard Overview](/screenshots/overview.png)

## ✨ Features

### User Interface

- Dark/light/color themes with system preference detection
- Global search across all resources
- Responsive design for desktop, tablet, and mobile
- i18n support (English and Chinese)

### Multi-Cluster Management

- Switch between multiple Kubernetes clusters
- Kubernetes-authorized Prometheus integration per cluster
- Credential-free HTTPS Kubernetes API connections
- Add, edit, switch, and remove cluster catalog entries

### Resource Management

- Full coverage: Pods, Deployments, Services, ConfigMaps, Secrets, PVs, PVCs, Nodes, and more
- Live YAML editing with Monaco editor (syntax highlighting and validation)
- Detailed views with containers, volumes, events, and conditions
- Resource relationships (e.g., Deployment -> Pods)
- Create, update, delete, scale, and restart operations
- Custom Resource Definitions (CRDs) support
- Quick image tag selector using Docker and container registry APIs
- Customizable sidebar with CRD shortcuts
- Kube proxy for direct pod/service access (no more `kubectl port-forward`)

### Monitoring & Observability

- Real-time CPU, memory, and network charts (Prometheus)
- Live pod logs with filtering and search
- Web terminal for pods and nodes
- Built-in kubectl console

### Security

- Provider-neutral OpenID Connect Authorization Code + PKCE
- Encrypted server-side provider sessions
- Kubernetes-native RBAC for every resource operation
- No stored kubeconfig, bearer token, client certificate, or privileged ServiceAccount

## Kite vs Headlamp / Kubernetes Dashboard

Headlamp and Kubernetes Dashboard are strong resource inspection and operation
tools. Kite follows the same focused dashboard category with a multi-cluster UI:

- Unified workspace for observability and multi-cluster resource operations
- Direct OIDC identity propagation to Kubernetes-native authorization
- Operational workflows including logs, metrics, Helm, search, terminals, and kube proxy
- Attributable operations without a second resource-permission database

Kite's product boundary is a professional Kubernetes resource dashboard, not a
general-purpose identity, automation, or AI platform.

## Getting Started

Ready to explore Kite? Check out the [installation guide](./installation).
