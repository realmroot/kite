# Helm Chart Values

Chart 部署的 Kite 不挂载 ServiceAccount token，也不会为 Dashboard 的资源访问
创建 Kubernetes RBAC 授权。请配置 OIDC，并在每个目标集群分别绑定用户/group。

## 必填身份配置

| Value | 说明 |
| --- | --- |
| `image.repository` | 从源码渲染 Chart 时必须显式设置的镜像仓库 |
| `oidc.issuer` | 标准 OpenID Connect issuer URL |
| `oidc.clientId` | 公开 PKCE 或机密 Client ID |
| `oidc.clientSecret` | 可选的机密 Client Secret；公开 PKCE Client 留空 |
| `platformAdminGroups` | 可管理 Kite 自有共享数据的 group；group 含空格、逗号或其他标点时使用 JSON 字符串数组 |
| `platformAdminSubjects` | 具有相同平台权限的精确 OIDC `sub`；同样支持 JSON 字符串数组 |
| `encryptKey` | 加密服务端提供方 token 的随机密钥 |
| `jwtSecret` | 用于短时隧道注册凭据的独立随机密钥 |

可选映射包括 `oidc.providerName`、`oidc.scopes`、
`oidc.usernameClaim`、`oidc.groupsClaim`、`oidc.nameClaim` 和
`oidc.pictureClaim`，均为标准协议配置。

私有 Issuer CA 可以放入 Kubernetes Secret，并通过
`oidc.ca.existingSecret` 引用。`oidc.ca.key` 默认为 `ca.crt`；Chart 会以
只读方式挂载，并自动设置 `OIDC_CA_FILE`。

生产环境建议使用 `secret.create=false` 和 `secret.existingSecret`。已有
Secret 必须包含 `OIDC_CLIENT_ID`、`KITE_ENCRYPT_KEY`、`JWT_SECRET`，以及需要的
数据库配置。只有机密客户端才需要 `OIDC_CLIENT_SECRET`。

## 运行与暴露

| Value | 默认值 | 说明 |
| --- | --- | --- |
| `replicaCount` | `1` | 副本数 |
| `image.repository` | 源码 Chart 中为空 | 镜像仓库；发布打包时写入发布者仓库 |
| `image.tag` | Chart appVersion | 镜像 Tag |
| `deploymentStrategy.type` | `Recreate` | SQLite 单副本的安全默认值；外部数据库可用 `RollingUpdate` |
| `host` | 必填 | OIDC Callback 使用的公网 HTTPS Origin；不允许包含路径 |
| `basePath` | 空 | 可选 URL 前缀 |
| `service.type` | `ClusterIP` | Service 类型 |
| `service.port` | `8080` | HTTP 端口 |
| `ingress.enabled` | `false` | 创建 Ingress |
| `gateway.enabled` | `false` | 创建 Gateway API 资源 |
| `debug` | `false` | 详细日志 |
| `terminalImages.kubectl` | `alpine/kubectl:1.36.3` | 版本化 Shell + kubectl 终端镜像 |
| `terminalImages.node` | `busybox:1.37.0` | 版本化 Node 终端镜像 |
| `imageRegistryHosts` | 空 | 镜像 Tag 查询可访问的额外 Registry `host[:port]`，以逗号分隔 |
| `clusterAgentImage` | 当前 Kite 镜像 | 可选的独立 Cluster Agent 镜像 |
| `releaseAPIURL` | 空 | 可选 GitHub 兼容更新 API；为空时不发起外部检查 |
| `analytics.enabled` | `false` | 加载部署方配置的统计脚本 |
| `analytics.scriptURL` | 空 | Umami 兼容的 HTTPS 脚本地址；必须与 `analytics.websiteID` 同时配置 |
| `analytics.websiteID` | 空 | 部署方自己的统计站点 ID |

Ingress/Gateway 终止 TLS 时必须保留公网 Host 与协议；生产环境应显式设置
`host`。

## 数据库

| Value | 默认值 | 说明 |
| --- | --- | --- |
| `db.type` | `sqlite` | `sqlite`、`postgres` 或 `mysql` |
| `db.dsn` | 空 | 外部数据库 DSN |
| `db.sqlite.persistence.pvc.enabled` | `true` | 使用 PVC 持久化 SQLite |
| `db.sqlite.persistence.pvc.existingClaim` | 空 | 复用 PVC |
| `db.sqlite.persistence.pvc.storageClass` | 空 | StorageClass |
| `db.sqlite.persistence.pvc.size` | `1Gi` | 容量 |
| `db.sqlite.persistence.mountPath` | `/data` | 挂载路径 |
| `db.sqlite.persistence.filename` | `kite.db` | 文件名 |

生产多副本需要 PostgreSQL 或 MySQL。Chart 会拒绝 SQLite 多副本、SQLite
PVC 滚动升级、PVC 与 hostPath 同时启用、不支持的数据库类型，以及缺少 DSN
的外部数据库配置。
SQLite 使用单个应用连接，并启用外键、Busy Timeout 与 WAL，从而避免 OIDC
会话事务之间的锁竞争。

## 无凭据集群目录

`config.enabled` 挂载声明式集群目录。可以通过 `config.existingSecret`
引用含 `config.yaml` 的 Secret，或内联 `config.clusters`。只接受
[配置文件](./config-file.md)记录的字段；kubeconfig 与身份策略会被拒绝。

## Pod 身份与安全

| Value | 默认值 | 说明 |
| --- | --- | --- |
| `serviceAccount.create` | `true` | 为 Kite Pod 创建身份 |
| `serviceAccount.automount` | `false` | 挂载 Kubernetes API token；应保持关闭 |
| `podSecurityContext` | 非 root UID/GID 65532、RuntimeDefault seccomp | Pod Security Context |
| `securityContext` | 禁止提权、只读根文件系统、删除全部 capabilities | Container Security Context |
| `resources` | `{}` | Requests/Limits |
| `nodeSelector`、`affinity`、`tolerations` | 空 | 调度配置 |
| `extraEnvs` | 空 | 额外环境变量 |
| `volumes`、`volumeMounts` | 空 | 额外挂载 |

包括 Gateway API 与 Probe 结构在内的完整权威默认值位于
`charts/kite/values.yaml`。
