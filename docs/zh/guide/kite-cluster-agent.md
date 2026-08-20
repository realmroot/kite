# 私网集群隧道

Tunnel 模式在不向 Kite 或隧道 Agent 授予 Kubernetes 身份的前提下连接私网 API
Server。Agent 只是传输组件，不是 Controller，也不是授权代理。

## 数据路径

1. 平台管理员以 `connectionMode: tunnel` 创建集群。
2. Kite 返回短时有效且加密的 Manifest URL。
3. 生成的 Deployment 建立经过认证的反向隧道，只注册 API Server URL、CA、
   TLS Server Name 和 Agent 版本。
4. 每个 Dashboard 请求都由 Kite 将当前用户原始 OIDC ID token 通过隧道发送。
5. Kubernetes API Server 认证该用户并执行原生 RBAC。

Agent 不会向 Kite 发送 kubeconfig、bearer token、客户端证书/私钥或
ServiceAccount token。生成的 Pod 设置 `automountServiceAccountToken: false`，
没有 Role/ClusterRole Binding，只读取命名空间公开的 `kube-root-ca.crt`
ConfigMap。

## 安装

在 **设置 → 集群** 中创建 Tunnel 记录，立即下载并应用 Manifest：

```bash
kubectl apply -f kite-cluster-agent.yaml
```

Manifest URL 含十分钟后过期的加密授权。持久 Agent 注册 token 只存在于下载的
Secret 中，Kite 只保存其哈希；它只用于认证反向隧道，不是 Kubernetes 凭据。

生成的命令等价于：

```text
kite cluster-agent \
  --server=https://kite.example.com \
  --token=<隧道注册 token> \
  --public-key=<注册加密公钥> \
  --api-server=https://kubernetes.default.svc \
  --ca-file=/var/run/kite/ca/ca.crt
```

`--api-server` 必填且必须为 HTTPS。网络地址与证书名称不同时可使用
`--tls-server-name`。不存在 kubeconfig 或自动读取 in-cluster 凭据的模式。

## 权限

请为实际操作该集群的 OIDC 用户或 group 创建正常的 Kubernetes
RoleBinding/ClusterRoleBinding，不要绑定 Kite 或 Agent ServiceAccount。无论
Direct 还是 Tunnel 模式，API Server 审计身份都必须是实际登录用户。

Agent 断开时，Kite 会显示目录记录未连接，并让隧道请求失败；不会回退到存储
凭据或其他网络路径。
