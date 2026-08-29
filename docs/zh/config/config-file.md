# 配置文件

可选的 YAML 配置文件用于声明式管理“不含凭据”的集群目录。OIDC Client 与平台
策略属于部署环境配置，Kubernetes 授权仍然在 Kubernetes 中完成。

设置 `KITE_CONFIG_FILE=/etc/lightkite/config.yaml`。未知字段会直接报错，因此旧版
kubeconfig、身份提供方或 Lightkite RBAC 配置不会被静默接受。

## Schema

```yaml
clusters:
  - name: production
    description: 生产集群
    apiServerUrl: https://api.production.example:6443
    caBundle: |
      -----BEGIN CERTIFICATE-----
      ...
      -----END CERTIFICATE-----
    tlsServerName: api.production.example
    prometheusURL: http://prometheus.monitoring.svc.cluster.local:9090
    default: true
```

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `name` | 是 | 唯一展示名称 |
| `description` | 否 | 人类可读说明 |
| `apiServerUrl` | 是 | 不含凭据的 Kubernetes HTTPS API 地址 |
| `caBundle` | 否 | PEM 或 base64 编码的 PEM 信任链 |
| `tlsServerName` | 否 | 私网地址的 TLS 名称覆盖 |
| `prometheusURL` | 否 | 优先使用集群内 Service URL |
| `default` | 否 | 是否作为默认集群 |

该文件不得包含 kubeconfig、bearer token、客户端证书/私钥、ServiceAccount
token、OAuth client secret、用户、LDAP 设置或角色映射。

存在 `clusters` 时，该文件是集群目录的权威来源，对应 UI 变为只读。Lightkite 会
监听文件并以事务方式应用有效变更；同名集群原地更新，因此稳定集群身份及资源
历史不会因重启或热更新丢失。从文件移除集群会删除目录记录，
但不会修改该集群中的 Kubernetes 资源。启动时配置无效会阻止 Lightkite 就绪；热更新
无效时继续使用上一份有效目录和连接，修正文件后再重试。

## Helm Chart

使用已有 Secret：

```bash
kubectl create secret generic lightkite-config \
  --from-file=config.yaml=./config.yaml
```

```yaml
config:
  enabled: true
  existingSecret: lightkite-config
```

也可以把相同的 `clusters` 数组写在 Chart values 的 `config` 下。敏感的
OIDC 与数据库配置应放在应用 Secret 中，而不是集群目录文件中。
