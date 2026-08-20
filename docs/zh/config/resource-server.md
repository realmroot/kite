# 外部工具访问

Kite 可以作为标准 OAuth Resource Server 对外提供能力，不会在业务系统内嵌 Agent
运行时。只有配置 `RESOURCE_SERVER_URL` 后才会启用：

```text
RESOURCE_SERVER_URL=https://kite.example.com/api/agent/v1
RESOURCE_SERVER_ISSUER=https://identity.example.com
RESOURCE_SERVER_AUTHORIZED_CLIENT_IDS=agent-protocol-client
RESOURCE_SERVER_JWT_ALGORITHMS=RS256
```

精确的 Resource URL 通过 RFC 8631 `service-desc` 暴露 OpenAPI；RFC 9728
元数据位于 `/.well-known/oauth-protected-resource/api/agent/v1`。契约包含三个 scope：

- `clusters:read`：读取 Kite 的无凭据集群目录；
- `kubernetes:read`：向选定集群发起 GET、HEAD 请求；
- `kubernetes:write`：向选定集群发起 POST、PUT、PATCH、DELETE 请求。

Scope 只是委托权限的外层上限。Kite 会把经过验证、绑定到 Resource audience 的
subject token 转发给 Kubernetes API Server，最终权限仍由 Kubernetes RBAC 决定，
scope 不能覆盖 Kubernetes 的拒绝。

所有受保护请求必须使用 `at+jwt`、`Authorization: DPoP` 和新的 RFC 9449 ES256
证明。Kite 会验证 issuer、精确 audience、有效期、client ID、scope、actor、密钥绑定、
请求方法、目标 URL、token hash 和持久化防重放状态，不提供 Bearer 降级。控制者 subject
与稳定 Agent actor 都会写入 Resource 访问审计。

浏览器交互令牌与 Resource 访问令牌使用不同 audience。当两种令牌来自同一个 issuer
时，Kubernetes 要求在同一个结构化 JWT authenticator 中配置两个 audience，并使用
`MatchAny`（`AuthenticationConfiguration` 内的 issuer URL 必须唯一）：

```yaml
apiVersion: apiserver.config.k8s.io/v1
kind: AuthenticationConfiguration
jwt:
  - issuer:
      url: https://identity.example.com
      audiences:
        - kite-browser-client
        - https://kite.example.com/api/agent/v1
      audienceMatchPolicy: MatchAny
    claimMappings:
      username:
        expression: claims.sub
      groups:
        expression: claims.groups
```

通过 `kube-apiserver --authentication-config` 加载该文件。Kubernetes 1.30–1.33 使用
`apiserver.config.k8s.io/v1beta1`，Kubernetes 1.34 及以上使用 `v1`。不要同时配置
`--authentication-config` 和 `--oidc-*` 参数。最终仍然通过标准 Kubernetes RBAC 将
这些 subject 或 group 绑定到 Role。
