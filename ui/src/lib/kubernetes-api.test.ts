import { afterEach, describe, expect, it, vi } from 'vitest'

import { apiClient } from './api-client'
import {
  getKubernetesResourceDescriptor,
  getKubernetesResourcePath,
} from './kubernetes-api'

afterEach(() => vi.restoreAllMocks())

describe('Kubernetes resource paths', () => {
  it('builds core and grouped resource paths with Kubernetes scope semantics', async () => {
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
