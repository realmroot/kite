---
# https://vitepress.dev/reference/default-theme-home-page
layout: home

hero:
  name: "Lightkite"
  text: "现代 Kubernetes 仪表盘"
  tagline: "跨集群查看、操作与排查 Kubernetes 资源"
  image:
    src: /logo.svg
    alt: Lightkite Logo
  actions:
    - theme: brand
      text: 开始使用
      link: /zh/guide/
    - theme: alt
      text: 架构说明
      link: /oidc-kubernetes

features:
  - icon: 🖥️
    title: 用户界面
    details: 暗色/亮色/彩色主题、全局搜索、响应式设计与国际化支持
  - icon: 🏘
    title: 多集群管理
    details: 无凭据 HTTPS API 连接，以及经 Kubernetes 授权的 Prometheus
  - icon: 🔍
    title: 资源管理
    details: 全资源覆盖、实时 YAML 编辑、资源关系展示、CRD 与 Helm 支持、Kube Proxy 直连访问
  - icon: 📈
    title: 监控与可观测性
    details: 实时指标、Pod 日志、Pod/Node Web 终端与内置 kubectl 控制台
  - icon: 🔐
    title: 安全
    details: 标准 OIDC 会话、身份直传、Kubernetes 原生 RBAC 与审计记录
---
