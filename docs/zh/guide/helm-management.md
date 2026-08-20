# Helm 管理

Kite 在 Dashboard 中提供基础 Helm 管理能力，包括 Chart 发现、Release 安装、升级、回滚和卸载。

## App Catalog

从侧边栏打开 **App Catalog** 可以浏览 Helm Charts。

Kite 支持两类 Chart 来源：

- **Artifact Hub**：搜索公开 Helm Charts。
- **Repositories**：浏览在 Kite 中托管的 Helm Repositories。

::: tip
使用 Artifact Hub 来源时，Kite 可能会请求 Artifact Hub 来获取 Chart 列表和详情。
:::

::: warning
Kite 只是展示 Chart 信息，不对其中的内容负责。安装或升级前，请仔细审查 Chart 详情、templates 和 values。
:::

配置为平台管理员的 OIDC 主体可以添加或删除 Helm Repository。删除 Repository
只会从 Kite 移除这个来源，不会卸载已有 Release。

进入 Chart 详情后，可以查看 README、values、templates 和版本。如果 Chart package 可用，可以直接从 Kite 安装。

安装和升级请求使用来源、Repository、Chart 名称与版本标识包。Kite 会从已配置的
Repository Index 或 Artifact Hub 重新解析下载地址，不会把浏览器提交的 URL 当作
后端出站请求目标。Catalog、内容和 Archive 缓存均有全局条目上限与过期时间，过大
的 Chart Archive 会被拒绝。

## Helm Releases

从侧边栏打开 **Helm Release** 可以查看已安装的 Releases。

Release 详情页会展示状态、Chart 版本、values、资源、历史记录、日志和渲染后的 manifests。

安装和升级前支持 dry-run 预览。你可以在详情页升级 Release，在历史记录中回滚，也可以删除 Release 来从集群中卸载。

## 自动升级

自动升级能力予以保留，并与交互式 Helm 操作遵循相同的身份边界。保存自动升级
配置时，任务会显式绑定当前 OIDC 会话。执行时 Kite 刷新该会话，把得到的用户
Token 交给 Kubernetes，由 Kubernetes RBAC 对 Release Secret 和 Chart 渲染资源
进行最终授权。Kite 不会替换为平台凭据或集群 ServiceAccount 凭据。

读取或修改自动升级配置之前，当前 Kubernetes 身份必须能够访问对应的 Helm
Release。保存配置后，后续任务会归属于本次保存操作的用户及会话；如果已保存的
操作者与会话不再匹配，任务会停用，而不会在身份归属不明确的情况下继续执行。

如果身份提供方撤销 Refresh Grant、用户退出该会话，或者提供方不再签发刷新后的
ID Token，任务会记录错误并自动停用。用户重新登录后再次启用即可重新授权。

## 权限

Repository 元数据属于 Kite 自有共享数据，需要 `PLATFORM_ADMIN_GROUPS` 提供的
平台管理权限。Helm Release 操作使用当前用户的 OIDC 身份访问所选集群；针对
Release Secret 和被管理资源的 Kubernetes RBAC 是最终授权依据。

Kubernetes 必须向当前用户授予 Helm 所需的每项操作权限，包括访问 Helm Release
Secret 以及操作 Chart 渲染出的资源。
