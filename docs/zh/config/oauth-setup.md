# OpenID Connect 配置

Kite 使用 OIDC Authorization Code + PKCE，同时支持公开客户端和机密客户端。
每个部署配置一个 issuer，Dashboard 内不提供 Provider CRUD。

## 注册 Client

优先在任意符合标准的 OIDC 提供方创建强制 PKCE 的公开 Client，并注册精确回调地址：

```text
https://kite.example.com/api/auth/callback
```

设置 `HOST=https://kite.example.com`，避免回调与安全 Cookie 行为依赖转发头。

## 配置 Kite

```text
OIDC_ISSUER=https://identity.example.com
OIDC_CLIENT_ID=kite
# 公开 PKCE Client 不设置 OIDC_CLIENT_SECRET。
OIDC_PROVIDER_NAME=Corporate Identity
OIDC_SCOPES=openid profile email groups offline_access
OIDC_USERNAME_CLAIM=email
OIDC_GROUPS_CLAIM=groups
OIDC_NAME_CLAIM=name
OIDC_PICTURE_CLAIM=picture
PLATFORM_ADMIN_GROUPS=kite-platform-admins
HOST=https://kite.example.com
KITE_ENCRYPT_KEY=<独立随机密钥>
```

Issuer 必须提供标准 Discovery Metadata，`OIDC_SCOPES` 必须包含 `openid` 和
`offline_access`；Helm 定时任务需要用户的 Refresh Grant。`HOST` 是必填的
HTTPS Origin，不会从转发请求头推断。Claim 名称都是普通的 ID Token 顶层
Claim；Kite 不包含身份提供方定制逻辑。

仅在注册为机密客户端时设置 `OIDC_CLIENT_SECRET`。两种模式都会校验 PKCE、
state 与 nonce。

`PLATFORM_ADMIN_GROUPS` 只控制 Kite 自有共享数据。登录不要求属于这些 group，
这些 group 也不会授予 Kubernetes 权限。

## 对齐 Kubernetes

让每个 API Server 信任相同 issuer 与 audience，并使 username/groups claim
配置和身份提供方一致。再通过 Kubernetes RoleBinding 或 ClusterRoleBinding
绑定精确用户或 group。

Kite 转发原始 ID token。如果托管 Kubernetes 不接受此外部 issuer 或 audience，
Kite 无法在应用层绕过该限制。

## 排障

- Discovery 失败：检查 issuer 的 `/.well-known/openid-configuration`。
- Callback 被拒绝：检查精确 HTTPS callback 与 `HOST`。
- 登录成功但 Kubernetes 返回 401：对齐 issuer、audience、签名密钥和 API
  Server OIDC 参数。
- Kubernetes 返回 403：身份已认证成功，但缺少所需原生 RBAC binding。
