import { useCallback, useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Pod } from 'kubernetes-types/core/v1'

import {
  HelmChartContent,
  HelmChartContentType,
  HelmChartDetail,
  HelmChartList,
  HelmRelease,
  HelmReleaseDryRunResponse,
  HelmReleaseHistoryResponse,
  HelmReleaseInstallRequest,
  HelmReleaseUpgradeRequest,
  HelmRepository,
  ImageTagInfo,
  RelatedResources,
  ResourceHistoryResponse,
  ResourcesTypeMap,
  ResourceTemplate,
  ResourceType,
  ResourceTypeMap,
  WorkloadRevisionResourceType,
  WorkloadRevisionsResponse,
} from '@/types/api'
import { getKubernetesResourcePath } from '@/lib/kubernetes-api'
import { getResourceQueryKey } from '@/lib/resource-metadata'
import { enrichResourceList } from '@/lib/resource-metrics'
import { useCluster } from '@/hooks/use-cluster'

import { API_BASE_URL, apiClient } from '../api-client'
import { withCurrentClusterPath } from '../current-cluster'
import { withSubPath } from '../subpath'
import { fetchAPI } from './shared'

type ResourcesItems<T extends ResourceType> = ResourcesTypeMap[T]['items']

export const fetchResources = async <T>(
  resource: string,
  namespace?: string,
  opts?: {
    limit?: number
    continueToken?: string
    labelSelector?: string
    fieldSelector?: string
  }
): Promise<T> => {
  const params = new URLSearchParams()

  if (opts?.limit) {
    params.append('limit', opts.limit.toString())
  }
  if (opts?.continueToken) {
    params.append('continue', opts.continueToken)
  }
  if (opts?.labelSelector) {
    params.append('labelSelector', opts.labelSelector)
  }
  if (opts?.fieldSelector) {
    params.append('fieldSelector', opts.fieldSelector)
  }
  if (resource === 'helmrelease') {
    const path = `/${resource}${namespace ? `/${namespace}` : ''}`
    return fetchAPI<T>(params.size ? `${path}?${params.toString()}` : path)
  }
  const path = await getKubernetesResourcePath(resource, namespace)
  const list = await fetchAPI<T>(
    params.size ? `${path}?${params.toString()}` : path
  )
  return enrichResourceList(resource, namespace, list)
}

// Search API types
export interface SearchResult {
  id: string
  name: string
  namespace?: string
  resourceType: string
  createdAt: string
}

export interface SearchResponse {
  results: SearchResult[]
  total: number
}

// Global search API
export const globalSearch = async (
  query: string,
  options?: {
    limit?: number
    signal?: AbortSignal
  }
): Promise<SearchResponse> => {
  if (!query.trim()) {
    return { results: [], total: 0 }
  }

  const params = new URLSearchParams({
    q: query,
    limit: String(options?.limit || 50),
  })

  const endpoint = `/search?${params.toString()}`
  const response = await apiClient.get<SearchResponse>(endpoint, {
    signal: options?.signal,
  })
  const results = response.results || []
  return {
    ...response,
    results,
    total: response.total ?? results.length,
  }
}
// Scale deployment API
export const upgradeHelmRelease = async (
  namespace: string,
  name: string,
  body?: HelmReleaseUpgradeRequest
): Promise<{ message?: string }> => {
  return apiClient.put<{ message?: string }>(
    `/helmrelease/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/upgrade`,
    body || {}
  )
}

export const dryRunUpgradeHelmRelease = async (
  namespace: string,
  name: string,
  body?: HelmReleaseUpgradeRequest
): Promise<HelmReleaseDryRunResponse> => {
  return apiClient.put<HelmReleaseDryRunResponse>(
    `/helmrelease/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/upgrade/dry-run`,
    body || {}
  )
}

export const rollbackHelmRelease = async (
  namespace: string,
  name: string,
  revision?: number
): Promise<{ message?: string }> => {
  return apiClient.put<{ message?: string }>(
    `/helmrelease/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/rollback`,
    revision ? { revision } : {}
  )
}

export const installHelmRelease = async (
  namespace: string,
  body: HelmReleaseInstallRequest
): Promise<HelmRelease> => {
  return apiClient.post<HelmRelease>(
    `/helmrelease/${encodeURIComponent(namespace)}`,
    body
  )
}

export const dryRunInstallHelmRelease = async (
  namespace: string,
  body: HelmReleaseInstallRequest
): Promise<HelmReleaseDryRunResponse> => {
  return apiClient.post<HelmReleaseDryRunResponse>(
    `/helmrelease/${encodeURIComponent(namespace)}/dry-run`,
    body
  )
}

export const fetchHelmReleaseHistory = (
  namespace: string,
  name: string
): Promise<HelmReleaseHistoryResponse> => {
  return fetchAPI<HelmReleaseHistoryResponse>(
    `/helmrelease/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/history`
  )
}

export const useHelmReleaseHistory = (
  namespace: string,
  name: string,
  options?: { enabled?: boolean; staleTime?: number }
) => {
  return useQuery({
    queryKey: ['helmrelease-history', namespace, name],
    queryFn: () => fetchHelmReleaseHistory(namespace, name),
    enabled: options?.enabled ?? true,
    staleTime: options?.staleTime || 30000,
  })
}

export const fetchHelmRepositories = (): Promise<HelmRepository[]> => {
  return fetchAPI<HelmRepository[]>('/charts/repositories')
}

export const createHelmRepository = (
  body: Pick<HelmRepository, 'name' | 'url'> & {
    username?: string
    password?: string
  }
): Promise<HelmRepository> => {
  return apiClient.post<HelmRepository>('/admin/charts/repositories', body)
}

export const deleteHelmRepository = (
  id: number
): Promise<{ message: string }> => {
  return apiClient.delete<{ message: string }>(
    `/admin/charts/repositories/${id}`
  )
}

export const fetchHelmCharts = (options?: {
  repository?: string
  query?: string
}): Promise<HelmChartList> => {
  const params = new URLSearchParams()
  if (options?.repository) {
    params.append('repository', options.repository)
  }
  if (options?.query) {
    params.append('q', options.query)
  }
  const query = params.toString()
  return fetchAPI<HelmChartList>(`/charts${query ? `?${query}` : ''}`)
}

export const fetchArtifactHubCharts = (options?: {
  query?: string
  verifiedPublisher?: boolean
  limit?: number
  offset?: number
}): Promise<HelmChartList> => {
  const params = new URLSearchParams()
  if (options?.query) {
    params.append('q', options.query)
  }
  if (options?.verifiedPublisher !== undefined) {
    params.append('verifiedPublisher', String(options.verifiedPublisher))
  }
  if (options?.limit) {
    params.append('limit', String(options.limit))
  }
  if (options?.offset) {
    params.append('offset', String(options.offset))
  }
  const query = params.toString()
  return fetchAPI<HelmChartList>(
    `/charts/artifacthub${query ? `?${query}` : ''}`
  )
}

export const fetchHelmChart = (
  repository: string,
  name: string,
  version?: string,
  source?: 'repository' | 'artifacthub'
): Promise<HelmChartDetail> => {
  const params = new URLSearchParams()
  if (version) {
    params.append('version', version)
  }
  const query = params.toString()
  const endpoint =
    source === 'artifacthub'
      ? `/charts/artifacthub/${encodeURIComponent(repository)}/${encodeURIComponent(name)}`
      : `/charts/${encodeURIComponent(repository)}/${encodeURIComponent(name)}`

  return fetchAPI<HelmChartDetail>(`${endpoint}${query ? `?${query}` : ''}`)
}

export const fetchHelmChartContent = (
  repository: string,
  name: string,
  content: HelmChartContentType,
  version?: string,
  source?: 'repository' | 'artifacthub'
): Promise<HelmChartContent> => {
  const params = new URLSearchParams()
  if (version) {
    params.append('version', version)
  }
  const query = params.toString()
  const endpoint =
    source === 'artifacthub'
      ? `/charts/artifacthub/${encodeURIComponent(repository)}/${encodeURIComponent(name)}/content/${content}`
      : `/charts/${encodeURIComponent(repository)}/${encodeURIComponent(name)}/content/${content}`

  return fetchAPI<HelmChartContent>(`${endpoint}${query ? `?${query}` : ''}`)
}

export const useHelmRepositories = () => {
  return useQuery({
    queryKey: ['helmcharts', 'repositories'],
    queryFn: fetchHelmRepositories,
  })
}

export const useHelmCharts = (options?: {
  repository?: string
  query?: string
  enabled?: boolean
}) => {
  return useQuery({
    queryKey: [
      'helmcharts',
      'charts',
      options?.repository || '',
      options?.query || '',
    ],
    queryFn: () => fetchHelmCharts(options),
    enabled: options?.enabled ?? true,
  })
}

export const useArtifactHubCharts = (options?: {
  query?: string
  verifiedPublisher?: boolean
  limit?: number
  offset?: number
  enabled?: boolean
}) => {
  return useQuery({
    queryKey: [
      'helmcharts',
      'artifacthub',
      options?.query || '',
      options?.verifiedPublisher ?? true,
      options?.limit || 20,
      options?.offset || 0,
    ],
    queryFn: () => fetchArtifactHubCharts(options),
    enabled: options?.enabled ?? true,
  })
}

export const useHelmChart = (
  repository: string | undefined,
  name: string | undefined,
  version?: string,
  source?: 'repository' | 'artifacthub',
  enabled = true
) => {
  return useQuery({
    queryKey: [
      'helmcharts',
      'chart',
      source || 'repository',
      repository,
      name,
      version || '',
    ],
    queryFn: () =>
      fetchHelmChart(repository || '', name || '', version, source),
    enabled: Boolean(enabled && repository && name),
  })
}

export const useHelmChartContent = (
  repository: string | undefined,
  name: string | undefined,
  content: HelmChartContentType,
  version?: string,
  source?: 'repository' | 'artifacthub',
  enabled = true
) => {
  return useQuery({
    queryKey: [
      'helmcharts',
      'chart-content',
      source || 'repository',
      repository,
      name,
      version || '',
      content,
    ],
    queryFn: () =>
      fetchHelmChartContent(
        repository || '',
        name || '',
        content,
        version,
        source
      ),
    enabled: Boolean(enabled && repository && name),
  })
}

// Node operation APIs
export const drainNode = async (
  nodeName: string,
  options: {
    force: boolean
    gracePeriod: number
    deleteLocalData: boolean
    ignoreDaemonsets: boolean
  }
): Promise<{
  message: string
  node: string
  pods: number
  warnings?: string | string[]
}> => {
  const endpoint = `/nodes/_all/${nodeName}/drain`
  const response = await apiClient.put<{
    message: string
    node: string
    pods: number
    warnings?: string | string[]
  }>(endpoint, options)

  return response
}

export const cordonNode = async (
  nodeName: string
): Promise<{ message: string; node: string; unschedulable: boolean }> => {
  await patchResource('nodes', nodeName, undefined, {
    spec: { unschedulable: true },
  })
  return { message: 'node cordoned', node: nodeName, unschedulable: true }
}

export const uncordonNode = async (
  nodeName: string
): Promise<{ message: string; node: string; unschedulable: boolean }> => {
  await patchResource('nodes', nodeName, undefined, {
    spec: { unschedulable: false },
  })
  return { message: 'node uncordoned', node: nodeName, unschedulable: false }
}

export const taintNode = async (
  nodeName: string,
  taint: {
    key: string
    value: string
    effect: 'NoSchedule' | 'PreferNoSchedule' | 'NoExecute'
  }
): Promise<{ message: string; node: string; taint: unknown }> => {
  const node = await fetchResource<ResourceTypeMap['nodes']>('nodes', nodeName)
  const taints = (node.spec?.taints || []).filter(
    (existing) => existing.key !== taint.key
  )
  taints.push(taint)
  await updateResource('nodes', nodeName, undefined, {
    ...node,
    spec: { ...node.spec, taints },
  })
  return { message: 'node tainted', node: nodeName, taint }
}

export const untaintNode = async (
  nodeName: string,
  key: string
): Promise<{ message: string; node: string; removedTaintKey: string }> => {
  const node = await fetchResource<ResourceTypeMap['nodes']>('nodes', nodeName)
  const taints = (node.spec?.taints || []).filter(
    (existing) => existing.key !== key
  )
  await updateResource('nodes', nodeName, undefined, {
    ...node,
    spec: { ...node.spec, taints },
  })
  return { message: 'node untainted', node: nodeName, removedTaintKey: key }
}

export const updateResource = async <T extends ResourceType>(
  resource: T,
  name: string,
  namespace: string | undefined,
  body: ResourceTypeMap[T]
): Promise<void> => {
  if (resource === 'helmrelease') {
    await apiClient.put(`/${resource}/${namespace || '_all'}/${name}`, body)
    return
  }
  const endpoint = await getKubernetesResourcePath(resource, namespace, name)
  await apiClient.put(endpoint, body)
}

export const resizePod = async (
  namespace: string,
  name: string,
  body: Partial<Pod>
): Promise<void> => {
  const endpoint = await getKubernetesResourcePath(
    'pods',
    namespace,
    name,
    'resize'
  )
  await apiClient.patch(endpoint, body, {
    headers: { 'Content-Type': 'application/merge-patch+json' },
  })
}

type DeepPartial<T> = T extends object
  ? {
      [P in keyof T]?: DeepPartial<T[P]>
    }
  : T
export const patchResource = async <T extends ResourceType>(
  resource: T,
  name: string,
  namespace: string | undefined,
  body: DeepPartial<ResourceTypeMap[T]>
): Promise<void> => {
  const endpoint = await getKubernetesResourcePath(resource, namespace, name)
  await apiClient.patch(endpoint, body, {
    headers: { 'Content-Type': 'application/merge-patch+json' },
  })
}

export const restartWorkload = async (
  resource: 'deployments' | 'statefulsets',
  name: string,
  namespace: string
): Promise<void> => {
  const endpoint = await getKubernetesResourcePath(resource, namespace, name)
  await apiClient.patch(
    endpoint,
    {
      spec: {
        template: {
          metadata: {
            annotations: {
              'kite.kubernetes.io/restartedAt': new Date().toISOString(),
            },
          },
        },
      },
    },
    { headers: { 'Content-Type': 'application/merge-patch+json' } }
  )
}

export const createResource = async <T extends ResourceType>(
  resource: T,
  namespace: string | undefined,
  body: ResourceTypeMap[T]
): Promise<ResourceTypeMap[T]> => {
  if (resource === 'helmrelease') {
    return apiClient.post<ResourceTypeMap[T]>(
      `/${resource}/${namespace || '_all'}`,
      body
    )
  }
  const endpoint = await getKubernetesResourcePath(resource, namespace)
  return await apiClient.post<ResourceTypeMap[T]>(endpoint, body)
}

export const deleteResource = async <T extends ResourceType>(
  resource: T,
  name: string,
  namespace: string | undefined,
  opts?: {
    force?: boolean
    wait?: boolean
  }
): Promise<void> => {
  if (resource === 'helmrelease') {
    await apiClient.delete(`/${resource}/${namespace || '_all'}/${name}`)
    return
  }
  const endpoint = await getKubernetesResourcePath(resource, namespace, name)
  if (opts?.force) {
    await apiClient.deleteWithBody(endpoint, {
      apiVersion: 'v1',
      kind: 'DeleteOptions',
      gracePeriodSeconds: 0,
      propagationPolicy: 'Background',
    })
    return
  }
  await apiClient.delete(endpoint)
}

// Apply resource from YAML
export interface ApplyResourceRequest {
  yaml: string
}

export interface ApplyResourceResponse {
  message: string
  kind?: string
  name?: string
  namespace?: string
  count?: number
  resources?: Array<{
    kind: string
    name: string
    namespace?: string
  }>
}

export const applyResource = async (
  yaml: string
): Promise<ApplyResourceResponse> => {
  return await apiClient.post<ApplyResourceResponse>('/resources/apply', {
    yaml,
  })
}

export const useResourcesEvents = <T extends ResourceType>(
  resource: T,
  name: string,
  namespace?: string
) => {
  return useQuery({
    queryKey: ['resource-events', resource, namespace, name],
    queryFn: async () => {
      const target = await fetchResource<ResourceTypeMap[T]>(
        resource,
        name,
        namespace
      )
      const targetIdentity = target as {
        kind?: string
        metadata?: { uid?: string }
      }
      const selectors = [
        targetIdentity.kind ? `involvedObject.kind=${targetIdentity.kind}` : '',
        `involvedObject.name=${name}`,
        targetIdentity.metadata?.uid
          ? `involvedObject.uid=${targetIdentity.metadata.uid}`
          : '',
      ].filter(Boolean)
      return fetchResources<ResourcesTypeMap['events']>('events', namespace, {
        fieldSelector: selectors.join(','),
      })
    },
    select: (data: ResourcesTypeMap['events']): ResourcesItems<'events'> =>
      data.items,
    placeholderData: (prevData) => prevData,
  })
}

export const useResources = <T extends ResourceType>(
  resource: T,
  namespace?: string,
  options?: {
    staleTime?: number
    limit?: number
    labelSelector?: string
    fieldSelector?: string
    refreshInterval?: number
    disable?: boolean
  }
) => {
  return useQuery({
    queryKey: [
      resource,
      namespace,
      options?.limit,
      options?.labelSelector,
      options?.fieldSelector,
    ],
    queryFn: () => {
      return fetchResources<ResourcesTypeMap[T]>(resource, namespace, {
        limit: options?.limit,
        continueToken: undefined,
        labelSelector: options?.labelSelector,
        fieldSelector: options?.fieldSelector,
      })
    },
    enabled: !options?.disable,
    select: (data: ResourcesTypeMap[T]): ResourcesItems<T> => data.items,
    placeholderData: (prevData) => prevData,
    refetchInterval: options?.refreshInterval || 0,
    staleTime: options?.staleTime || (resource === 'crds' ? 5000 : 1000),
  })
}

// Hook: SSE watch for resource lists (initial snapshot + ADDED/MODIFIED/DELETED)
export function useResourcesWatch<T extends ResourceType>(
  resource: T,
  namespace?: string,
  options?: {
    labelSelector?: string
    fieldSelector?: string
    enabled?: boolean
  }
) {
  const [data, setData] = useState<ResourcesItems<T> | undefined>(undefined)
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<Error | null>(null)
  const [isConnected, setIsConnected] = useState(false)
  const abortControllerRef = useRef<AbortController | null>(null)
  const metricsRefreshRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const buildQuery = useCallback(() => {
    const params = new URLSearchParams()
    if (options?.labelSelector)
      params.append('labelSelector', options.labelSelector)
    if (options?.fieldSelector)
      params.append('fieldSelector', options.fieldSelector)
    return params
  }, [options?.labelSelector, options?.fieldSelector])

  const disconnect = useCallback(() => {
    abortControllerRef.current?.abort()
    abortControllerRef.current = null
    if (metricsRefreshRef.current) {
      clearInterval(metricsRefreshRef.current)
      metricsRefreshRef.current = null
    }
  }, [])

  const connect = useCallback(async () => {
    disconnect()
    setData(undefined)
    if (options?.enabled === false) return
    const controller = new AbortController()
    abortControllerRef.current = controller
    setError(null)
    setIsConnected(false)
    setIsLoading(true)

    try {
      const endpoint = await getKubernetesResourcePath(resource, namespace)
      const list = await fetchResources<ResourcesTypeMap[T]>(
        resource,
        namespace,
        {
          labelSelector: options?.labelSelector,
          fieldSelector: options?.fieldSelector,
        }
      )
      if (controller.signal.aborted) return
      setData(list.items)
      setIsLoading(false)

      if (resource === 'pods' || resource === 'nodes') {
        metricsRefreshRef.current = setInterval(() => {
          void fetchResources<ResourcesTypeMap[T]>(resource, namespace, {
            labelSelector: options?.labelSelector,
            fieldSelector: options?.fieldSelector,
          })
            .then((refreshed) => {
              if (!controller.signal.aborted) setData(refreshed.items)
            })
            .catch((refreshError: unknown) => {
              if (!controller.signal.aborted && refreshError instanceof Error) {
                setError(refreshError)
              }
            })
        }, 15_000)
      }

      const getKey = (obj: ResourceTypeMap[T]) => {
        return (
          (obj.metadata?.namespace || '') + '/' + (obj.metadata?.name || '')
        )
      }

      const upsert = (object: ResourceTypeMap[T]) => {
        setData((prev) => {
          const arr = prev ? [...prev] : []
          const key = getKey(object)
          const idx = arr.findIndex(
            (it) => getKey(it as ResourceTypeMap[T]) === key
          )
          if (idx >= 0) arr[idx] = object
          else arr.unshift(object)
          return arr as ResourcesItems<T>
        })
      }

      const remove = (object: ResourceTypeMap[T]) => {
        setData((prev) => {
          const arr = prev ? [...prev] : []
          const key = getKey(object)
          const filtered = arr.filter(
            (it) => getKey(it as ResourceTypeMap[T]) !== key
          )
          return filtered as ResourcesItems<T>
        })
      }

      const watchParams = buildQuery()
      watchParams.set('watch', 'true')
      watchParams.set('allowWatchBookmarks', 'true')
      const resourceVersion = list.metadata?.resourceVersion
      if (resourceVersion) watchParams.set('resourceVersion', resourceVersion)
      const response = await apiClient.request(
        `${endpoint}?${watchParams.toString()}`,
        { signal: controller.signal }
      )
      if (!response.ok) {
        const status = (await response.json().catch(() => undefined)) as
          { message?: string } | undefined
        throw new Error(status?.message || `Watch failed (${response.status})`)
      }
      if (!response.body) throw new Error('Watch response has no body')
      setIsConnected(true)

      const reader = response.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''
      while (!controller.signal.aborted) {
        const { done, value } = await reader.read()
        buffer += decoder.decode(value, { stream: !done })
        const lines = buffer.split('\n')
        buffer = lines.pop() || ''
        for (const line of lines) {
          if (!line.trim()) continue
          const event = JSON.parse(line) as {
            type: 'ADDED' | 'MODIFIED' | 'DELETED' | 'BOOKMARK' | 'ERROR'
            object: ResourceTypeMap[T] & { message?: string }
          }
          if (event.type === 'ADDED' || event.type === 'MODIFIED') {
            upsert(event.object)
          } else if (event.type === 'DELETED') {
            remove(event.object)
          } else if (event.type === 'ERROR') {
            throw new Error(event.object.message || 'Kubernetes watch failed')
          }
        }
        if (done) break
      }
    } catch (err) {
      if (!controller.signal.aborted && err instanceof Error) setError(err)
      setIsLoading(false)
      setIsConnected(false)
    } finally {
      if (abortControllerRef.current === controller) {
        abortControllerRef.current = null
        setIsConnected(false)
      }
      if (metricsRefreshRef.current) {
        clearInterval(metricsRefreshRef.current)
        metricsRefreshRef.current = null
      }
    }
  }, [
    buildQuery,
    disconnect,
    namespace,
    options?.enabled,
    options?.fieldSelector,
    options?.labelSelector,
    resource,
  ])

  const refetch = useCallback(() => {
    disconnect()
    void connect()
  }, [disconnect, connect])

  useEffect(() => {
    if (options?.enabled === false) return
    void connect()
    return () => {
      disconnect()
    }
  }, [connect, disconnect, options?.enabled])

  return { data, isLoading, error, isConnected, refetch, stop: disconnect }
}

export const fetchResource = <T>(
  resource: string,
  name: string,
  namespace?: string,
  cluster?: string | null
): Promise<T> => {
  if (resource === 'helmrelease') {
    const path = `/${resource}/${namespace || '_all'}/${name}`
    return fetchAPI<T>(withCurrentClusterPath(path, cluster))
  }
  return getKubernetesResourcePath(resource, namespace, name).then((path) =>
    fetchAPI<T>(withCurrentClusterPath(path, cluster))
  )
}
export const useResource = <T extends keyof ResourceTypeMap>(
  resource: T,
  name: string,
  namespace?: string,
  options?: { staleTime?: number; refreshInterval?: number }
) => {
  const { currentCluster } = useCluster()
  const ns = namespace || '_all'
  return useQuery({
    queryKey: getResourceQueryKey(resource, ns, name, currentCluster),
    queryFn: () => {
      return fetchResource<ResourceTypeMap[T]>(
        resource,
        name,
        ns,
        currentCluster
      )
    },
    refetchOnWindowFocus: 'always',
    refetchInterval: options?.refreshInterval || 0, // Default to no auto-refresh
    staleTime: options?.staleTime || 1000,
  })
}
// Pod describe API
export const fetchDescribe = async (
  resourceType: ResourceType,
  name: string,
  namespace?: string
): Promise<{ result: string }> => {
  const endpoint = `/${resourceType}/${namespace ?? '_all'}/${name}/describe`
  return fetchAPI<{ result: string }>(endpoint)
}

export const useDescribe = (
  resourceType: ResourceType,
  name: string,
  namespace?: string,
  options?: { staleTime?: number; enabled?: boolean }
) => {
  return useQuery({
    queryKey: [resourceType, name, namespace, 'describe'],
    queryFn: () => fetchDescribe(resourceType, name, namespace),
    enabled: (options?.enabled ?? true) && !!name,
    staleTime: options?.staleTime || 0,
    retry: 0,
  })
}
export interface FileInfo {
  name: string
  isDir: boolean
  size: string
  modTime: string
  mode: string
  uid: string
  gid: string
}

export const podListFiles = async (
  namespace: string,
  podName: string,
  container: string,
  path: string,
  options?: RequestInit
): Promise<FileInfo[]> => {
  const params = new URLSearchParams({
    container,
    path,
  })
  return apiClient.get<FileInfo[]>(
    `${withCurrentClusterPath(`/pods/${namespace}/${podName}/files`)}?${params.toString()}`,
    options
  )
}

export const podDownloadFile = (
  namespace: string,
  podName: string,
  container: string,
  path: string
) => {
  const params = new URLSearchParams({
    container,
    path,
  })
  const url = withSubPath(
    `${API_BASE_URL}${withCurrentClusterPath(`/pods/${namespace}/${podName}/files/download`)}?${params.toString()}`
  )
  window.open(url, '_blank')
}

export const podPreviewFile = (
  namespace: string,
  podName: string,
  container: string,
  path: string
) => {
  const params = new URLSearchParams({
    container,
    path,
  })
  const url = withSubPath(
    `${API_BASE_URL}${withCurrentClusterPath(`/pods/${namespace}/${podName}/files/preview`)}?${params.toString()}`
  )
  window.open(url, '_blank')
}

export const podUploadFile = async (
  namespace: string,
  podName: string,
  container: string,
  path: string,
  file: File
): Promise<void> => {
  const formData = new FormData()
  formData.append('file', file)
  const params = new URLSearchParams({
    container,
    path,
  })

  await apiClient.put(
    `${withCurrentClusterPath(`/pods/${namespace}/${podName}/files/upload`)}?${params.toString()}`,
    formData
  )
}

export const fetchTemplates = async (): Promise<ResourceTemplate[]> => {
  return fetchAPI<ResourceTemplate[]>('/templates/')
}

export const createTemplate = async (
  data: Omit<ResourceTemplate, 'id'>
): Promise<ResourceTemplate> => {
  return apiClient.post<ResourceTemplate>('/admin/templates/', data)
}

export const updateTemplate = async (
  id: number,
  data: Partial<ResourceTemplate>
): Promise<ResourceTemplate> => {
  return apiClient.put<ResourceTemplate>(`/admin/templates/${id}`, data)
}

export const deleteTemplate = async (id: number): Promise<void> => {
  await apiClient.delete(`/admin/templates/${id}`)
}

export const useTemplates = (options?: { staleTime?: number }) => {
  return useQuery({
    queryKey: ['templates'],
    queryFn: fetchTemplates,
    staleTime: options?.staleTime || 30000,
  })
}
export async function getImageTags(image: string): Promise<ImageTagInfo[]> {
  if (!image) return []
  const resp = await apiClient.get<ImageTagInfo[]>(
    `/image/tags?image=${encodeURIComponent(image)}`
  )
  return resp
}

export function useImageTags(image: string, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: ['image-tags', image],
    queryFn: () => getImageTags(image),
    enabled: !!image && (options?.enabled ?? true),
    staleTime: 60 * 1000, // 1 min
    placeholderData: (prev) => prev,
  })
}

export async function getRelatedResources(
  resource: ResourceType,
  name: string,
  namespace?: string
) {
  const resp = await apiClient.get<RelatedResources[]>(
    `/${resource}/${namespace ? namespace : '_all'}/${name}/related`
  )
  return resp
}

export function useRelatedResources(
  resource: ResourceType,
  name: string,
  namespace?: string
) {
  return useQuery({
    queryKey: ['related-resources', resource, name, namespace],
    queryFn: () => getRelatedResources(resource, name, namespace),
    staleTime: 60 * 1000, // 1 min
    placeholderData: (prev) => prev,
  })
}
// Resource History API
export const fetchResourceHistory = (
  resourceType: string,
  namespace: string,
  name: string,
  page: number = 1,
  pageSize: number = 10
): Promise<ResourceHistoryResponse> => {
  const endpoint = `/${resourceType}/${namespace}/${name}/history?page=${page}&pageSize=${pageSize}`
  return fetchAPI<ResourceHistoryResponse>(endpoint)
}

export const fetchWorkloadRevisions = (
  resourceType: WorkloadRevisionResourceType,
  namespace: string,
  name: string
): Promise<WorkloadRevisionsResponse> => {
  return fetchAPI<WorkloadRevisionsResponse>(
    `/${resourceType}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/revisions`
  )
}

export const useWorkloadRevisions = (
  resourceType: WorkloadRevisionResourceType,
  namespace: string,
  name: string,
  options?: { enabled?: boolean; staleTime?: number }
) => {
  return useQuery({
    queryKey: ['workload-revisions', resourceType, namespace, name],
    queryFn: () => fetchWorkloadRevisions(resourceType, namespace, name),
    enabled: options?.enabled ?? true,
    staleTime: options?.staleTime ?? 30000,
  })
}

export const rollbackWorkload = async (
  resourceType: WorkloadRevisionResourceType,
  namespace: string,
  name: string,
  revision: number
): Promise<{ message?: string; revision?: number }> => {
  return apiClient.put<{ message?: string; revision?: number }>(
    `/${resourceType}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/rollback`,
    { revision }
  )
}

export const useResourceHistory = (
  resourceType: string,
  namespace: string,
  name: string,
  page: number = 1,
  pageSize: number = 10,
  options?: { enabled?: boolean; staleTime?: number }
) => {
  return useQuery({
    queryKey: [
      'resource-history',
      resourceType,
      namespace,
      name,
      page,
      pageSize,
    ],
    queryFn: () =>
      fetchResourceHistory(resourceType, namespace, name, page, pageSize),
    enabled: options?.enabled ?? true,
    staleTime: options?.staleTime || 30000, // 30 seconds cache
  })
}
export const usePodFiles = (
  namespace: string,
  podName: string,
  container: string,
  path: string,
  options?: { enabled?: boolean }
) => {
  return useQuery({
    queryKey: ['pod-files', namespace, podName, container, path],
    queryFn: () => podListFiles(namespace, podName, container, path),
    enabled: options?.enabled !== false,
    staleTime: 10000, // 10 seconds cache
  })
}
