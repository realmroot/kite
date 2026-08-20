# 托管 Kubernetes 认证

Kite 不导入 `kubectl` 使用的云厂商 CLI kubeconfig。只有当托管集群 API Server
能够认证 Kite 使用的同一外部 OIDC issuer 与 audience 时，该集群才兼容。

## 兼容性检查

确认托管服务允许：

1. 信任外部 OIDC issuer 及其签名密钥；
2. 接受 Kite OIDC Client ID 作为 token audience；
3. 配置或明确 username/groups claim；
4. 为这些身份创建原生 RoleBinding/ClusterRoleBinding；以及
5. 提供 Kite 可直接访问或通过 transport-only 隧道访问的 HTTPS API 地址。

添加集群时只填写 API URL、CA Bundle、可选 TLS Server Name 和连接模式。

基于短时 `exec` 插件的云 IAM Authenticator 与直接 OIDC 不是同一协议。Kite
不会执行云 CLI、保存其 token、创建高权限 ServiceAccount，或通过用户模拟弥合
差异。

如果服务无法信任此外部 issuer，它目前不兼容用户 token 直传。未来任何厂商
桥接都需要独立评审的 Workload Identity 与 Token Exchange 设计；不要通过给
Kite `cluster-admin` 解决。

Kubernetes 返回 401 通常表示 issuer/audience/签名/claim 认证未对齐；返回 403
表示认证成功，但该精确用户或 group 缺少合适的 Kubernetes RBAC Binding。
