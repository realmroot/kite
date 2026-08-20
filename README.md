# Kite — OIDC-native Kubernetes Console

<div align="center">

<img src="./docs/assets/logo.svg" alt="Kite Logo" width="128" height="128">

_A Kubernetes dashboard where the user identity reaches the API server_

<a href="https://trendshift.io/repositories/21820" target="_blank"><img src="https://trendshift.io/api/badge/repositories/21820" alt="kite-org%2Fkite | Trendshift" style="width: 250px; height: 55px;" width="250" height="55"/></a>

<a href="https://github.com/kite-org/kite/stargazers"><img src="https://img.shields.io/github/stars/kite-org/kite?color=ffcb47&labelColor=black&style=flat-square&logo=github&label=Stars" /></a>
<a href="https://github.com/kite-org/kite/releases"><img src="https://img.shields.io/github/downloads/kite-org/kite/total?color=369eff&labelColor=black&logo=github&style=flat-square&label=Downloads" /></a>
<a href="https://github.com/kite-org/kite/graphs/contributors"><img src="https://img.shields.io/github/contributors/kite-org/kite?style=flat-square&logo=github&label=Contributors&labelColor=black" /></a>
[![License](https://img.shields.io/badge/License-Apache-green.svg)](LICENSE)
<a href="https://join.slack.com/t/kite-dashboard/shared_invite/zt-3cl9mccs7-eQZ1_t6IoTPHZkxXED1ceg"><img alt="Join Kite" src="https://badgen.net/badge/Slack/Join%20Kite/0abd59?icon=slack" /></a>


[**Live Demo**](https://kite-demo.zzde.me) | [**Documentation**](https://kite.zzde.me)
<br>
**English** | [中文](./README_zh.md)

</div>

This fork keeps Kite's Kubernetes UI and replaces its local identity, local
authorization, and shared privileged kubeconfigs with standard OpenID Connect
and Kubernetes-native RBAC. The browser signs in through the configured OIDC
provider; Kite sends that user's validated ID token to the selected Kubernetes
API server. Configured group claims therefore map directly to Kubernetes
RoleBindings and ClusterRoleBindings.

See the [OIDC Kubernetes architecture](docs/oidc-kubernetes.md). Provider-specific
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
- Direct and private-tunnel connectivity without stored Kubernetes credentials
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

For detailed instructions, please refer to the [documentation](https://kite.zzde.me/guide/installation.html).

### Docker

Configure the required OIDC and secret values described in
[docs/oidc-kubernetes.md](docs/oidc-kubernetes.md). Startup fails
closed if they are absent or use the upstream development defaults.

### Deploy in Kubernetes

#### Using Helm (Recommended)

1. **Install from OCI registry**

   ```bash
   helm install kite oci://ghcr.io/kite-org/charts/kite -n kube-system
   ```

2. **Or install from Helm repository**

   ```bash
   helm repo add kite https://kite-org.github.io/kite/
   helm repo update
   helm install kite kite/kite -n kube-system
   ```

#### Using kubectl

1. **Apply deployment manifests**

   ```bash
   kubectl apply -f deploy/install.yaml
   # or install it online
   # Note: This method may not be suitable for a production environment, as it does not include any configuration related to persistence. You will need to manually mount the persistence volume and set the environment variable DB_DSN=/data/db.sqlite to ensure that data is not lost. Alternatively, an external database can be used.
   # ref: https://kite.zzde.me/faq.html#persistence-issues
   kubectl apply -f https://raw.githubusercontent.com/kite-org/kite/refs/heads/main/deploy/install.yaml
   ```

2. **Access via port-forward**

   ```bash
   kubectl port-forward -n kube-system svc/kite 8080:8080
   ```

### Build from Source

1. **Clone the repository**

   ```bash
   git clone https://github.com/kite-org/kite.git
   cd kite
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

For troubleshooting, please refer to the [documentation](https://kite.zzde.me).

## 💖 Support This Project

If you find Kite helpful, please consider supporting its development! Your donations help maintain and improve this project.

### Donation Methods

<table>
  <tr>
    <td align="center">
      <b>Alipay</b><br>
      <img src="./docs/donate/alipay.jpeg" alt="Alipay QR Code" width="200">
    </td>
    <td align="center">
      <b>WeChat Pay</b><br>
      <img src="./docs/donate/wechat.jpeg" alt="WeChat Pay QR Code" width="200">
    </td>
    <td align="center">
      <b>PayPal</b><br>
      <a href="https://www.paypal.me/zxh326">
        <img src="https://www.paypalobjects.com/webstatic/mktg/logo/pp_cc_mark_111x69.jpg" alt="PayPal" width="150">
      </a>
    </td>
  </tr>
</table>

Thank you for your support! ❤️

## 🤝 Contributing

We welcome contributions! Please see our [contributing guidelines](./CONTRIBUTING.md) for details on how to get involved.

## 📄 License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
