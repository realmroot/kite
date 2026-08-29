# 集群目录 API

集群目录只保存传输与展示元数据。`/api/v1/admin/clusters/` 下的接口要求有效
OIDC 会话，且用户 group 命中 `PLATFORM_ADMIN_GROUPS`。该平台权限不会授予
Kubernetes 资源权限。

## 保存字段

| 字段 | 说明 |
| --- | --- |
| `name` | 唯一展示名称 |
| `description` | 可选说明 |
| `apiServerUrl` | Kite 可连接的 Kubernetes HTTPS 地址 |
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

响应包含上述字段和 `id`，绝不返回 Kubernetes 凭据。

普通已登录用户通过以下接口查询可选择的集群名：

```http
GET /api/v1/clusters
```

## 创建

```http
POST /api/v1/admin/clusters/
Content-Type: application/json
```

```json
{
  "name": "production",
  "description": "生产集群",
  "apiServerUrl": "https://api.production.example:6443",
  "caBundle": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----",
  "tlsServerName": "api.production.example",
  "prometheusURL": "http://prometheus.monitoring.svc.cluster.local:9090",
  "isDefault": true
}
```

`apiServerUrl` 必须使用 HTTPS，并且 Kite 必须可以连接该地址。私网路由、DNS
及隧道属于部署基础设施，不是 Kite 私有协议。

## 更新与删除

```http
PUT    /api/v1/admin/clusters/:id
DELETE /api/v1/admin/clusters/:id
```

更新支持 `name`、`description`、`apiServerUrl`、`caBundle`、
`tlsServerName`、`prometheusURL`、`isDefault`、`enabled`。默认集群必须先被
其他记录替代后才能删除。删除非默认集群允许复用目录名称；历史审计元数据仍保留
操作发生时记录的集群身份。

如果目录由 `KITE_CONFIG_FILE` 管理，创建、更新和删除会返回 403，必须在
配置文件中修改。
