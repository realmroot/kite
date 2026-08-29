/* eslint-disable react-refresh/only-export-components */
import {
  createContext,
  ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
} from 'react'

import type { BootstrapCapabilities } from '@/lib/api'
import {
  initiateOIDCLogin,
  logout as logoutUser,
  refreshAuthToken,
  useBootstrap,
  type AuthUser,
} from '@/lib/api'
import { withSubPath } from '@/lib/subpath'

interface User extends AuthUser {
  isAdmin(): boolean
  Key(): string
}

interface AuthContextType {
  user: User | null
  isLoading: boolean
  hasGlobalSidebarPreference: boolean
  globalSidebarPreference: string
  providerName: string
  loginPrompt: string
  capabilities: BootstrapCapabilities
  login: () => Promise<void>
  logout: () => Promise<void>
  checkAuth: () => Promise<void>
  refreshToken: () => Promise<void>
}

const AuthContext = createContext<AuthContextType | undefined>(undefined)
const defaultCapabilities: BootstrapCapabilities = {
  kubectlEnabled: false,
}

export function useAuth() {
  const context = useContext(AuthContext)
  if (!context) throw new Error('useAuth must be used within an AuthProvider')
  return context
}

function normalizeUser(user: AuthUser, platformAdmin: boolean): User {
  return {
    ...user,
    isAdmin: () => platformAdmin,
    Key() {
      return this.username || this.sub || this.id
    },
  }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const {
    data: bootstrap,
    isLoading,
    refetch: refetchBootstrap,
  } = useBootstrap()

  const checkAuth = useCallback(async () => {
    await refetchBootstrap()
  }, [refetchBootstrap])

  const login = useCallback(async () => {
    const { auth_url } = await initiateOIDCLogin()
    window.location.href = auth_url
  }, [])

  const logout = useCallback(async () => {
    await logoutUser()
    await refetchBootstrap()
    window.location.href = withSubPath('/login')
  }, [refetchBootstrap])

  const refreshToken = useCallback(async () => {
    try {
      await refreshAuthToken()
    } catch {
      await refetchBootstrap()
      window.location.href = withSubPath('/login')
    }
  }, [refetchBootstrap])

  const user = useMemo(
    () =>
      bootstrap?.user
        ? normalizeUser(bootstrap.user, bootstrap.platformAdmin)
        : null,
    [bootstrap?.platformAdmin, bootstrap?.user]
  )

  useEffect(() => {
    if (!user) return
    const interval = window.setInterval(refreshToken, 30 * 60 * 1000)
    return () => window.clearInterval(interval)
  }, [refreshToken, user])

  const globalSidebarPreference = bootstrap?.globalSidebarPreference ?? ''
  const value = useMemo<AuthContextType>(
    () => ({
      user,
      isLoading,
      hasGlobalSidebarPreference:
        bootstrap?.hasGlobalSidebarPreference ?? false,
      globalSidebarPreference,
      providerName: bootstrap?.auth.providerName ?? 'OpenID Connect',
      loginPrompt: bootstrap?.auth.loginPrompt ?? '',
      capabilities: bootstrap?.capabilities ?? defaultCapabilities,
      login,
      logout,
      checkAuth,
      refreshToken,
    }),
    [
      bootstrap,
      checkAuth,
      globalSidebarPreference,
      isLoading,
      login,
      logout,
      refreshToken,
      user,
    ]
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
