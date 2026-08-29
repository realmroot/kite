# 什么是 Lightkite？

Lightkite 是一个轻量级、现代化的 Kubernetes Dashboard，专注于实时可观测性与多集群资源管理。

![Dashboard Overview](/screenshots/overview.png)

## ✨ 功能特性

### 用户界面

- 暗色/亮色/彩色主题，支持自动跟随系统偏好
- 跨所有资源的全局搜索
- 适配桌面、平板和移动端的响应式设计
- 国际化支持（中文和英文）

### 多集群管理

- 在多个 Kubernetes 集群间切换
- 每个集群通过 Kubernetes 授权访问 Prometheus
- 无凭据 HTTPS Kubernetes API 连接
- 添加、编辑、切换和删除集群目录记录

### 资源管理

- 全面覆盖：Pods、Deployments、Services、ConfigMaps、Secrets、PVs、PVCs、Nodes、Helm Releases 等
- 基于 Monaco 编辑器的实时 YAML 编辑（语法高亮和校验）
- 提供容器、卷、事件和状态等详细视图
- 资源关系展示（例如 Deployment -> Pods）
- 支持创建、更新、删除、扩缩容和重启操作
- 支持 CRD（Custom Resource Definitions）
- 基于 Docker 和容器镜像仓库 API 的镜像标签快速选择器
- 可自定义侧边栏并添加 CRD 快捷入口
- 通过 Kube Proxy 直接访问 Pod/Service（无需 `kubectl port-forward`）

### 监控与可观测性

- 实时 CPU、内存和网络图表（Prometheus）
- 支持过滤和搜索的实时 Pod 日志
- 面向 Pod 和 Node 的 Web 终端
- 内置 kubectl 控制台

### 安全

- 与提供方无关的 OIDC Authorization Code + PKCE
- 加密的服务端 Provider 会话
- 所有资源操作使用 Kubernetes 原生 RBAC
- 不保存 kubeconfig、bearer token、客户端证书或高权限 ServiceAccount

## Lightkite 与 Headlamp / Kubernetes Dashboard 的差异

Headlamp 和 Kubernetes Dashboard 都是优秀的资源查看与操作工具。Lightkite 属于
同一类专注的 Dashboard，并提供多集群界面：

- 在同一个工作空间整合可观测性与多集群资源运维
- 将 OIDC 用户身份直接传递给 Kubernetes 原生授权
- 日志、Metrics、Helm、Search、终端与 Kube Proxy 等运维工作流
- 无需第二套资源权限数据库即可记录可归因操作

Lightkite 的产品边界是专业 Kubernetes 资源 Dashboard，而不是通用身份、自动化或
AI 平台。

## 开始使用

准备好探索 Lightkite 了吗？查看[安装指南](./installation)。
