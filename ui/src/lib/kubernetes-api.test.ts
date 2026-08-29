import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { apiClient } from './api-client'
import {
  clearKubernetesDiscoveryCache,
  getKubernetesResourceDescriptor,
  getKubernetesResourcePath,
} from './kubernetes-api'

beforeEach(clearKubernetesDiscoveryCache)
afterEach(() => vi.restoreAllMocks())

describe('Kubernetes resource paths', () => {
  it('builds core and grouped resource paths with Kubernetes scope semantics', async () => {
    vi.spyOn(apiClient, 'get').mockImplementation(async (path) => {
      if (path === '/kubernetes/api/v1') {
        return {
          groupVersion: 'v1',
          resources: [
            { name: 'pods', namespaced: true },
            { name: 'nodes', namespaced: false },
          ],
        }
      }
      if (path === '/kubernetes/apis') {
        return {
          groups: [
            {
              name: 'apps',
              preferredVersion: { groupVersion: 'apps/v1' },
              versions: [{ groupVersion: 'apps/v1' }],
            },
          ],
        }
      }
      if (path === '/kubernetes/apis/apps/v1') {
        return {
          groupVersion: 'apps/v1',
          resources: [{ name: 'deployments', namespaced: true }],
        }
      }
      throw new Error(`unexpected discovery path ${path}`)
    })
    await expect(
      getKubernetesResourcePath('pods', 'default', 'api-0')
    ).resolves.toBe('/kubernetes/api/v1/namespaces/default/pods/api-0')
    await expect(
      getKubernetesResourcePath('deployments', '_all')
    ).resolves.toBe('/kubernetes/apis/apps/v1/deployments')
    await expect(
      getKubernetesResourcePath('nodes', undefined, 'worker-0')
    ).resolves.toBe('/kubernetes/api/v1/nodes/worker-0')
  })

  it('uses the server preferred version instead of a hard-coded table', async () => {
    vi.spyOn(apiClient, 'get').mockImplementation(async (path) => {
      if (path === '/kubernetes/apis') {
        return {
          groups: [
            {
              name: 'gateway.networking.k8s.io',
              preferredVersion: {
                groupVersion: 'gateway.networking.k8s.io/v2',
              },
              versions: [{ groupVersion: 'gateway.networking.k8s.io/v2' }],
            },
          ],
        }
      }
      if (path === '/kubernetes/apis/gateway.networking.k8s.io/v2') {
        return {
          groupVersion: 'gateway.networking.k8s.io/v2',
          resources: [{ name: 'gateways', namespaced: true }],
        }
      }
      throw new Error(`unexpected discovery path ${path}`)
    })

    await expect(
      getKubernetesResourcePath('gateways', 'default')
    ).resolves.toBe(
      '/kubernetes/apis/gateway.networking.k8s.io/v2/namespaces/default/gateways'
    )
  })

  it('discovers custom-resource group, version, plural, and scope from its CRD', async () => {
    vi.spyOn(apiClient, 'get').mockResolvedValue({
      apiVersion: 'apiextensions.k8s.io/v1',
      kind: 'CustomResourceDefinition',
      metadata: { name: 'widgets.example.com' },
      spec: {
        group: 'example.com',
        names: { kind: 'Widget', plural: 'widgets' },
        scope: 'Namespaced',
        versions: [
          { name: 'v1beta1', served: true, storage: false },
          { name: 'v1', served: true, storage: true },
        ],
      },
    })

    await expect(
      getKubernetesResourceDescriptor('widgets.example.com')
    ).resolves.toEqual({
      apiVersion: 'example.com/v1',
      plural: 'widgets',
      namespaced: true,
    })
    await expect(
      getKubernetesResourcePath('widgets.example.com', 'tenant-a', 'example')
    ).resolves.toBe(
      '/kubernetes/apis/example.com/v1/namespaces/tenant-a/widgets/example'
    )
  })
})
