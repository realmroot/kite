# Kubernetes RBAC 配置

Kubernetes 是 Kubernetes 资源的唯一授权方。Kite 不在自己的数据库里定义角色、
资源过滤、deny 规则或用户角色映射。

为 group 授予命名空间只读权限：

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: application-viewers
  namespace: application
subjects:
  - kind: Group
    name: application-viewers
    apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: view
  apiGroup: rbac.authorization.k8s.io
```

精确用户绑定使用 `kind: User`，名称取 API Server 配置的 username claim；
group 绑定使用 groups claim。两者都是标准 Kubernetes RBAC subject。

子资源需要独立权限，例如 `pods/log`、`pods/exec`、集群内 Prometheus 使用
的 `services/proxy`，以及 Helm Release 底层 Secret 与被管理资源。因此 UI
可以显示某项操作，而 Kubernetes 仍会正确地以 403 拒绝未授权请求。

`PLATFORM_ADMIN_GROUPS` 不是 Kubernetes RBAC。它只控制 Kite 自有共享数据，
不能让被 Kubernetes 拒绝的操作成功。
