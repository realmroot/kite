# Prometheus 设置指南

本指南介绍如何配置 Lightkite 与 Prometheus 的监控集成，以实现实时指标和监控功能。

## 概述

Lightkite 与 Prometheus 集成提供：

- 实时集群资源指标
- 历史数据可视化
- Pod 和容器资源使用跟踪
- 节点性能监控

安装 metrics-server 后，当前 CPU、内存采样来自 Kubernetes Metrics API。
Prometheus 是可选能力，用于提供 CPU、内存、网络和磁盘的历史序列；二者相互独立。

## 前提条件

- 一个运行中的 Kubernetes 集群
- 配置了集群访问权限的 `kubectl`
- 集群管理员权限（用于安装 Prometheus）

## Prometheus 安装选项

### 选项 1：使用 kube-prometheus-stack（推荐）

[kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack) Helm chart 提供了完整的监控解决方案，包括 Prometheus、Alertmanager 和 Grafana。

```bash
# 添加 Prometheus 社区 Helm 仓库
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

# 安装 kube-prometheus-stack
helm install prometheus prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  --create-namespace
```

### 选项 2：手动安装 Prometheus

如需对安装有更多控制，您可以手动安装 Prometheus 组件：

1. **[Prometheus 服务器](https://prometheus.io/docs/prometheus/latest/installation/)** - 收集并存储指标
2. **[kube-state-metrics](https://github.com/kubernetes/kube-state-metrics)** - 提供 Kubernetes 对象指标
3. **[metrics-server](https://github.com/kubernetes-sigs/metrics-server)** - 提供容器资源指标
4. **Node Exporter** - 收集主机系统指标

按照每个组件的官方文档获取详细的安装说明。

## 连接 Lightkite 到 Prometheus

进入 **设置 > 集群**，编辑目标集群并填写集群内 Prometheus Service 地址，例如：

```text
http://prometheus-kube-prometheus-prometheus.monitoring.svc:9090
```

Lightkite 只接受 `<service>.<namespace>.svc` 或
`<service>.<namespace>.svc.cluster.local` 形式的基础地址。每个请求都会使用
当前登录用户的 OIDC Token，通过 Kubernetes API 的 Service Proxy 转发。Lightkite
不保存 Prometheus 凭据，也不会匿名直连外部 Prometheus。

需要查看历史指标的用户必须有权读取该 Prometheus Service 的 proxy 子资源。
请按实际安装调整 Service 名和 Namespace：

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: lightkite-prometheus-reader
  namespace: monitoring
rules:
  - apiGroups: [""]
    resources: ["services/proxy"]
    resourceNames: ["prometheus-kube-prometheus-prometheus:9090"]
    verbs: ["get"]
```

再通过 RoleBinding 将该 Role 绑定到所需的 OIDC 用户或组。最终授权仍由
Kubernetes API Server 决定。返回 Prometheus 数据前，Lightkite 还会向 Kubernetes
发起 `SelfSubjectAccessReview`：

- Pod 历史指标要求对该 Pod 执行 `get`；
- 指定 Node 的历史指标要求对该 Node 执行 `get`；
- 集群汇总历史指标要求对 Nodes 执行 `list`。

这层校验避免仅拥有较宽泛的 Prometheus Service Proxy 权限就能看到无权读取的
Kubernetes 资源指标。metrics-server 降级路径也始终使用同一个用户 Token 直接
查询。短期内存采样缓存仅在当前用户的 API 请求成功后按资源共享，具有全局硬上限，
不会为每个用户复制一份。

## 故障排除

### 常见问题

1. **未显示指标**：

   - 验证 Prometheus URL 是否正确
   - 检查 Prometheus 服务器是否运行
   - 确保 Prometheus 可以从目标抓取指标

2. **指标不完整**：

   - 确保 kube-state-metrics 正在运行
   - 检查 Prometheus 配置是否包含所有必要的抓取任务
   - 验证目标 Pod/节点是否正确标记以供 Prometheus 发现

3. **授权错误**：

   - 检查 OIDC 用户或组是否有权对配置的 Service 与端口执行
     `get services/proxy`。
   - 检查用户是否有权读取上面所述的目标 Pod 或 Node。
   - 检查集群 API Server 是否接受当前用户的 OIDC Token。
   - Lightkite 明确不提供共享 Prometheus 凭据作为降级路径。

### 验证 Prometheus 配置

要检查 Prometheus 是否正确抓取目标：

```bash
# 端口转发到 Prometheus UI
kubectl port-forward -n monitoring svc/prometheus-server 9090:9090

# 然后在浏览器中打开：
# http://localhost:9090/targets
```
