# 认证 API

Kite 只使用由部署者配置的一个标准 OpenID Connect 提供方。它不再提供密码、
LDAP、Passkey、Kite 托管 MFA、OAuth Provider CRUD 或 API Key 认证接口。

## 浏览器登录流程

`GET /api/auth/login` 创建带 PKCE 的 Authorization Code 事务，并返回认证地址：

```json
{
  "auth_url": "https://identity.example.com/authorize?...",
  "provider": "OpenID Connect"
}
```

浏览器跳转到 `auth_url`。身份提供方回调 `GET /api/auth/callback` 后，Kite 会
验证 state、nonce、issuer、签名、audience 和 token 有效期，然后建立会话。

浏览器只获得一个不透明的 `HttpOnly`、`SameSite=Lax` Cookie。ID token、access
token 和 refresh token 均加密保存在服务端会话中；HTTPS 部署还会设置
`Secure` 属性。

## 会话接口

```text
GET  /api/auth/user
POST /api/auth/refresh
POST /api/auth/logout
GET  /api/v1/bootstrap
```

`GET /api/auth/user` 返回本地展示资料、用户是否可管理 Kite 自有共享数据，
以及最终生效的侧边栏偏好；它不会返回提供方 token。

`POST /api/auth/refresh` 刷新并重新验证上游 OIDC 会话。
`POST /api/auth/logout` 删除服务端会话并使 Cookie 过期。

## 授权边界

Kite 会把当前会话的 OIDC ID token 用于该用户自己的 Kubernetes 请求。
Kubernetes 负责认证 token，并通过原生 RBAC 对用户或 group 授权。此路径中
不存在 Kite Role 或 API Key。

`PLATFORM_ADMIN_GROUPS` 只控制集群目录、全局偏好等 Kite 自有共享数据，绝不
授予 Kubernetes 权限。

部署与 claim 映射要求参见 [OIDC 原生 Kubernetes 架构](../../oidc-kubernetes.md)。
