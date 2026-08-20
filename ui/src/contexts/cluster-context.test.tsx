import type { ReactNode } from 'react'
import { renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { useCluster } from '@/hooks/use-cluster'

import { ClusterProvider } from './cluster-context'

const mocks = vi.hoisted(() => ({
  resetQueries: vi.fn().mockResolvedValue(undefined),
  clusterQuery: {
    data: [{ name: 'local-kind', isDefault: true }],
    isLoading: false,
    error: null,
  },
}))

vi.mock('@tanstack/react-query', () => ({
  useQueryClient: () => ({ resetQueries: mocks.resetQueries }),
}))

vi.mock('@/lib/api/cluster', () => ({
  useCurrentClusterList: () => mocks.clusterQuery,
}))

const wrapper = ({ children }: { children: ReactNode }) => (
  <ClusterProvider>{children}</ClusterProvider>
)

describe('ClusterProvider', () => {
  beforeEach(() => {
    localStorage.setItem('current-cluster', 'removed-cluster')
    sessionStorage.setItem('current-cluster', 'removed-cluster')
    mocks.resetQueries.mockClear()
    mocks.clusterQuery.data = [{ name: 'local-kind', isDefault: true }]
  })

  it('recovers a removed persisted cluster without requiring a reload', async () => {
    const { result } = renderHook(() => useCluster(), { wrapper })

    await waitFor(() => {
      expect(result.current.currentCluster).toBe('local-kind')
    })

    expect(localStorage.getItem('current-cluster')).toBe('local-kind')
    expect(sessionStorage.getItem('current-cluster')).toBe('local-kind')
    expect(mocks.resetQueries).toHaveBeenCalledOnce()
  })
})
