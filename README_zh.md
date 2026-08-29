# Lightkite — 基于标准协议的 Kubernetes Dashboard

<div align="center">

<img src="./docs/assets/logo.svg" alt="Lightkite Logo" width="128" height="128">

_让用户身份直达 API Server 的 Kubernetes Dashboard_

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://go.dev)
[![React](https://img.shields.io/badge/React-19+-61DAFB?style=flat&logo=react)](https://reactjs.org)
[![TypeScript](https://img.shields.io/badge/TypeScript-6+-3178C6?style=flat&logo=typescript)](https://www.typescriptlang.org)
[![License](https://img.shields.io/badge/License-Apache-green.svg)](LICENSE)
[**文档**](docs/zh/index.md) | [**架构**](docs/oidc-kubernetes.md) | [**与 Kite 的区别**](docs/zh/architecture/upstream-kite.md)
<br>
[English](./README.md) | **中文**

</div>

Lightkite 是 [Kite](https://github.com/kite-org/kite) 的独立 Fork。它保留 Kite 的 Kubernetes 操作界面，并以标准 OpenID Connect 和
Kubernetes 原生 RBAC 替换本地身份、本地授权以及共享高权限 kubeconfig。
用户通过配置的 OIDC 提供方登录，Lightkite 将该用户验证后的 ID token 发送给所选
Kubernetes API Server；用户或 group 直接绑定 RoleBinding/ClusterRoleBinding。

Lightkite 只作为全新部署安装，不支持在现有 Kite 安装上原地升级。

架构说明参见 [OIDC Kubernetes 架构](docs/oidc-kubernetes.md)和
[与上游 Kite 的详细对比](docs/zh/architecture/upstream-kite.md)。内置 AI/Agent
已经移除；Helm、Metrics/Prometheus、Search 以及 Kubernetes 资源管理继续保留。

<img width="1586" height="1167" alt="image" src="https://github.com/user-attachments/assets/a88a63b7-5b71-444d-8d98-66f147a68ef7" />

## ✨ 功能特性

### 用户界面

- 暗色/亮色/彩色主题，支持自动跟随系统偏好
- 跨所有资源的全局搜索
- 适配桌面、平板和移动端的响应式设计
- 国际化支持（中文和英文）

### 多集群管理

- 在多个 Kubernetes 集群间切换
- 每个集群通过 Kubernetes 授权访问集群内 Prometheus
- 无 Kubernetes 凭据的 HTTPS API Server 连接
- 添加、编辑、切换和删除集群目录记录

### 资源管理

- 全面覆盖：Pods、Deployments、Services、ConfigMaps、Secrets、PVs、PVCs、Nodes 等
- 基于 Monaco 编辑器的实时 YAML 编辑（语法高亮和校验）
- 提供容器、卷、事件和状态等详细视图
- 资源关系展示（例如 Deployment → Pods）
- 支持创建、更新、删除、扩缩容和重启操作
- 支持 CRD（Custom Resource Definitions）
- 基于 Docker 和容器镜像仓库 API 的镜像标签快速选择器
- 支持 Helm Chart 发现、安装、升级、回滚和 Release 管理
- 可自定义侧边栏并添加 CRD 快捷入口
- 通过 Kube Proxy 直接访问 Pod/Service（无需 `kubectl port-forward`）

### 监控与可观测性

- 实时 CPU、内存和网络图表（Prometheus）
- 支持过滤和搜索的实时 Pod 日志
- 面向 Pod 和 Node 的 Web 终端
- 内置 kubectl 控制台

### 安全

- 与提供方无关的 OIDC Authorization Code + PKCE
- OIDC token 加密保存在服务端，不暴露给浏览器 JavaScript
- Kubernetes 原生 RBAC 是资源权限的唯一来源
- 不保存 kubeconfig、bearer token、客户端证书或高权限 ServiceAccount

---

## 🚀 快速开始

有关详细说明，请参阅[安装指南](docs/zh/guide/installation.md)。

### Docker

请先配置 [OIDC 架构文档](docs/oidc-kubernetes.md)列出的 OIDC、公开 Host 与独立
加密密钥。缺少必需配置或继续使用开发默认密钥时，服务会拒绝启动。

### 在 Kubernetes 中部署

#### 使用 Helm (推荐)

1.  **安装 Lightkite 发布的版本化 OCI Chart**

    ```bash
    helm install lightkite oci://ghcr.io/realmroot/charts/lightkite \
      --version <version> -n lightkite-system --create-namespace -f values.yaml
    ```

2.  **或从 Helm 仓库安装**

    ```bash
    helm repo add lightkite https://realmroot.github.io/lightkite/
    helm repo update
    helm install lightkite lightkite/lightkite --version <version> \
      -n lightkite-system --create-namespace -f values.yaml
    ```

#### 使用 kubectl

1.  **应用部署清单**

    ```bash
    kubectl apply -f deploy/install.yaml
    # Release 资产包含带不可变镜像版本的同一清单。
    curl -fLO https://github.com/realmroot/lightkite/releases/download/vX.Y.Z/install.yaml
    $EDITOR install.yaml
    kubectl apply -f install.yaml
    ```

2.  **通过端口转发访问**

    ```bash
    kubectl port-forward -n lightkite-system svc/lightkite 8080:8080
    ```

### 从源码构建

1.  **克隆仓库**

    ```bash
    git clone https://github.com/realmroot/lightkite.git
    cd lightkite
    ```

2.  **构建项目**

    ```bash
    make deps
    make build
    ```

3.  **运行服务**

    ```bash
    make run
    ```

---

## 🔍 问题排查

有关问题排查，请参阅本仓库的[常见问题](docs/zh/faq.md)和配置指南。

## 🤝 贡献

我们欢迎贡献！请参阅我们的[贡献指南](./CONTRIBUTING.md)了解如何参与。

## 🙏 上游项目

Lightkite 基于 Kite，并感谢其维护者和贡献者创建了本项目所依赖的 Dashboard、
资源视图与交互基础。Lightkite 由独立团队维护，与上游项目不存在从属或背书
关系，也不计划把这一架构 Fork 整体合并回 Kite。具有普遍价值的小型修复仍可
根据情况贡献给上游。

## 📄 许可证

本项目采用 Apache License 2.0 许可证 - 详见 [LICENSE](LICENSE) 文件。
