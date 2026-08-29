import type { CustomResourceDefinition } from 'kubernetes-types/apiextensions/v1'

import type { ResourceType } from '@/types/api'
import { apiClient } from '@/lib/api-client'
import { isClusterScopedResource } from '@/lib/resource-metadata'

type KubernetesResourceDescriptor = {
  apiVersion: string
  plural: string
  namespaced: boolean
}

const apiVersions: Partial<Record<ResourceType, string>> = {
  pods: 'v1',
  namespaces: 'v1',
  nodes: 'v1',
  services: 'v1',
  endpoints: 'v1',
  podtemplates: 'v1',
  replicationcontrollers: 'v1',
  limitranges: 'v1',
  resourcequotas: 'v1',
  componentstatuses: 'v1',
  configmaps: 'v1',
  secrets: 'v1',
  persistentvolumes: 'v1',
  persistentvolumeclaims: 'v1',
  serviceaccounts: 'v1',
  events: 'v1',
  deployments: 'apps/v1',
  replicasets: 'apps/v1',
  controllerrevisions: 'apps/v1',
  statefulsets: 'apps/v1',
  daemonsets: 'apps/v1',
  poddisruptionbudgets: 'policy/v1',
  jobs: 'batch/v1',
  cronjobs: 'batch/v1',
  ingresses: 'networking.k8s.io/v1',
  networkpolicies: 'networking.k8s.io/v1',
  ingressclasses: 'networking.k8s.io/v1',
  ipaddresses: 'networking.k8s.io/v1',
  servicecidrs: 'networking.k8s.io/v1',
  storageclasses: 'storage.k8s.io/v1',
  volumeattachments: 'storage.k8s.io/v1',
  csidrivers: 'storage.k8s.io/v1',
  csinodes: 'storage.k8s.io/v1',
  csistoragecapacities: 'storage.k8s.io/v1',
  volumeattributesclasses: 'storage.k8s.io/v1',
  roles: 'rbac.authorization.k8s.io/v1',
  rolebindings: 'rbac.authorization.k8s.io/v1',
  clusterroles: 'rbac.authorization.k8s.io/v1',
  clusterrolebindings: 'rbac.authorization.k8s.io/v1',
  certificatesigningrequests: 'certificates.k8s.io/v1',
  clustertrustbundles: 'certificates.k8s.io/v1alpha1',
  podcertificaterequests: 'certificates.k8s.io/v1beta1',
  leases: 'coordination.k8s.io/v1',
  leasecandidates: 'coordination.k8s.io/v1alpha2',
  runtimeclasses: 'node.k8s.io/v1',
  priorityclasses: 'scheduling.k8s.io/v1',
  workloads: 'scheduling.k8s.io/v1alpha2',
  podgroups: 'scheduling.k8s.io/v1alpha2',
  flowschemas: 'flowcontrol.apiserver.k8s.io/v1',
  prioritylevelconfigurations: 'flowcontrol.apiserver.k8s.io/v1',
  validatingadmissionpolicies: 'admissionregistration.k8s.io/v1',
  validatingadmissionpolicybindings: 'admissionregistration.k8s.io/v1',
  validatingwebhookconfigurations: 'admissionregistration.k8s.io/v1',
  mutatingwebhookconfigurations: 'admissionregistration.k8s.io/v1',
  mutatingadmissionpolicies: 'admissionregistration.k8s.io/v1',
  mutatingadmissionpolicybindings: 'admissionregistration.k8s.io/v1',
  resourceslices: 'resource.k8s.io/v1',
  resourceclaims: 'resource.k8s.io/v1',
  deviceclasses: 'resource.k8s.io/v1',
  resourceclaimtemplates: 'resource.k8s.io/v1',
  devicetaintrules: 'resource.k8s.io/v1alpha3',
  resourcepoolstatusrequests: 'resource.k8s.io/v1alpha3',
  storageversions: 'internal.apiserver.k8s.io/v1alpha1',
  storageversionmigrations: 'storagemigration.k8s.io/v1beta1',
  crds: 'apiextensions.k8s.io/v1',
  podmetrics: 'metrics.k8s.io/v1beta1',
  gateways: 'gateway.networking.k8s.io/v1',
  httproutes: 'gateway.networking.k8s.io/v1',
  horizontalpodautoscalers: 'autoscaling/v2',
  endpointslices: 'discovery.k8s.io/v1',
}

const customResourceDescriptors = new Map<
  string,
  Promise<KubernetesResourceDescriptor>
>()

function apiRoot(apiVersion: string): string {
  return apiVersion === 'v1' ? '/api/v1' : `/apis/${apiVersion}`
}

async function discoverCustomResource(
  crdName: string
): Promise<KubernetesResourceDescriptor> {
  const existing = customResourceDescriptors.get(crdName)
  if (existing) return existing

  const discovery = apiClient
    .get<CustomResourceDefinition>(
      `/kubernetes/apis/apiextensions.k8s.io/v1/customresourcedefinitions/${encodeURIComponent(crdName)}`
    )
    .then((crd) => {
      const version =
        crd.spec.versions.find((candidate) => candidate.storage)?.name ??
        crd.spec.versions.find((candidate) => candidate.served)?.name
      if (!version) {
        throw new Error(`Custom resource ${crdName} has no served version`)
      }
      return {
        apiVersion: `${crd.spec.group}/${version}`,
        plural: crd.spec.names.plural,
        namespaced: crd.spec.scope === 'Namespaced',
      }
    })
    .catch((error) => {
      customResourceDescriptors.delete(crdName)
      throw error
    })
  customResourceDescriptors.set(crdName, discovery)
  return discovery
}

export async function getKubernetesResourceDescriptor(
  resource: string
): Promise<KubernetesResourceDescriptor> {
  const apiVersion = apiVersions[resource as ResourceType]
  if (apiVersion) {
    return {
      apiVersion,
      plural: resource === 'podmetrics' ? 'pods' : resource,
      namespaced: !isClusterScopedResource(resource),
    }
  }
  return discoverCustomResource(resource)
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
