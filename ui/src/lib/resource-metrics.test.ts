import { afterEach, describe, expect, it, vi } from 'vitest'

import { apiClient } from './api-client'
import { clearKubernetesDiscoveryCache } from './kubernetes-api'
import { enrichResourceList } from './resource-metrics'

afterEach(() => {
  clearKubernetesDiscoveryCache()
  vi.restoreAllMocks()
})

describe('standard Kubernetes metrics composition', () => {
  it('preserves pod usage, requests, and limits without a Kite list handler', async () => {
    vi.spyOn(apiClient, 'get').mockImplementation(async (path) => {
      if (path === '/kubernetes/apis') {
        return {
          groups: [
            {
              name: 'metrics.k8s.io',
              preferredVersion: { groupVersion: 'metrics.k8s.io/v1beta1' },
              versions: [{ groupVersion: 'metrics.k8s.io/v1beta1' }],
            },
          ],
        }
      }
      if (path === '/kubernetes/apis/metrics.k8s.io/v1beta1') {
        return {
          groupVersion: 'metrics.k8s.io/v1beta1',
          resources: [{ name: 'pods', namespaced: true }],
        }
      }
      throw new Error(`unexpected discovery path ${path}`)
    })
    vi.spyOn(apiClient, 'request').mockResolvedValue(
      new Response(
        JSON.stringify({
          items: [
            {
              metadata: { name: 'api', namespace: 'default' },
              containers: [{ usage: { cpu: '500000000n', memory: '512Ki' } }],
            },
          ],
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } }
      )
    )

    const result = await enrichResourceList('pods', 'default', {
      items: [
        {
          metadata: { name: 'api', namespace: 'default' },
          spec: {
            containers: [
              {
                name: 'api',
                resources: {
                  requests: { cpu: '250m', memory: '1Mi' },
                  limits: { cpu: '1', memory: '2Mi' },
                },
              },
            ],
          },
        },
      ],
    })

    expect(result.items[0].metrics).toEqual({
      cpuUsage: 500,
      memoryUsage: 512 * 1024,
      cpuRequest: 250,
      memoryRequest: 1024 * 1024,
      cpuLimit: 1000,
      memoryLimit: 2 * 1024 * 1024,
    })
  })
})
