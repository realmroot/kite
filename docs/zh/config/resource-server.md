# 外部工具访问

Kite 只作为 Dashboard 客户端。Agent 访问由 Kube Cluster Hub 提供，不再由
Kite 进程内嵌 OAuth Resource Server。

```text
CLUSTER_GATEWAY_URL=https://clusters.example.com
```

Kite 在 Authorization Code + PKCE 登录时请求目录的 RFC 8707 Resource Indicator。
目录 API 使用 Access Token，Kubernetes API 使用同一服务端会话中的 ID Token；最终权限
仍由 Kubernetes RBAC 决定。Hub 独立提供 RFC 9728、
OpenAPI、DPoP Agent 访问与 Agent 审计。

Kite 不再包含 Resource Server token 校验、DPoP replay 表或高权限 Kubernetes 执行凭据。
