import type { CustomResourceDefinition } from 'kubernetes-types/apiextensions/v1'

import { apiClient } from '@/lib/api-client'
import { getCurrentCluster } from '@/lib/current-cluster'

type KubernetesResourceDescriptor = {
  apiVersion: string
  plural: string
  namespaced: boolean
}

type APIResource = {
  name: string
  namespaced: boolean
}

type APIResourceList = {
  groupVersion: string
  resources: APIResource[]
}

type APIGroupList = {
  groups: Array<{
    name: string
    preferredVersion?: { groupVersion: string }
    versions: Array<{ groupVersion: string }>
  }>
}

const groupHints: Record<string, string> = {}

function assignGroup(group: string, resources: string[]) {
  for (const resource of resources) groupHints[resource] = group
}

assignGroup('', [
  'pods',
  'namespaces',
  'nodes',
  'services',
  'endpoints',
  'podtemplates',
  'replicationcontrollers',
  'limitranges',
  'resourcequotas',
  'componentstatuses',
  'configmaps',
  'secrets',
  'persistentvolumes',
  'persistentvolumeclaims',
  'serviceaccounts',
  'events',
])
assignGroup('apps', [
  'deployments',
  'replicasets',
  'controllerrevisions',
  'statefulsets',
  'daemonsets',
])
assignGroup('policy', ['poddisruptionbudgets'])
assignGroup('batch', ['jobs', 'cronjobs'])
assignGroup('networking.k8s.io', [
  'ingresses',
  'networkpolicies',
  'ingressclasses',
  'ipaddresses',
  'servicecidrs',
])
assignGroup('storage.k8s.io', [
  'storageclasses',
  'volumeattachments',
  'csidrivers',
  'csinodes',
  'csistoragecapacities',
  'volumeattributesclasses',
])
assignGroup('rbac.authorization.k8s.io', [
  'roles',
  'rolebindings',
  'clusterroles',
  'clusterrolebindings',
])
assignGroup('certificates.k8s.io', [
  'certificatesigningrequests',
  'clustertrustbundles',
  'podcertificaterequests',
])
assignGroup('coordination.k8s.io', ['leases', 'leasecandidates'])
assignGroup('node.k8s.io', ['runtimeclasses'])
assignGroup('scheduling.k8s.io', ['priorityclasses', 'workloads', 'podgroups'])
assignGroup('flowcontrol.apiserver.k8s.io', [
  'flowschemas',
  'prioritylevelconfigurations',
])
assignGroup('admissionregistration.k8s.io', [
  'validatingadmissionpolicies',
  'validatingadmissionpolicybindings',
  'validatingwebhookconfigurations',
  'mutatingwebhookconfigurations',
  'mutatingadmissionpolicies',
  'mutatingadmissionpolicybindings',
])
assignGroup('resource.k8s.io', [
  'resourceslices',
  'resourceclaims',
  'deviceclasses',
  'resourceclaimtemplates',
  'devicetaintrules',
  'resourcepoolstatusrequests',
])
assignGroup('internal.apiserver.k8s.io', ['storageversions'])
assignGroup('storagemigration.k8s.io', ['storageversionmigrations'])
assignGroup('apiextensions.k8s.io', ['crds'])
assignGroup('metrics.k8s.io', ['podmetrics', 'nodemetrics'])
assignGroup('gateway.networking.k8s.io', ['gateways', 'httproutes'])
assignGroup('autoscaling', ['horizontalpodautoscalers'])
assignGroup('discovery.k8s.io', ['endpointslices'])

const pluralAliases: Record<string, string> = {
  crds: 'customresourcedefinitions',
  podmetrics: 'pods',
  nodemetrics: 'nodes',
}

const descriptors = new Map<string, Promise<KubernetesResourceDescriptor>>()
const apiGroups = new Map<string, Promise<APIGroupList>>()
const resourceLists = new Map<string, Promise<APIResourceList>>()

function clusterKey() {
  return getCurrentCluster() || '_default'
}

function cached<T>(
  cache: Map<string, Promise<T>>,
  key: string,
  loader: () => Promise<T>
) {
  const existing = cache.get(key)
  if (existing) return existing
  const promise = loader().catch((error) => {
    cache.delete(key)
    throw error
  })
  cache.set(key, promise)
  return promise
}

function loadAPIGroups() {
  const key = clusterKey()
  return cached(apiGroups, key, () =>
    apiClient.get<APIGroupList>('/kubernetes/apis')
  )
}

function loadResourceList(groupVersion: string) {
  const key = `${clusterKey()}:${groupVersion}`
  const path =
    groupVersion === 'v1'
      ? '/kubernetes/api/v1'
      : `/kubernetes/apis/${groupVersion}`
  return cached(resourceLists, key, () => apiClient.get<APIResourceList>(path))
}

async function discoverBuiltInResource(
  resource: string,
  group: string
): Promise<KubernetesResourceDescriptor> {
  const plural = pluralAliases[resource] || resource
  const versions =
    group === ''
      ? ['v1']
      : await loadAPIGroups().then((discovery) => {
          const discoveredGroup = discovery.groups.find(
            (candidate) => candidate.name === group
          )
          if (!discoveredGroup) return []
          const preferred = discoveredGroup.preferredVersion?.groupVersion
          return [
            ...(preferred ? [preferred] : []),
            ...discoveredGroup.versions.map((version) => version.groupVersion),
          ].filter((version, index, all) => all.indexOf(version) === index)
        })

  for (const groupVersion of versions) {
    const list = await loadResourceList(groupVersion)
    const discovered = list.resources.find(
      (candidate) => candidate.name === plural
    )
    if (discovered) {
      return {
        apiVersion: list.groupVersion,
        plural,
        namespaced: discovered.namespaced,
      }
    }
  }
  throw new Error(`Kubernetes API does not serve resource ${resource}`)
}

async function discoverCustomResource(
  crdName: string
): Promise<KubernetesResourceDescriptor> {
  const crd = await apiClient.get<CustomResourceDefinition>(
    `/kubernetes/apis/apiextensions.k8s.io/v1/customresourcedefinitions/${encodeURIComponent(crdName)}`
  )
  const version =
    crd.spec.versions.find((candidate) => candidate.storage)?.name ??
    crd.spec.versions.find((candidate) => candidate.served)?.name
  if (!version)
    throw new Error(`Custom resource ${crdName} has no served version`)
  return {
    apiVersion: `${crd.spec.group}/${version}`,
    plural: crd.spec.names.plural,
    namespaced: crd.spec.scope === 'Namespaced',
  }
}

export function getKubernetesResourceDescriptor(resource: string) {
  const key = `${clusterKey()}:${resource}`
  return cached(descriptors, key, () => {
    const group = groupHints[resource]
    return group === undefined
      ? discoverCustomResource(resource)
      : discoverBuiltInResource(resource, group)
  })
}

function apiRoot(apiVersion: string): string {
  return apiVersion === 'v1' ? '/api/v1' : `/apis/${apiVersion}`
}

export async function getKubernetesResourcePath(
  resource: string,
  namespace?: string,
  name?: string,
  subresource?: string
): Promise<string> {
  const descriptor = await getKubernetesResourceDescriptor(resource)
  let path = apiRoot(descriptor.apiVersion)
  if (descriptor.namespaced && namespace && namespace !== '_all') {
    path += `/namespaces/${encodeURIComponent(namespace)}`
  }
  path += `/${encodeURIComponent(descriptor.plural)}`
  if (name) path += `/${encodeURIComponent(name)}`
  if (subresource) path += `/${encodeURIComponent(subresource)}`
  return `/kubernetes${path}`
}

export function clearKubernetesDiscoveryCache() {
  descriptors.clear()
  apiGroups.clear()
  resourceLists.clear()
}
