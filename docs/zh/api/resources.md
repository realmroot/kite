# Kubernetes 资源

Lightkite 前端直接使用 Kubernetes API，不再为内置资源或自定义资源维护第二套 CRUD 协议。

## 网关

登录后的浏览器可通过以下路径访问 Kubernetes API：

```text
/api/v1/kubernetes/<kubernetes-api-path>
/api/v1/_clusters/<cluster>/kubernetes/<kubernetes-api-path>
```

网关会原样保留 HTTP 方法、查询参数、请求体、响应状态、Kubernetes `Status`
对象、流式响应和 Content-Type。浏览器 Cookie 和凭据不会转发给集群；网关使用当前
用户绑定到目标集群的 OIDC transport。资源授权仍完全由 Kubernetes 决定。

例如：

```text
GET    /api/v1/kubernetes/api/v1/namespaces/default/configmaps
GET    /api/v1/kubernetes/apis/apps/v1/namespaces/default/deployments/example
POST   /api/v1/kubernetes/api/v1/namespaces/default/configmaps
PUT    /api/v1/kubernetes/api/v1/namespaces/default/configmaps/example
PATCH  /api/v1/kubernetes/apis/apps/v1/namespaces/default/deployments/example
DELETE /api/v1/kubernetes/api/v1/namespaces/default/configmaps/example
```

请求应使用 Kubernetes 原生 media type 和数据结构。例如 Merge Patch 使用
`application/merge-patch+json`，删除传播策略和宽限期使用 Kubernetes
`DeleteOptions`。列表直接使用 Kubernetes 的 `limit`、`continue`、
`labelSelector`、`fieldSelector` 和 `watch` 参数。

前端目录维护内置资源的标准 group/version。自定义资源则通过
`apiextensions.k8s.io/v1` 读取 CRD，选择 storage version（没有时选择第一个
served version），并使用 CRD 声明的 plural 和 scope。因此新增 CRD 不需要新增
Lightkite handler。

Pod 和 Node 表格中的指标由前端组合 core API 与标准
`metrics.k8s.io/v1beta1` 得到。Metrics API 未安装或当前用户无权读取时，基础资源
仍可正常显示。

## Lightkite 专用资源操作

只有无法表达为单次 Kubernetes 资源操作的能力才保留 Lightkite API：

- 多文档 YAML apply 和历史回滚编排；
- 资源历史与审计展示；
- 与 `kubectl describe` 兼容的聚合输出；
- 关联资源聚合；
- 工作负载 revision 展示与回滚编排；
- Node drain（组合 cordon 和多个 Pod eviction）；
- 基于 exec 的 Pod 文件浏览与传输；
- Helm release 操作，因为 Helm release 不是 Kubernetes API 资源。

这些接口不再承载普通的 Kubernetes list/get/create/update/patch/delete。

## 资源历史

内置资源和自定义资源详情页通过 `/<resource>/<namespace>/<name>/history`
读取分页历史；集群级资源使用 `_all`。读取 Lightkite 历史数据库之前，后端会使用当前
用户 token 对准确的 group、plural、namespace 和 name 提交 Kubernetes
`SelfSubjectAccessReview`。拒绝时返回 `403`，平台管理权限不能绕过此检查。

通过标准 Kubernetes 网关发出的变更会在这个统一边界记录。分页参数为从 1 开始的
`page` 和 `pageSize`，其中 `pageSize` 最大为 100。Secret 操作只保留归属、成功或
失败等元数据，不保存 Secret YAML 或原始错误详情。历史回滚会产生一次新的 apply，
并再次接受 Kubernetes 授权。

历史记录绑定不可变的集群目录 ID，而不是只绑定显示名称。重命名集群不会丢失历史；
删除后再创建同名集群也不会继承旧集群的 YAML。
