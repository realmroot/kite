---
# https://vitepress.dev/reference/default-theme-home-page
layout: home

hero:
  name: "Lightkite"
  text: "A modern Kubernetes dashboard"
  tagline: "Inspect, operate, and troubleshoot Kubernetes resources across clusters"
  image:
    src: /logo.svg
    alt: Lightkite Logo
  actions:
    - theme: brand
      text: Get Started
      link: /guide/
    - theme: alt
      text: Architecture
      link: /oidc-kubernetes

features:
  - icon: 🖥️
    title: User Interface
    details: Dark/light/color themes, global search, responsive design, and i18n support
  - icon: 🏘
    title: Multi-Cluster Management
    details: Credential-free HTTPS API connections with Kubernetes-authorized Prometheus
  - icon: 🔍
    title: Resource Management
    details: Full resource coverage, live YAML editing, relationship view, CRD support, and kube proxy access
  - icon: 📈
    title: Monitoring & Observability
    details: Real-time metrics, live logs, pod/node web terminal, and built-in kubectl console
  - icon: 🔐
    title: Security
    details: Standard OIDC sessions, direct identity propagation, Kubernetes-native RBAC, and audit history
---
