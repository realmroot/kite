# 授权

Lightkite 不提供应用层 RBAC 管理 API。Kubernetes 资源权限通过原生 `Role`、
`ClusterRole`、`RoleBinding` 和 `ClusterRoleBinding` 对象在 Kubernetes 中配置。

Binding 可以绑定精确的 OIDC 用户，也可以绑定 OIDC group；取值必须与 API
Server 配置的 username/groups claim 一致。Lightkite 只转发当前用户已签名的 ID
token，不会模拟其他主体。

Lightkite 自有共享数据采用独立且收窄的平台策略：`PLATFORM_ADMIN_GROUPS` 成员可
管理集群目录、共享模板/偏好、Helm Repository 元数据及审计视图。该策略绝不
扩大用户的 Kubernetes 权限。

平台审计视图只返回操作者和操作元数据，不返回复制的 Kubernetes YAML。只有在
Kubernetes 对具体资源的 `get` 权限检查通过后，用户才能从该资源的历史路由读取
YAML 历史；其中 Secret 正文不会被保存。

排查资源权限时，应使用相同 OIDC 身份调用 Kubernetes
`SelfSubjectAccessReview` 或 `kubectl auth can-i`。
