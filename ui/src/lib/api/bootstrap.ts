import { useQuery } from '@tanstack/react-query'

import type { AuthUser } from './auth'
import { fetchAPI } from './shared'

export interface BootstrapCapabilities {
  kubectlEnabled: boolean
  clusterGatewayEnabled: boolean
}

export interface AuthProviderCatalog {
  providerName: string
  loginPrompt: string
}

export interface BootstrapResponse {
  auth: AuthProviderCatalog
  capabilities: BootstrapCapabilities
  user: AuthUser | null
  platformAdmin: boolean
  hasGlobalSidebarPreference: boolean
  globalSidebarPreference: string
}

export const fetchBootstrap = (): Promise<BootstrapResponse> => {
  return fetchAPI<BootstrapResponse>('/bootstrap')
}

export const useBootstrap = (options?: {
  enabled?: boolean
  staleTime?: number
}) => {
  return useQuery({
    queryKey: ['bootstrap'],
    queryFn: fetchBootstrap,
    enabled: options?.enabled ?? true,
    staleTime: options?.staleTime ?? 0,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
}
