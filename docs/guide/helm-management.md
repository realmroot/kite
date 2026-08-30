# Helm Management

Lightkite provides basic Helm management in the dashboard, covering chart discovery, release installation, upgrade, rollback, and uninstall.

## App Catalog

Open **App Catalog** from the sidebar to browse Helm charts.

Lightkite supports two chart sources:

- **Artifact Hub**: search public Helm charts.
- **Repositories**: browse Helm repositories managed in Lightkite.

::: tip
When using the Artifact Hub source, Lightkite may request Artifact Hub to fetch chart lists and chart details.
:::

::: warning
Lightkite only displays chart information and is not responsible for the chart content. Review chart details, templates, and values carefully before installing or upgrading.
:::

OIDC principals configured as platform administrators can add or remove Helm
repositories. Removing a repository only removes it from Lightkite and does not
uninstall existing releases.

Open a chart to view its README, values, templates, and versions. If the chart package is available, you can install it directly from Lightkite.

Install and upgrade requests identify a package by source, repository, chart
name, and version. Lightkite resolves the download URL again from the configured
repository index or Artifact Hub; it never treats a browser-supplied URL as an
outbound fetch target. Catalog, content, and archive caches have global entry
limits and expiration, and oversized chart archives are rejected.

## Helm Releases

Open **Helm Release** from the sidebar to view installed releases.

The release detail page shows release status, chart version, values, resources, history, logs, and rendered manifests.

Lightkite supports dry-run previews before install and upgrade. You can upgrade a release from the detail page, roll back from the history tab, or delete a release to uninstall it from the cluster.

## Permissions

Repository metadata is Lightkite-owned shared data and requires platform-management
access from `PLATFORM_ADMIN_GROUPS`. Helm release operations run against the
selected cluster with the current user's OIDC identity; Kubernetes RBAC on the
release Secrets and managed resources is authoritative.

Kubernetes must grant the current user every operation Helm needs, including
access to Helm release Secrets and the resources rendered by the chart.
