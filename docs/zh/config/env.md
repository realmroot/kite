# 环境变量

Kite 默认支持一些环境变量，来改变一些配置项的默认值。

- **KITE_CONFIG_FILE**：可选的无凭据集群目录配置文件路径。
- **OIDC_ISSUER**：必填的 OpenID Connect issuer URL，Kite 使用标准 Discovery 获取端点和 JWKS。
- **OIDC_CLIENT_ID** / **OIDC_CLIENT_SECRET**：必填的机密 Web 应用凭据。
- **OIDC_PROVIDER_NAME**：登录页显示名称，默认为 `OpenID Connect`。
- **OIDC_SCOPES**：空格或逗号分隔的 Scope，必须包含 `openid`。
- **OIDC_USERNAME_CLAIM** / **OIDC_GROUPS_CLAIM**：映射用户名和 Kubernetes Group 的 Claim 名称，默认分别为 `email` 和 `groups`。
- **OIDC_NAME_CLAIM** / **OIDC_PICTURE_CLAIM**：可选的资料 Claim 名称，默认分别为 `name` 和 `picture`。
- **PLATFORM_ADMIN_GROUPS**：可管理 Kite 共享集群目录的 Group；它不会授予 Kubernetes 权限。

- **JWT_SECRET**：用于签名和验证 JWT 的密钥
- **KITE_ENCRYPT_KEY**：用于加密服务端 OIDC Token 的密钥。

- **HOST**: 用户 OAuth 2.0 授权回调地址生成，默认会从请求头获取，如果您发现结果不及预期可以手动配置此环境变量。

- **TRUSTED_PROXIES**：以逗号分隔的反向代理、Ingress 或负载均衡器 IP/CIDR 列表；只有这些直连 Kite 的上一跳才会被信任，Kite 才会读取其转发的 `X-Forwarded-For` / `X-Real-IP` 来判断客户端 IP。默认信任本地和常见私网网段（`127.0.0.0/8`、`10.0.0.0/8`、`172.16.0.0/12`、`192.168.0.0/16`、`::1`、`fc00::/7`），方便常见 Ingress 部署拿到真实用户 IP。生产环境建议配置为更窄的范围，例如 `TRUSTED_PROXIES=10.42.0.0/16,192.168.1.10`；如需忽略所有客户端转发头，可设置 `TRUSTED_PROXIES=none`。

- **CLUSTER_AGENT_IMAGE**: 为 Cluster Agent 类型集群生成 Cluster Agent 部署清单时使用的 Docker 镜像。

- **ENABLE_ANALYTICS**：启用数据分析功能，默认值为 `false`。当启用后，Kite 将收集有限数据以帮助改进产品。

- **PORT**：Kite 运行的端口，默认值为 `8080`。
