import { defineConfig } from "vitepress";

const analyticsScriptURL = process.env.DOCS_ANALYTICS_SCRIPT_URL?.trim();
const analyticsWebsiteID = process.env.DOCS_ANALYTICS_WEBSITE_ID?.trim();
if (!!analyticsScriptURL !== !!analyticsWebsiteID) {
  throw new Error(
    "DOCS_ANALYTICS_SCRIPT_URL and DOCS_ANALYTICS_WEBSITE_ID must be configured together",
  );
}
if (analyticsScriptURL && new URL(analyticsScriptURL).protocol !== "https:") {
  throw new Error("DOCS_ANALYTICS_SCRIPT_URL must use HTTPS");
}

// https://vitepress.dev/reference/site-config
export default defineConfig({
  title: "Lightkite",
  description: "A modern Kubernetes dashboard",

  sitemap: {
    hostname: "https://realmroot.github.io/lightkite",
    lastmodDateOnly: false,
  },

  markdown: {
    image: {
      lazyLoading: true,
    },
  },

  lastUpdated: true,
  locales: {
    root: {
      label: "English",
      lang: "en",
    },
    zh: {
      label: "中文",
      lang: "zh-CN",
      link: "/zh/",
      title: "Lightkite",
      description: "一个现代 Kubernetes 仪表盘",
      themeConfig: {
        nav: [
          { text: "首页", link: "/zh/" },
          { text: "指南", link: "/zh/guide/" },
          { text: "配置", link: "/zh/config/" },
          { text: "API", link: "/zh/api/authentication" },
          { text: "常见问题", link: "/zh/faq" },
        ],
        editLink: {
          pattern: "https://github.com/realmroot/lightkite/tree/main/docs/:path",
          text: "在 GitHub 上编辑此页面",
        },
      },
    },
  },

  head: [
    ["link", { rel: "icon", href: "/logo.svg" }],
    ...(analyticsScriptURL && analyticsWebsiteID
      ? [
          [
            "script",
            {
              src: analyticsScriptURL,
              "data-website-id": analyticsWebsiteID,
              defer: "true",
            },
          ] as const,
        ]
      : []),
  ],

  themeConfig: {
    // https://vitepress.dev/reference/default-theme-config
    logo: "/logo.svg",
    search: {
      provider: "local",
    },
    langMenuLabel: "Language",
    editLink: {
      pattern: "https://github.com/realmroot/lightkite/tree/main/docs/:path",
      text: "Edit this page on GitHub",
    },

    nav: [
      { text: "Home", link: "/" },
      { text: "Guide", link: "/guide/" },
      { text: "Configuration", link: "/config/" },
      { text: "API", link: "/api/authentication" },
      { text: "FAQ", link: "/faq" },
    ],

    sidebar: {
      "/": [
        {
          text: "Introduction",
          items: [
            { text: "What is Lightkite?", link: "/guide/" },
            { text: "Getting Started", link: "/guide/installation" },
          ],
        },
        {
          text: "Configuration",
          items: [
            { text: "User Management", link: "/config/user-management" },
            { text: "OAuth Setup", link: "/config/oauth-setup" },
            { text: "RBAC Configuration", link: "/config/rbac-config" },
            { text: "Prometheus Setup", link: "/config/prometheus-setup" },
            { text: "Managed K8s Auth", link: "/config/managed-k8s-auth" },
            { text: "Environment Variables", link: "/config/env" },
            { text: "Configuration File", link: "/config/config-file" },
            { text: "Chart Values", link: "/config/chart-values" },
            {
              text: "Kubernetes Compatibility",
              link: "/architecture/kubernetes-compatibility",
            },
            {
              text: "Differences from Kite",
              link: "/architecture/upstream-kite",
            },
          ],
        },
        {
          text: "Usage",
          items: [
            { text: "Global Search", link: "/guide/global-search" },
            { text: "Related Resources", link: "/guide/related-resources" },
            { text: "Logs", link: "/guide/logs" },
            { text: "Monitor", link: "/guide/monitoring" },
            { text: "Helm Management", link: "/guide/helm-management" },
            { text: "Web Terminal", link: "/guide/web-terminal" },
            { text: "Resource History", link: "/guide/resource-history" },
            { text: "Custom Sidebar", link: "/guide/custom-sidebar" },
            { text: "Kube Proxy", link: "/guide/kube-proxy" },
          ],
        },
        {
          text: "FAQ",
          link: "/faq",
        },
      ],
      "/api/": [
        {
          text: "Authentication",
          link: "/api/authentication",
        },
        {
          text: "Resources",
          link: "/api/resources",
        },
        {
          text: "Cluster Management",
          link: "/api/cluster-management",
        },
        {
          text: "Authorization",
          link: "/api/rbac-management",
        },
        {
          text: "User Preferences",
          link: "/api/user-management",
        },
      ],
      "/zh/": [
        {
          text: "介绍",
          items: [
            { text: "什么是 Lightkite?", link: "/zh/guide/" },
            { text: "开始", link: "/zh/guide/installation" },
          ],
        },
        {
          text: "配置",
          items: [
            { text: "用户管理", link: "/zh/config/user-management" },
            { text: "OAuth 设置", link: "/zh/config/oauth-setup" },
            { text: "RBAC 配置", link: "/zh/config/rbac-config" },
            { text: "Prometheus 设置", link: "/zh/config/prometheus-setup" },
            { text: "托管 K8s 认证", link: "/zh/config/managed-k8s-auth" },
            { text: "环境变量", link: "/zh/config/env" },
            { text: "配置文件", link: "/zh/config/config-file" },
            { text: "Chart Values", link: "/zh/config/chart-values" },
            {
              text: "Kubernetes 兼容性",
              link: "/zh/architecture/kubernetes-compatibility",
            },
            {
              text: "与上游 Kite 的区别",
              link: "/zh/architecture/upstream-kite",
            },
          ],
        },
        {
          text: "使用指南",
          items: [
            { text: "全局搜索", link: "/zh/guide/global-search" },
            { text: "相关资源", link: "/zh/guide/related-resources" },
            { text: "日志", link: "/zh/guide/logs" },
            { text: "监控", link: "/zh/guide/monitoring" },
            { text: "Helm 管理", link: "/zh/guide/helm-management" },
            { text: "Web 终端", link: "/zh/guide/web-terminal" },
            { text: "资源历史", link: "/zh/guide/resource-history" },
            { text: "自定义侧边栏", link: "/zh/guide/custom-sidebar" },
            { text: "Kube Proxy", link: "/zh/guide/kube-proxy" },
          ],
        },
        {
          text: "常见问题",
          link: "/zh/faq",
        },
      ],
      "/zh/api/": [
        {
          text: "认证",
          link: "/zh/api/authentication",
        },
        {
          text: "资源操作",
          link: "/zh/api/resources",
        },
        {
          text: "集群管理",
          link: "/zh/api/cluster-management",
        },
        {
          text: "授权边界",
          link: "/zh/api/rbac-management",
        },
        {
          text: "用户偏好",
          link: "/zh/api/user-management",
        },
      ],
    },

    socialLinks: [{ icon: "github", link: "https://github.com/realmroot/lightkite" }],

    footer: {
      message: "Released under the Apache License.",
      copyright: "Copyright © 2025-present Lightkite contributors",
    },
  },
});
