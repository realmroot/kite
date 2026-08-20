# 当前用户偏好 API

用户生命周期和账户安全由配置的 OIDC 提供方负责。Kite 不创建、停用、删除、
重置用户，也不为用户分配角色。OIDC 登录验证通过后，Kite 只会按
`issuer + sub` 创建或更新本地展示资料。

Kite 仅保存展示信息、最后登录时间、用于展示/平台策略判断的提供方 group，
以及 Dashboard 偏好。

## 侧边栏偏好

```text
POST /api/users/sidebar_preference
```

```json
{
  "sidebar_preference": "<序列化后的偏好>"
}
```

平台管理员可以管理共享默认值：

```text
POST   /api/v1/admin/sidebar_preference/global
DELETE /api/v1/admin/sidebar_preference/global
```

全局更新使用相同的请求体。这些接口只影响 Dashboard 展示，不影响任何
Kubernetes 权限。
