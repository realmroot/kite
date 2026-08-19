import { useQuery } from '@tanstack/react-query'

import { fetchBootstrap, useBootstrap } from './bootstrap'
import { fetchAPI } from './shared'

// Initialize API types
export interface InitCheckResponse {
  initialized: boolean
  step: number
}

// Initialize API function
export const fetchInitCheck = async (): Promise<InitCheckResponse> => {
  return (await fetchBootstrap()).setup
}

export const useInitCheck = () => {
  const query = useBootstrap({ staleTime: 0 })
  return {
    ...query,
    data: query.data?.setup,
  }
}

// Version information
export interface VersionInfo {
  version: string
  buildDate: string
  commitId: string
  hasNewVersion: boolean
  releaseUrl: string
}

export const fetchVersionInfo = (): Promise<VersionInfo> => {
  return fetchAPI<VersionInfo>('/version')
}

export const useVersionInfo = () => {
  return useQuery({
    queryKey: ['version-info'],
    queryFn: fetchVersionInfo,
    staleTime: 1000 * 60 * 60, // 1 hour
    refetchInterval: 0, // No auto-refresh
  })
}
