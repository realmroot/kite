import { useQuery } from '@tanstack/react-query'

import { AuditLogResponse, Cluster } from '@/types/api'

import { apiClient } from '../api-client'
import { fetchAPI } from './shared'

export interface ClusterCreateRequest {
  name: string
  description?: string
  apiServerUrl?: string
  caBundle?: string
  tlsServerName?: string
  prometheusURL?: string
  isDefault?: boolean
  enabled?: boolean
}

export interface ClusterUpdateRequest {
  name: string
  description?: string
  apiServerUrl?: string
  caBundle?: string
  tlsServerName?: string
  prometheusURL?: string
  isDefault?: boolean
  enabled?: boolean
}

// Get cluster list for management
export const fetchClusterList = (): Promise<Cluster[]> => {
  return fetchAPI<Cluster[]>('/admin/clusters/')
}

export const useClusterList = (options?: {
  staleTime?: number
  refetchInterval?: number | false
}) => {
  return useQuery({
    queryKey: ['cluster-list'],
    queryFn: fetchClusterList,
    staleTime: options?.staleTime ?? 30000, // 30 seconds cache
    refetchInterval: options?.refetchInterval,
  })
}

// Create cluster
export const createCluster = async (
  clusterData: ClusterCreateRequest
): Promise<{ id: number; message: string }> => {
  return await apiClient.post<{ id: number; message: string }>(
    '/admin/clusters/',
    clusterData
  )
}

// Update cluster
export const updateCluster = async (
  id: number,
  clusterData: ClusterUpdateRequest
): Promise<{ message: string }> => {
  return await apiClient.put<{ message: string }>(
    `/admin/clusters/${id}`,
    clusterData
  )
}

// Delete cluster
export const deleteCluster = async (
  id: number
): Promise<{ message: string }> => {
  return await apiClient.delete<{ message: string }>(`/admin/clusters/${id}`)
}

export const fetchAuditLogs = async (
  page = 1,
  size = 20,
  operator?: string,
  search?: string,
  operation?: string,
  cluster?: string,
  resourceType?: string,
  resourceName?: string,
  namespace?: string
): Promise<AuditLogResponse> => {
  const params = new URLSearchParams({
    page: String(page),
    size: String(size),
  })
  if (operator) {
    params.set('operator', operator)
  }
  if (search) {
    params.set('search', search)
  }
  if (operation) {
    params.set('operation', operation)
  }
  if (cluster) {
    params.set('cluster', cluster)
  }
  if (resourceType) {
    params.set('resourceType', resourceType)
  }
  if (resourceName) {
    params.set('resourceName', resourceName)
  }
  if (namespace) {
    params.set('namespace', namespace)
  }
  return fetchAPI<AuditLogResponse>(`/admin/audit-logs?${params.toString()}`)
}

export const useAuditLogs = (
  page = 1,
  size = 20,
  operator?: string,
  search?: string,
  operation?: string,
  cluster?: string,
  resourceType?: string,
  resourceName?: string,
  namespace?: string
) => {
  return useQuery<AuditLogResponse, Error>({
    queryKey: [
      'audit-logs',
      page,
      size,
      operator,
      search,
      operation,
      cluster,
      resourceType,
      resourceName,
      namespace,
    ],
    queryFn: () =>
      fetchAuditLogs(
        page,
        size,
        operator,
        search,
        operation,
        cluster,
        resourceType,
        resourceName,
        namespace
      ),
    staleTime: 20000,
  })
}
export interface GeneralSetting {
  kubectlEnabled: boolean
  kubectlImage: string
  nodeTerminalImage: string
  enableAnalytics: boolean
  analyticsConfigured: boolean
  enableVersionCheck: boolean
  loginPrompt: string
}

export interface GeneralSettingUpdateRequest {
  kubectlEnabled?: boolean
  kubectlImage?: string
  nodeTerminalImage?: string
  enableAnalytics?: boolean
  enableVersionCheck?: boolean
  loginPrompt?: string
}

export const fetchGeneralSetting = async (): Promise<GeneralSetting> => {
  return fetchAPI<GeneralSetting>('/admin/general-setting/')
}

export const useGeneralSetting = (options?: {
  staleTime?: number
  enabled?: boolean
}) => {
  return useQuery({
    queryKey: ['general-setting'],
    queryFn: fetchGeneralSetting,
    enabled: options?.enabled ?? true,
    staleTime: options?.staleTime || 30000,
  })
}

export const updateGeneralSetting = async (
  data: GeneralSettingUpdateRequest
): Promise<GeneralSetting> => {
  return await apiClient.put<GeneralSetting>('/admin/general-setting/', data)
}

export const setGlobalSidebarPreference = async (sidebarPreference: string) => {
  return await apiClient.post<{ success: boolean }>(
    '/admin/sidebar_preference/global',
    {
      sidebar_preference: sidebarPreference,
    }
  )
}

export const clearGlobalSidebarPreference = async () => {
  return await apiClient.delete<{ success: boolean }>(
    '/admin/sidebar_preference/global'
  )
}
