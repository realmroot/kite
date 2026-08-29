# Kubernetes 兼容性

Lightkite 跟随最近三个 Kubernetes 小版本；每次发布前，同一套浏览器与 API
端到端测试必须在三个版本上全部通过。

| Kubernetes API Server | 客户端库 | 发布状态 |
| --- | --- | --- |
| 1.36 | `k8s.io/*` 0.36 | 必须通过的发布门禁 |
| 1.35 | `k8s.io/*` 0.36 | 必须通过的发布门禁 |
| 1.34 | `k8s.io/*` 0.36 | 必须通过的发布门禁 |

CI 使用 kind 0.32.0 发布且固定 digest 的节点镜像，覆盖 OIDC 登录、
Kubernetes RBAC 允许与拒绝、资源发现与通用资源、Metrics、Search、Helm、
日志、Exec、浏览器终端，以及通过部署方网络可达的 HTTPS API Endpoint。发布工作流还会在最新小版本
上重复执行整套测试。

Lightkite 优先使用稳定 Kubernetes API 和 API Discovery。专用实现必须在最老
支持版本移除旧 API 前完成迁移。例如 Service 到 Pod 的关联发现使用
`discovery.k8s.io/v1` EndpointSlice；只有集群没有 EndpointSlice API 时才
兼容 `v1 Endpoints`。

Kubernetes 发布新小版本时，需要升级 `k8s.io/*` 模块、加入固定 digest 的
新 kind 镜像、移除矩阵中最旧版本、跑完整矩阵，并记录用户可见的兼容变化。
只有矩阵测试通过后，才会把一个版本列为受支持。
