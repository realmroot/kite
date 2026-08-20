# 全局搜索

Kite 提供 Kubernetes 资源全局搜索，可以按名称或精确标签查找资源。

你可以通过顶部的搜索栏或者在任意界面使用快捷键 `Ctrl + K` (Windows/Linux) 或 `Cmd + K` (macOS) 来激活全局搜索。

![Dashboard Overview](/screenshots/global-search.png)

## 支持功能

### 收藏

点击资源右边的小星星之后，即可以收藏这个资源，下次激活全局搜索时，可以在列表中快速找到。

### 搜索指定资源

您可输入资源名称的前缀加上空格和你想输入的搜索词，例如：

```
pod nginx
```

这样只搜索名称中包含 `nginx` 的 `Pod`。

`pod` 也可以缩写成 `po`

支持的资源类型和缩写来自 Kite 的 Kubernetes 资源注册表，因此会与当前版本
提供的资源页面保持一致。

## 限制

- 不支持模糊搜索
- 不支持跨集群搜索
- 搜索会在请求时向 Kubernetes 发起有结果上限的 List 请求，因此集群规模很大或
  用户权限范围很宽时，延迟可能增加。

每一类搜索 List 请求都使用当前用户的 OIDC Token，因此哪些资源类型和命名空间
能够产生结果由 Kubernetes RBAC 决定。Kite 明确不缓存搜索结果，因此每次搜索
都会重新检查用户当前的 Kubernetes 权限；最终结果最多返回 100 条。
