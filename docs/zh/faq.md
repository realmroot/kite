# 常见问题 (FAQ)

## 数据共享

默认情况下，Lightkite 不会收集任何分析数据。

Lightkite 不内置统计账户或数据接收端。运维人员可以配置自己的 Umami 兼容
`ANALYTICS_SCRIPT_URL` 和 `ANALYTICS_WEBSITE_ID`，再通过
`ENABLE_ANALYTICS=true` 或管理设置页面启用；关闭时浏览器不会加载统计脚本。

## 权限问题

如果在访问资源时，遇到如下错误提示，

```txt
用户 admin 没有权限在集群 in-cluster 的命名空间 lightkite 中执行 获取 configmaps
```

这表示 Kubernetes 已认证显示的 OIDC 身份，但原生 RBAC 不允许它读取 `lightkite`
命名空间中的 `configmaps`。

你需要参考 [RBAC 配置指南](./config/rbac-config) 来配置用户的权限。

## 托管 Kubernetes 集群连接问题

Lightkite 不使用云 CLI kubeconfig 或 `exec` 插件。托管 API Server 必须接受 Lightkite
登录使用的同一外部 OIDC issuer 与 audience，否则目前无法直接传递用户 token。
不要通过创建共享 ServiceAccount token 绕过这一限制。

参见[托管 Kubernetes 认证指南](./config/managed-k8s-auth)。

## 持久化相关

Lightkite 支持使用 SQLite、MySQL 或 PostgreSQL 作为数据库。

你可以通过环境变量 `DB_DSN` 来配置数据库连接字符串，`DB_TYPE` 来指定数据库类型（默认为 `sqlite`）。

- 如果使用 SQLite，默认情况下数据将存储在容器内，这意味着如果容器被删除，数据也会丢失。为了持久化数据，你需要将一个持久化卷挂载到 `/data` 路径，并设置环境变量 `DB_DSN=/data/db.sqlite`。（注意：`/data` 并非默认路径，你可以根据需要选择其他路径，但必须确保 `DB_DSN` 中的路径与挂载路径一致。）
- 如果使用 MySQL 或 PostgreSQL，你需要提供相应的连接字符串，例如 `DB_DSN=user:password@tcp(host:port)/dbname`。

建议通过 Helm Chart 进行安装，这样你可以更方便地配置持久化存储和数据库连接。

## SQLite 使用 hostPath 存储问题

如果您使用 SQLite 作为数据库，并在使用 `hostPath` 进行持久化存储时遇到"out of memory"错误：

```txt
panic: failed to connect database: unable to open database file: out of memory (14)
```

此问题与 Lightkite 使用的纯 Go SQLite 驱动有关（为避免 CGO 依赖）。该驱动在访问某些存储后端上的数据
库文件时存在限制。

**解决方案**：添加 SQLite 连接选项以提高与 hostPath 存储的兼容性。在 Helm values 中设置：

```yaml
db:
  sqlite:
    options: "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
```

这些选项启用预写日志（WAL）模式并增加忙碌超时时间，可以解决大多数 hostPath 兼容性问题。

**生产环境推荐**：对于需要持久化存储的生产环境部署，建议使用 MySQL 或 PostgreSQL 代替 SQLite。这些数据库更适合容器化环境和持久化存储场景。

## 如何更改字体

Lightkite 默认提供三种字体：系统默认、`Maple Mono` 和 `JetBrains Mono`。

如果您想使用其他字体，则需要自己构建项目。

用 make 构建 lightkite，并在 `./ui/src/index.css` 中更改字体

```css
@font-face {
  font-family: "Maple Mono";
  font-style: normal;
  font-display: swap;
  font-weight: 400;
  src:
    url(https://cdn.jsdelivr.net/fontsource/fonts/maple-mono@latest/latin-400-normal.woff2)
      format("woff2"),
    url(https://cdn.jsdelivr.net/fontsource/fonts/maple-mono@latest/latin-400-normal.woff)
      format("woff");
}

body {
  font-family: "Maple Mono", var(--font-sans);
}
```

## 我如何为 Lightkite 做出贡献？

我们欢迎贡献！您可以：

- 在发布当前 Lightkite 构建的仓库 Issue Tracker 中报告错误和功能请求
- 提交拉取请求
- 改进文档
- 分享反馈和使用案例

## 我在哪里可以获得帮助？

您可以通过以下方式获得支持：

- 当前构建仓库的 Issue Tracker，用于提交错误报告和功能请求
- 部署与架构指南，用于排查运维问题

---

**没有找到您要找的内容？** 请在发布当前 Lightkite 构建的仓库中提交 Issue，
并附上版本接口的输出。
