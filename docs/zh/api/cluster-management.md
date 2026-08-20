# 集群目录 API

集群目录只保存传输与展示元数据。`/api/v1/admin/clusters/` 下的接口要求有效
OIDC 会话，且用户 group 命中 `PLATFORM_ADMIN_GROUPS`。该平台权限不会授予
Kubernetes 资源权限。

## 保存字段

| 字段 | 说明 |
| --- | --- |
| `name` | 唯一展示名称 |
| `description` | 可选说明 |
| `connectionMode` | `direct` 或 `tunnel` |
| `apiServerUrl` | direct 模式的 Kubernetes HTTPS 地址 |
| `caBundle` | 可选 PEM 或 base64 PEM 信任链 |
| `tlsServerName` | 可选 TLS 名称覆盖 |
| `prometheusURL` | 可选 Prometheus Service URL |
| `isDefault` | 默认选择 |
| `enabled` | 用户是否可选择 |

接口不接受 kubeconfig、token、用户名/密码、客户端证书/私钥或 ServiceAccount
字段；未知 JSON 字段会直接拒绝。

## 查询

```http
GET /api/v1/admin/clusters/
```

响应包含上述字段、`id`，以及隧道连接状态/Agent 版本（适用时）。响应绝不
返回注册 Secret 或 Kubernetes 凭据。

普通已登录用户通过以下接口查询可选择的集群名：

```http
GET /api/v1/clusters
```

## 创建

```http
POST /api/v1/admin/clusters/
Content-Type: application/json
```

Direct 示例：

```json
{
  "name": "production",
  "description": "生产集群",
  "connectionMode": "direct",
  "apiServerUrl": "https://api.production.example:6443",
  "caBundle": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
  "tlsServerName": "api.production.example",
  "prometheusURL": "http://prometheus.monitoring.svc.cluster.local:9090",
  "isDefault": true
}
```

`apiServerUrl` 必须使用 HTTPS，并且 Kite 必须可以连接该地址。

Tunnel 示例：

```json
{
  "name": "private",
  "description": "私网集群",
  "connectionMode": "tunnel",
  "isDefault": false
}
```

Tunnel 响应包含短时有效且加密的 `clusterAgentManifestURL`，应及时下载并应用。
URL 授权会过期，后续列表接口不会再次返回。生成的 Agent 只负责传输，不挂载
ServiceAccount token，也没有 Kubernetes RBAC 授权。

## 更新与删除

```http
PUT    /api/v1/admin/clusters/:id
DELETE /api/v1/admin/clusters/:id
```

更新支持 `name`、`description`、`apiServerUrl`、`caBundle`、
`tlsServerName`、`prometheusURL`、`isDefault`、`enabled`。连接模式
不可变；direct/tunnel 切换应替换目录记录。默认集群必须先被其他记录替代后
才能删除。

重命名集群时，对应的 Helm 定时任务会在同一事务中重新绑定。停用集群会同时
停用这些任务，避免持续对不可用目标执行；重新启用集群后，运维需要逐项重新启用
任务。删除非默认集群会永久删除其定时任务，并允许复用该目录名称；历史审计元数据
仍保留操作发生时记录的集群名称。

如果目录由 `KITE_CONFIG_FILE` 管理，创建、更新和删除会返回 403，必须在
配置文件中修改。
