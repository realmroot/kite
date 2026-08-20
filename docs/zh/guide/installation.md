# 安装指南

本指南详细介绍如何在 Kubernetes 环境下安装 Kite。

## 前提条件

- 拥有集群管理员权限的 `kubectl`
- Helm v4，或支持 OCI chart 的 Helm v3.8+
- MySQL/PostgreSQL 数据库，或本地存储用于 sqlite

## 安装方式

### 方式一：Helm Chart（推荐）

使用 fork 发布的版本化 OCI Chart 安装（替换 `<owner>` 与 `<version>`）：

```bash
helm install kite oci://ghcr.io/<owner>/charts/kite \
  --version <version> -n kite-system --create-namespace -f values.yaml
```

如果仓库启用了可选的 GitHub Pages Helm Index，也可以安装相同版本：

```bash
helm repo add kite https://<owner>.github.io/kite/
helm repo update
helm install kite kite/kite --version <version> \
  -n kite-system --create-namespace -f values.yaml
```

#### 自定义安装

可通过自定义 values 文件调整安装参数：

完整配置参考 [Chart Values](../config/chart-values)。

使用自定义值安装：

```bash
helm upgrade --install kite oci://ghcr.io/<owner>/charts/kite \
  --version <version> -n kite-system --create-namespace -f values.yaml

# 或使用 Helm 仓库
helm upgrade --install kite kite/kite --version <version> \
  -n kite-system --create-namespace -f values.yaml
```

### 方式二：YAML 清单

每个 Release 都包含版本化的 `install.yaml`，默认带 SQLite 持久化与非 root
安全上下文。下载后替换 OIDC、Secret 与公网 Host 占位符，审查后再应用：

```bash
curl -fLO https://github.com/<owner>/kite/releases/download/vX.Y.Z/install.yaml
$EDITOR install.yaml
kubectl apply -f install.yaml
```

外部数据库、私有 Issuer CA、Ingress 等高级配置应使用 Helm Chart。

## 访问 Kite

### 端口转发（测试环境）

测试期间可通过端口转发快速访问 Kite：

```bash
kubectl port-forward -n kite-system svc/kite 8080:8080
```

### LoadBalancer 服务

如集群支持 LoadBalancer，可直接暴露 Kite 服务：

```bash
kubectl patch svc kite -n kite-system -p '{"spec": {"type": "LoadBalancer"}}'
```

获取分配的 IP：

```bash
kubectl get svc kite -n kite-system
```

### Ingress（生产环境推荐）

生产环境建议通过 Ingress 控制器并启用 TLS 暴露 Kite：

::: warning
Kite 的日志和 Web 终端功能需支持 websocket。
部分 Ingress 控制器可能需额外配置以正确处理 websocket。
:::

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: kite
  namespace: kite-system
spec:
  ingressClassName: nginx
  rules:
    - host: kite.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: kite
                port:
                  number: 8080
  tls:
    - hosts:
        - kite.example.com
      secretName: kite-tls
```

## 在子路径下部署（basePath）

如果您希望将 Kite 部署在一个子路径下，例如 `https://example.com/kite`，可以使用 Helm Chart 的 `basePath` 值来配置。

如何设置：

- 在 `values.yaml` 中：

```yaml
basePath: "/kite"
```

- 或使用 Helm CLI：

```fish
helm upgrade --install kite oci://ghcr.io/<owner>/charts/kite \
  --version <version> -n kite-system --create-namespace \
  -f values.yaml --set basePath="/kite"
```

说明：

- Ingress 配置：确保 Ingress 的 `paths` 与子路径一致，并使用合适的 `pathType`（例如 `Prefix`）。示例：

```yaml
ingress:
  enabled: true
  hosts:
    - host: kite.example.com
      paths:
        - path: /kite
          pathType: Prefix
```

- OIDC Callback：使用子路径时应注册包含该路径的地址，例如
  `https://kite.example.com/kite/api/auth/callback`。

## 验证安装

安装完成后，打开 Kite 并通过配置的 OIDC Provider 登录。Provider 回调 Kite
后应直接进入 Overview 页面。

::: tip
如需通过环境变量配置 Kite，请参考 [环境变量](../config/env)。
:::

`PLATFORM_ADMIN_GROUPS` 中的管理员可以进入 **设置 > 集群** 添加第一个集群。
直连模式只填写 API Server URL、CA Bundle 和可选 TLS Server Name；私有 API
Server 可以使用无集群凭据的隧道 Agent。Kite 不会把自身 Pod ServiceAccount
当作集群凭据。目标集群必须信任同一个 OIDC Issuer，并通过 Kubernetes RBAC
绑定登录用户或 group。

## 卸载 Kite

### Helm 卸载

```bash
helm uninstall kite -n kite-system
```

### YAML 卸载

```bash
kubectl delete -f install.yaml
```

## 后续步骤

Kite 安装完成后，您可以继续：

- [添加用户](../config/user-management)
- [配置 RBAC](../config/rbac-config)
- [配置 OAuth 认证](../config/oauth-setup)
- [设置 Prometheus 监控](../config/prometheus-setup)
