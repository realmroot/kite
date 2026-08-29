# Prometheus Setup Guide

This guide explains how to configure Lightkite's monitoring integration with Prometheus to achieve real-time metrics and monitoring functionality.

## Overview

Lightkite's integration with Prometheus provides:

- Real-time cluster resource metrics
- Historical data visualization
- Pod and container resource usage tracking
- Node performance monitoring

Current CPU and memory samples come from the Kubernetes Metrics API when
metrics-server is installed. Prometheus is optional and adds historical CPU,
memory, network, and disk series. The two integrations are independent.

## Prerequisites

- A running Kubernetes cluster
- `kubectl` configured with cluster access permissions
- Cluster administrator privileges (for Prometheus installation)

## Prometheus Installation Options

### Option 1: Using kube-prometheus-stack (Recommended)

The [kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack) Helm chart provides a complete monitoring solution including Prometheus, Alertmanager, and Grafana.

```bash
# Add Prometheus community Helm repository
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

# Install kube-prometheus-stack
helm install prometheus prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  --create-namespace
```

### Option 2: Manual Prometheus Installation

For more control over the installation, you can manually install Prometheus components:

1. **[Prometheus Server](https://prometheus.io/docs/prometheus/latest/installation/)** - Collects and stores metrics
2. **[kube-state-metrics](https://github.com/kubernetes/kube-state-metrics)** - Provides Kubernetes object metrics
3. **[metrics-server](https://github.com/kubernetes-sigs/metrics-server)** - Provides container resource metrics
4. **Node Exporter** - Collects host system metrics

Follow the official documentation for each component for detailed installation instructions.

## Connecting Lightkite to Prometheus

Open **Settings > Clusters**, edit the cluster, and enter the cluster-local
Prometheus Service URL, for example:

```text
http://prometheus-kube-prometheus-prometheus.monitoring.svc:9090
```

Lightkite accepts only `<service>.<namespace>.svc` and
`<service>.<namespace>.svc.cluster.local` base URLs. It sends each request
through the Kubernetes API Service Proxy with the signed-in user's OIDC token.
Lightkite does not store Prometheus credentials and does not connect anonymously to
an external Prometheus endpoint.

Grant users who need historical metrics `get` access to the Prometheus Service
proxy subresource. Adapt the Service name and namespace to your installation:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: lightkite-prometheus-reader
  namespace: monitoring
rules:
  - apiGroups: [""]
    resources: ["services/proxy"]
    resourceNames: ["prometheus-kube-prometheus-prometheus:9090"]
    verbs: ["get"]
```

Bind this Role to the required OIDC users or groups with a RoleBinding. The
Kubernetes API server remains the authorization authority. Lightkite also performs
a Kubernetes `SelfSubjectAccessReview` before returning Prometheus data:

- Pod history requires `get` on that exact Pod.
- A named node history query requires `get` on that exact Node.
- Cluster-wide history requires `list` on Nodes.

This second check prevents broad Prometheus Service Proxy access from exposing
metrics for Kubernetes resources the user cannot read. The metrics-server
fallback is queried directly with the same user token. Its short in-memory
sample cache is shared by resource only after a successful current-user API
request, is capped globally, and is never duplicated per user.

## Troubleshooting

### Common Issues

1. **No metrics displayed**:

   - Verify Prometheus URL is correct
   - Check Prometheus server is running
   - Ensure Prometheus can scrape metrics from targets

2. **Incomplete metrics**:

   - Ensure kube-state-metrics is running
   - Check Prometheus configuration includes all necessary scrape jobs
   - Verify target pods/nodes are labeled correctly for Prometheus discovery

3. **Authorization errors**:

   - Verify the OIDC user or group can `get` `services/proxy` for the configured
     Service and port.
   - Verify the user can read the target Pod or Node as described above.
   - Verify the cluster's Kubernetes API server accepts the user's OIDC token.
   - Lightkite intentionally has no shared Prometheus credential fallback.

### Verifying Prometheus Configuration

To check if Prometheus is correctly scraping targets:

```bash
# Port-forward to Prometheus UI
kubectl port-forward -n monitoring svc/prometheus-server 9090:9090

# Then open in your browser:
# http://localhost:9090/targets
```
