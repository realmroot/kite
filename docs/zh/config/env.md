# 环境变量

Lightkite 默认支持一些环境变量，来改变一些配置项的默认值。

- **KITE_CONFIG_FILE**：可选的无凭据集群目录配置文件路径。
- **OIDC_ISSUER**：必填的 OpenID Connect issuer URL，Lightkite 使用标准 Discovery 获取端点和 JWKS。
- **OIDC_CLIENT_ID**：必填的应用 Client ID；支持公开的 Authorization Code + PKCE 客户端。
- **OIDC_CLIENT_SECRET**：可选的机密客户端 Secret；公开 PKCE 客户端不设置。
- **OIDC_PROVIDER_NAME**：登录页显示名称，默认为 `OpenID Connect`。
- **OIDC_SCOPES**：空格或逗号分隔的 Scope，必须包含 `openid` 和 `offline_access`，以便浏览器会话在服务端续期。
- **OIDC_USERNAME_CLAIM** / **OIDC_GROUPS_CLAIM**：映射用户名和 Kubernetes Group 的 Claim 名称，默认分别为 `email` 和 `groups`。
- **OIDC_NAME_CLAIM** / **OIDC_PICTURE_CLAIM**：可选的资料 Claim 名称，默认分别为 `name` 和 `picture`。
- **PLATFORM_ADMIN_GROUPS**：可管理 Lightkite 自有共享元数据的 OIDC Group。继续兼容逗号/空格分隔；Claim 值本身含分隔符时使用 JSON 字符串数组，例如 `["operators,west","platform admins"]`。该配置不会授予 Kubernetes 权限。
- **PLATFORM_ADMIN_SUBJECTS**：具有相同平台权限的精确 OIDC `sub`。支持相同的旧列表或 JSON 数组语法，适用于不返回 Group 的部署。
- **CLUSTER_INVENTORY_ENABLED**：启用标准 Cluster Inventory `ClusterProfile` 发现，默认 `false`；关闭时 Lightkite 继续使用本地无凭据集群配置。
- **CLUSTER_INVENTORY_KUBECONFIG**：可选的外部 Inventory Kubernetes API kubeconfig 文件路径；为空时使用集群内 ServiceAccount。
- **CLUSTER_INVENTORY_NAMESPACE**：监听 `ClusterProfile` 的命名空间，默认 `cluster-inventory`。
- **CLUSTER_INVENTORY_LABEL_SELECTOR**：可选的 Kubernetes Label Selector，用于限制发现范围。

- **KITE_ENCRYPT_KEY**：用于加密服务端 OIDC Token 的密钥。

- **HOST**：必填的公网 HTTPS Origin，用于 OAuth 2.0 Callback 和安全 Cookie。不能包含凭据、路径、Query 或 Fragment；只有本地开发的 Loopback 地址允许 HTTP，路径前缀使用 `KITE_BASE`。

- **TRUSTED_PROXIES**：以逗号分隔的反向代理、Ingress 或负载均衡器 IP/CIDR 列表；只有这些直连 Lightkite 的上一跳才会被信任，Lightkite 才会读取其转发的 `X-Forwarded-For` / `X-Real-IP` 来判断客户端 IP。默认信任本地和常见私网网段（`127.0.0.0/8`、`10.0.0.0/8`、`172.16.0.0/12`、`192.168.0.0/16`、`::1`、`fc00::/7`），方便常见 Ingress 部署拿到真实用户 IP。生产环境建议配置为更窄的范围，例如 `TRUSTED_PROXIES=10.42.0.0/16,192.168.1.10`；如需忽略所有客户端转发头，可设置 `TRUSTED_PROXIES=none`。

- **KUBECTL_TERMINAL_IMAGE** / **NODE_TERMINAL_IMAGE**：可选浏览器终端工作流使用的版本化运行时镜像。
- **RELEASE_API_URL**：可选的 GitHub 兼容最新 Release API；为空时不会发起版本检查请求。
- **KITE_IMAGE_REGISTRY_HOSTS**：以逗号分隔、额外允许查询镜像 Tag 的 Registry
  `host[:port]`。Docker Hub、GHCR、Quay、`registry.k8s.io` 与 GCR 默认启用；
  私有或自建 Registry 必须显式加入。该接口不接受 Registry 凭据。

- **ENABLE_ANALYTICS**：启用可选、由部署方管理的数据统计集成，默认值为 `false`。
- **ANALYTICS_SCRIPT_URL** / **ANALYTICS_WEBSITE_ID**：必须成对配置的 Umami 兼容脚本地址和站点 ID。Lightkite 不内置任何统计数据接收端；除本地 Loopback 开发环境外脚本地址必须使用 HTTPS，关闭统计时不会加载脚本。

- **PORT**：Lightkite 运行的端口，默认值为 `8080`。
- **PPROF_ADDRESS**：可选的 Go 诊断监听地址，例如 `127.0.0.1:6060`。默认关闭，不应暴露到公网。
