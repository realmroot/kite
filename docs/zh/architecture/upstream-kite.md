# Lightkite 与上游 Kite

Lightkite 是 [Kite](https://github.com/kite-org/kite) 的独立、标准协议导向
Fork，起点是 Kite `v0.15.0`，对应提交
`deb3503f2b9e73d90541ee4212468bd8788ae56c`。

我们感谢 Kite 的维护者和贡献者所创建的 Kubernetes Dashboard、资源页面、
Helm、搜索、监控和终端体验。Lightkite 由独立团队维护，与上游项目不存在
从属或背书关系。由于架构方向有意分化，我们不计划把整个 Fork 合并回 Kite。

## 产品边界

Lightkite 专注于 Kubernetes 集群资源管理。项目跟随 Kubernetes 版本与 API
演进，保留专业 Dashboard 所需的运维能力，不在产品内部另建身份系统、权限
模型、AI Agent Runtime 或私有集群传输协议。

## 架构差异

| 领域 | 上游 Kite | Lightkite |
| --- | --- | --- |
| 用户认证 | 应用自行管理认证与用户 | 与供应商无关的 OIDC Authorization Code + PKCE |
| 资源授权 | 应用权限控制叠加共享集群凭据 | 登录用户的 OIDC 身份直接接受 Kubernetes RBAC 授权 |
| 集群凭据 | kubeconfig 与 ServiceAccount 凭据流程 | 只保存无凭据集群端点，不保存 kubeconfig、Bearer Token、客户端证书或高权限 ServiceAccount |
| 集群目录 | 应用私有集群记录 | 本地无凭据记录与标准 Cluster Inventory API |
| Kubernetes 访问 | 产品 Handler 与 Kubernetes Client 并存 | 通用资源走标准 Kubernetes API Gateway；只在 Kubernetes 没有对应 Dashboard API 时保留窄接口 |
| Agent / AI | 产品扩展 | 已移除；外部 Agent 使用自身身份调用 Kubernetes 或 Resource Server API |
| 自动更新 | 产品内置升级流程 | 已移除；通过标准镜像和 Helm 发布流程部署 |
| 授权审计 | 应用定义授权决策 | Kubernetes 是唯一授权权威；Lightkite 记录可归因的代理操作 |

## 保留的能力

Lightkite 保留与新安全模型相容的 Dashboard 能力，包括资源查看与编辑、多集群
切换、搜索、Helm、Metrics、日志、Exec 与节点终端、CRD、资源关系、自定义导航，
以及 Kubernetes Service/Pod Proxy。所有操作仍受目标集群的 Kubernetes RBAC
约束。

## 命名与安装边界

产品名称、Go Module、可执行文件、容器镜像、Helm Chart 和 Kubernetes
部署资源使用 `Lightkite` 或 `lightkite`。

Lightkite 只作为全新部署安装，不支持把现有 Kite 安装原地转换或升级为
Lightkite。部署时应使用新的 Release、Namespace、配置、Secret 和数据库或 PVC。

Lightkite 的独立版本线从 `v0.1.0` 开始；上游 Kite 的版本号只用于说明代码来源，
不是 Lightkite 的升级前置版本。

现有 `KITE_*` 环境变量名称继续作为 Lightkite 的配置接口。这是有意保留的配置
约定，不代表项目承诺提供升级或数据迁移能力。

## 与上游的关系

在可行且相关的范围内，Lightkite 会继续跟进上游 Kite 的前端修复，但独立维护
安全与后端架构。通用的小型修复仍可能贡献给上游；Lightkite 特有的身份与集群
访问设计则留在本项目中。
