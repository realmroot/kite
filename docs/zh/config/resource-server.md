# 外部工具访问

Kite 只作为 Dashboard 客户端。Agent 访问由 Cluster Access Gateway 提供，不再由
Kite 进程内嵌 OAuth Resource Server。

```text
CLUSTER_GATEWAY_URL=https://clusters.example.com
```

Kite 从 Gateway 读取集群目录，并把当前登录用户的 Kubernetes OIDC ID Token 发送到
Gateway 的访问地址，最终权限仍由 Kubernetes RBAC 决定。Gateway 独立提供 RFC 9728、
OpenAPI、DPoP Agent 访问与 Agent 审计。

Kite 不再包含 Resource Server token 校验、DPoP replay 表或高权限 Kubernetes 执行凭据。
