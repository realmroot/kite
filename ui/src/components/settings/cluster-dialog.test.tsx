import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { Cluster } from '@/types/api'

import { ClusterDialog } from './cluster-dialog'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (_key: string, fallback?: string) => fallback ?? _key,
  }),
}))

const connectorCluster: Cluster = {
  id: 7,
  name: 'Production',
  connectionMode: 'connector',
  connectorId: 'production',
  connectorUrl: 'https://connector.example.test',
  apiServerUrl: '',
  enabled: true,
  clusterAgent: false,
  connected: true,
  isDefault: false,
  createdAt: '2026-08-28T00:00:00Z',
  updatedAt: '2026-08-28T00:00:00Z',
}

describe('ClusterDialog Gateway connector mode', () => {
  it('shows credential-free Connector settings and preserves the stable Connector ID when renamed', () => {
    const onSubmit = vi.fn()
    render(
      <ClusterDialog
        open
        onOpenChange={vi.fn()}
        cluster={connectorCluster}
        onSubmit={onSubmit}
        gatewayEnabled
      />
    )

    const connectorId = screen.getByLabelText('Connector ID *')
    expect(connectorId).toHaveValue('production')
    expect(connectorId).toHaveAttribute('readonly')
    expect(
      screen.queryByLabelText('Kubernetes API Server URL *')
    ).not.toBeInTheDocument()
    expect(
      screen.getByText(/never stores Kubernetes credentials/i)
    ).toBeVisible()

    fireEvent.change(screen.getByLabelText('Cluster Name *'), {
      target: { value: 'Renamed Production' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }))

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'Renamed Production',
        connectionMode: 'connector',
        connectorId: 'production',
        connectorUrl: 'https://connector.example.test',
      })
    )
  })
})
