# Frequently Asked Questions (FAQ)

## Data Sharing

By default, Kite does not collect any analytics data.

Kite has no built-in analytics account or destination. An operator may configure
their own Umami-compatible `ANALYTICS_SCRIPT_URL` and `ANALYTICS_WEBSITE_ID`,
then enable the integration with `ENABLE_ANALYTICS=true` or the admin settings
page. The browser script is not loaded while the integration is disabled.

## Permission Issues

If you encounter an error message like the following when accessing resources:

```txt
User admin does not have permission to get configmaps in namespace kite in cluster in-cluster
```

This means Kubernetes authenticated the displayed OIDC identity but native RBAC
does not allow it to read `configmaps` in the `kite` namespace.

You need to refer to the [RBAC Configuration Guide](./config/rbac-config) to configure user permissions.

## Managed Kubernetes Cluster Connection Issues

Cloud CLI kubeconfigs and `exec` plugins are not used by Kite. The managed API
server must accept the same external OIDC issuer and audience used for Kite
login; otherwise direct user-token propagation is not compatible as-is. Do not
work around this by creating a shared ServiceAccount token.

See the [Managed Kubernetes authentication guide](./config/managed-k8s-auth).

## Persistence Issues

Kite supports the use of SQLite, MySQL, or PostgreSQL as databases.

You can configure the database connection string using the `DB_DSN` environment variable, and specify the type of database using `DB_TYPE` (default is `sqlite`).

- If SQLite is used, data is stored within the container. This means that if the container is deleted, the data will be lost. To persistently store data, you need to mount a persistent volume at the `/data` path. You can set the environment variable `DB_DSN=/data/db.sqlite`. (Note: `/data` isn’t the default path. You can choose any other path as needed, but make sure the path specified in `DB_DSN` matches the mounted path.)

- If MySQL or PostgreSQL is used, you need to provide the appropriate connection string, such as `DB_DSN=user:password@tcp(host:port)/dbname`.

It’s recommended to install Kite using a Helm Chart. This makes it easier to configure persistent storage and database connections.

## SQLite with hostPath Storage

If you're using SQLite as the database and encountering an "out of memory" error when using `hostPath` for persistent storage:

```txt
panic: failed to connect database: unable to open database file: out of memory (14)
```

This issue is related to the pure Go SQLite driver used by Kite (to avoid CGO dependencies). The driver has limitations when accessing database files on certain storage backends.

**Solution**: Add SQLite connection options to improve compatibility with hostPath storage. In your Helm values, set:

```yaml
db:
  sqlite:
    options: "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
```

These options enable Write-Ahead Logging (WAL) mode and increase the busy timeout, which resolves most hostPath compatibility issues.

**Recommended for Production**: For production deployments requiring persistent storage, use MySQL or PostgreSQL instead of SQLite. These databases are better suited for containerized environments and persistent storage scenarios.

## How to Change Font

By default, Kite provides three fonts: system default, `Maple Mono`, and `JetBrains Mono`.

If you want to use a different font, you need to build the project yourself.

Build kite with make and change the font in `./ui/src/index.css`:

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

## How Can I Contribute to Kite?

We welcome contributions! You can:

- Report bugs and feature requests in this repository's issue tracker
- Submit pull requests
- Improve documentation
- Share feedback and use cases

## Where Can I Get Help?

You can get support through:

- This repository's issue tracker for bug reports and feature requests
- The deployment and architecture guides for operational questions

---

**Didn't find what you're looking for?** Open an issue in the repository that
published your Kite build and include the version endpoint output.
