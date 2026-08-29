import type { Node, Pod } from 'kubernetes-types/core/v1'

import type {
  MetricsData,
  NodeWithMetrics,
  PodMetrics,
  PodWithMetrics,
} from '@/types/api'
import { apiClient } from '@/lib/api-client'
import { getKubernetesResourcePath } from '@/lib/kubernetes-api'

type KubernetesList<T> = {
  items: T[]
  metadata?: { resourceVersion?: string }
}

type NodeMetric = {
  metadata: { name: string }
  usage: { cpu?: string; memory?: string }
}

function cpuMillis(value?: string): number {
  if (!value) return 0
  const parsed = Number.parseFloat(value)
  if (!Number.isFinite(parsed)) return 0
  if (value.endsWith('n')) return parsed / 1_000_000
  if (value.endsWith('u')) return parsed / 1_000
  if (value.endsWith('m')) return parsed
  return parsed * 1_000
}

function bytes(value?: string): number {
  if (!value) return 0
  const parsed = Number.parseFloat(value)
  if (!Number.isFinite(parsed)) return 0
  const suffix = value.match(/[a-zA-Z]+$/)?.[0] || ''
  const multipliers: Record<string, number> = {
    Ki: 1024,
    Mi: 1024 ** 2,
    Gi: 1024 ** 3,
    Ti: 1024 ** 4,
    K: 1000,
    M: 1000 ** 2,
    G: 1000 ** 3,
    T: 1000 ** 4,
  }
  return parsed * (multipliers[suffix] || 1)
}

function podCapacity(pod: Pod): MetricsData {
  const result: MetricsData = {}
  for (const container of pod.spec?.containers || []) {
    result.cpuLimit =
      (result.cpuLimit || 0) + cpuMillis(container.resources?.limits?.cpu)
    result.memoryLimit =
      (result.memoryLimit || 0) + bytes(container.resources?.limits?.memory)
    result.cpuRequest =
      (result.cpuRequest || 0) + cpuMillis(container.resources?.requests?.cpu)
    result.memoryRequest =
      (result.memoryRequest || 0) + bytes(container.resources?.requests?.memory)
  }
  return result
}

function podUsage(
  metric?: PodMetrics
): Pick<MetricsData, 'cpuUsage' | 'memoryUsage'> {
  const result = { cpuUsage: 0, memoryUsage: 0 }
  for (const container of metric?.containers || []) {
    result.cpuUsage += cpuMillis(container.usage.cpu)
    result.memoryUsage += bytes(container.usage.memory)
  }
  return result
}

async function optionalList<T>(
  path: string
): Promise<KubernetesList<T> | undefined> {
  const response = await apiClient.request(path, { method: 'GET' })
  if (!response.ok) return undefined
  return (await response.json()) as KubernetesList<T>
}

export async function enrichResourceList<T>(
  resource: string,
  namespace: string | undefined,
  list: T
): Promise<T> {
  if (resource === 'pods') {
    const metricsPath = await getKubernetesResourcePath('podmetrics', namespace)
    const metrics = await optionalList<PodMetrics>(metricsPath)
    const byPod = new Map(
      (metrics?.items || []).map((item) => [
        `${item.metadata?.namespace || ''}/${item.metadata?.name || ''}`,
        item,
      ])
    )
    const typed = list as KubernetesList<Pod>
    return {
      ...typed,
      items: typed.items.map((pod): PodWithMetrics => ({
        ...pod,
        metrics: {
          ...podCapacity(pod),
          ...podUsage(
            byPod.get(
              `${pod.metadata?.namespace || ''}/${pod.metadata?.name || ''}`
            )
          ),
        },
      })),
    } as T
  }

  if (resource === 'nodes') {
    const [nodeMetrics, pods] = await Promise.all([
      optionalList<NodeMetric>('/kubernetes/apis/metrics.k8s.io/v1beta1/nodes'),
      optionalList<Pod>('/kubernetes/api/v1/pods'),
    ])
    const usageByNode = new Map(
      (nodeMetrics?.items || []).map((item) => [item.metadata.name, item.usage])
    )
    const requestsByNode = new Map<string, MetricsData>()
    for (const pod of pods?.items || []) {
      const nodeName = pod.spec?.nodeName
      if (!nodeName) continue
      const current = requestsByNode.get(nodeName) || { pods: 0 }
      const capacity = podCapacity(pod)
      current.pods = (current.pods || 0) + 1
      current.cpuRequest =
        (current.cpuRequest || 0) + (capacity.cpuRequest || 0)
      current.memoryRequest =
        (current.memoryRequest || 0) + (capacity.memoryRequest || 0)
      requestsByNode.set(nodeName, current)
    }
    const typed = list as KubernetesList<Node>
    return {
      ...typed,
      items: typed.items.map((node): NodeWithMetrics => {
        const usage = usageByNode.get(node.metadata?.name || '')
        return {
          ...node,
          metrics: {
            ...requestsByNode.get(node.metadata?.name || ''),
            cpuUsage: cpuMillis(usage?.cpu),
            memoryUsage: bytes(usage?.memory),
            cpuLimit: cpuMillis(node.status?.allocatable?.cpu),
            memoryLimit: bytes(node.status?.allocatable?.memory),
            podsLimit: Number.parseInt(
              node.status?.allocatable?.pods || '0',
              10
            ),
          },
        }
      }),
    } as T
  }

  return list
}
