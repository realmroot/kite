import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { Cluster } from '@/types/api'

import { ClusterDialog } from './cluster-dialog'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (_key: string, fallback?: string) => fallback ?? _key,
  }),
}))

const gatewayCluster: Cluster = {
  id: 7,
  name: 'Production',
  connectionMode: 'direct',
  apiServerUrl: 'https://api.example.test',
  enabled: true,
  clusterAgent: false,
  connected: true,
  isDefault: false,
  createdAt: '2026-08-28T00:00:00Z',
  updatedAt: '2026-08-28T00:00:00Z',
}

describe('ClusterDialog Gateway mode', () => {
  it('shows only the cluster catalog fields', () => {
    const onSubmit = vi.fn()
    render(
      <ClusterDialog
        open
        onOpenChange={vi.fn()}
        cluster={gatewayCluster}
        onSubmit={onSubmit}
        gatewayEnabled
      />
    )

    expect(screen.queryByLabelText('Connection Mode')).not.toBeInTheDocument()
    expect(screen.queryByText('Connector')).not.toBeInTheDocument()
    expect(screen.getByLabelText('Kubernetes API Server URL *')).toHaveValue(
      'https://api.example.test'
    )

    fireEvent.change(screen.getByLabelText('Cluster Name *'), {
      target: { value: 'Renamed Production' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'Renamed Production',
        connectionMode: 'direct',
        apiServerUrl: 'https://api.example.test',
      })
    )
  })
})
