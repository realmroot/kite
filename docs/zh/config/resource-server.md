# 外部工具访问

Kite 只作为 Dashboard 客户端，不是 Agent Resource Server。Agent 访问由
Kube Cluster Hub 这类 Cluster Inventory Access Provider 提供。

Kite 从标准 `ClusterProfile` 发现 Access Provider，不调用 Hub 私有目录 API，
也不请求 Hub 专用 OAuth Scope。用户的 Kubernetes 请求只携带当前 OIDC ID Token，
最终权限仍由目标 Kubernetes RBAC 决定。

Kite 不包含 Resource Server Token 校验、DPoP Replay 表、目录凭据或高权限目标集群凭据。
