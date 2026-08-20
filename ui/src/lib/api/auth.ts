import { authApiClient } from '../api-client'

export interface AuthUser {
  id: string
  issuer: string
  sub: string
  username: string
  name: string
  avatar_url: string
  oidc_groups?: string[]
  sidebar_preference?: string
}

export interface OIDCLoginResponse {
  auth_url: string
  provider: string
}

export const initiateOIDCLogin = async (): Promise<OIDCLoginResponse> => {
  return authApiClient.get<OIDCLoginResponse>('/auth/login', {
    retryOnUnauthorized: false,
  })
}

export const refreshAuthToken = async (): Promise<void> => {
  await authApiClient.post<void>('/auth/refresh', undefined, {
    retryOnUnauthorized: false,
  })
}

export const logout = async (): Promise<void> => {
  await authApiClient.post<void>('/auth/logout', undefined, {
    retryOnUnauthorized: false,
  })
}
