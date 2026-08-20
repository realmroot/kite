import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'

import { SettingsHint } from './settings-hint'

vi.mock('@/lib/api', () => ({
  useClusterList: () => ({ data: [] }),
}))

describe('SettingsHint', () => {
  it('offers only configuration supported by the native Kubernetes fork', () => {
    render(
      <MemoryRouter>
        <SettingsHint />
      </MemoryRouter>
    )

    expect(screen.getByRole('link', { name: /Prometheus/ })).toHaveAttribute(
      'href',
      '/settings?tab=clusters'
    )
    expect(screen.queryByText('Authentication')).not.toBeInTheDocument()
    expect(screen.queryByText('RBAC')).not.toBeInTheDocument()
  })
})
