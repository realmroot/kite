import { useState } from 'react'
import { IconEdit, IconServer } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import { Cluster } from '@/types/api'
import { ClusterCreateRequest } from '@/lib/api'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

interface ClusterDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  cluster?: Cluster | null
  onSubmit: (clusterData: ClusterCreateRequest) => void
  gatewayEnabled: boolean
  isSubmitting?: boolean
}

function createClusterFormData(cluster?: Cluster | null): ClusterCreateRequest {
  return {
    name: cluster?.name || '',
    description: cluster?.description || '',
    apiServerUrl: cluster?.apiServerUrl || '',
    caBundle: cluster?.caBundle || '',
    tlsServerName: cluster?.tlsServerName || '',
    connectionMode: cluster?.connectionMode || 'direct',
    connectorId: cluster?.connectorId || '',
    connectorUrl: cluster?.connectorUrl || '',
    prometheusURL: cluster?.prometheusURL || '',
    enabled: cluster?.enabled ?? true,
    isDefault: cluster?.isDefault ?? false,
  }
}

function clusterIdFromName(name: string): string {
  const id = name
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '')
    .slice(0, 63)
    .replace(/-$/g, '')
  return id || 'cluster'
}

export function ClusterDialog({
  open,
  onOpenChange,
  cluster,
  onSubmit,
  gatewayEnabled,
  isSubmitting,
}: ClusterDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? (
        <ClusterDialogContent
          key={cluster?.id ?? 'new'}
          cluster={cluster}
          onOpenChange={onOpenChange}
          onSubmit={onSubmit}
          gatewayEnabled={gatewayEnabled}
          isSubmitting={isSubmitting}
        />
      ) : null}
    </Dialog>
  )
}

function ClusterDialogContent({
  cluster,
  onOpenChange,
  onSubmit,
  gatewayEnabled,
  isSubmitting,
}: Omit<ClusterDialogProps, 'open'>) {
  const { t } = useTranslation()
  const isEditMode = Boolean(cluster)
  const [formData, setFormData] = useState(() => createClusterFormData(cluster))
  const change = <K extends keyof ClusterCreateRequest>(
    key: K,
    value: ClusterCreateRequest[K]
  ) => setFormData((current) => ({ ...current, [key]: value }))
  const direct = formData.connectionMode === 'direct'
  const connector = formData.connectionMode === 'connector'
  const canSubmit =
    formData.name.trim() !== '' &&
    (formData.connectionMode === 'tunnel' ||
      (direct && formData.apiServerUrl?.trim() !== '') ||
      (connector &&
        formData.connectorId?.trim() !== '' &&
        formData.connectorUrl?.trim() !== ''))

  return (
    <DialogContent className="sm:max-w-[640px]">
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          {isEditMode ? (
            <IconEdit className="h-5 w-5" />
          ) : (
            <IconServer className="h-5 w-5" />
          )}
          {isEditMode
            ? t('clusterManagement.dialog.editTitle', 'Edit Cluster')
            : t('clusterManagement.dialog.createTitle', 'Add New Cluster')}
        </DialogTitle>
      </DialogHeader>

      <form
        className="space-y-4"
        onSubmit={(event) => {
          event.preventDefault()
          onSubmit(formData)
        }}
      >
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="cluster-name">
              {t('clusterManagement.dialog.name', 'Cluster Name')} *
            </Label>
            <Input
              id="cluster-name"
              value={formData.name}
              onChange={(event) => {
                const name = event.target.value
                setFormData((current) => ({
                  ...current,
                  name,
                  ...(!isEditMode && current.connectionMode === 'connector'
                    ? { connectorId: clusterIdFromName(name) }
                    : {}),
                }))
              }}
              placeholder="production"
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="cluster-connection-mode">
              {t('clusterManagement.dialog.connectionMode', 'Connection Mode')}
            </Label>
            <Select
              value={formData.connectionMode}
              disabled={isEditMode && !gatewayEnabled}
              onValueChange={(value: 'direct' | 'tunnel' | 'connector') => {
                setFormData((current) => ({
                  ...current,
                  connectionMode: value,
                  ...(value === 'direct' && gatewayEnabled
                    ? {
                        caBundle: '',
                        tlsServerName: '',
                        connectorId: '',
                        connectorUrl: '',
                      }
                    : {}),
                  ...(value === 'connector' && gatewayEnabled
                    ? {
                        apiServerUrl: '',
                        caBundle: '',
                        tlsServerName: '',
                      }
                    : {}),
                  ...(value === 'connector' && !current.connectorId
                    ? {
                        connectorId: clusterIdFromName(current.name),
                      }
                    : {}),
                }))
              }}
            >
              <SelectTrigger id="cluster-connection-mode">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="direct">
                  {t('clusterManagement.type.direct', 'Direct')}
                </SelectItem>
                {gatewayEnabled ? (
                  <SelectItem value="connector">
                    {t('clusterManagement.type.connector', 'Connector')}
                  </SelectItem>
                ) : (
                  <SelectItem value="tunnel">
                    {t('clusterManagement.type.tunnel', 'Private Tunnel')}
                  </SelectItem>
                )}
              </SelectContent>
            </Select>
          </div>
        </div>

        <div className="space-y-2">
          <Label htmlFor="cluster-description">
            {t('clusterManagement.dialog.description', 'Description')}
          </Label>
          <Textarea
            id="cluster-description"
            value={formData.description}
            onChange={(event) => change('description', event.target.value)}
            rows={2}
          />
        </div>

        {direct || connector ? (
          <>
            {direct ? (
              <div className="space-y-2">
                <Label htmlFor="api-server">
                  {t(
                    'clusterManagement.dialog.apiServerUrl',
                    'Kubernetes API Server URL'
                  )}{' '}
                  *
                </Label>
                <Input
                  id="api-server"
                  type="url"
                  value={formData.apiServerUrl}
                  onChange={(event) =>
                    change('apiServerUrl', event.target.value)
                  }
                  placeholder="https://api.cluster.example:6443"
                  required
                />
              </div>
            ) : null}
            {connector ? (
              <>
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                  <div className="space-y-2">
                    <Label htmlFor="connector-id">
                      {t(
                        'clusterManagement.dialog.connectorId',
                        'Connector ID'
                      )}{' '}
                      *
                    </Label>
                    <Input
                      id="connector-id"
                      value={formData.connectorId}
                      readOnly
                      aria-describedby="connector-id-description"
                      placeholder="production"
                      required
                    />
                    <p
                      id="connector-id-description"
                      className="text-xs text-muted-foreground"
                    >
                      {t(
                        'clusterManagement.dialog.connectorIdDescription',
                        'Generated from the cluster name and used by the Connector deployment.'
                      )}
                    </p>
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="connector-url">
                      {t(
                        'clusterManagement.dialog.connectorUrl',
                        'Connector URL'
                      )}{' '}
                      *
                    </Label>
                    <Input
                      id="connector-url"
                      type="url"
                      value={formData.connectorUrl}
                      onChange={(event) =>
                        change('connectorUrl', event.target.value)
                      }
                      placeholder="https://connector.cluster.example"
                      required
                    />
                  </div>
                </div>
                <p className="text-xs text-muted-foreground">
                  {t(
                    'clusterManagement.dialog.connectorDescription',
                    'Deploy one Connector for this cluster. The control plane stores only its HTTPS address and never stores Kubernetes credentials.'
                  )}
                </p>
              </>
            ) : gatewayEnabled ? (
              <p className="text-xs text-muted-foreground">
                {t(
                  'clusterManagement.dialog.directDescription',
                  'Direct mode requires a publicly reachable API server with a publicly trusted TLS certificate.'
                )}
              </p>
            ) : null}
          </>
        ) : (
          <div className="rounded-lg border border-blue-200 bg-blue-50 p-4 text-sm text-blue-700 dark:border-blue-800 dark:bg-blue-950/20 dark:text-blue-300">
            {t(
              'clusterManagement.dialog.tunnelDescription',
              'After creation, install the generated transport-only agent. It receives no Kubernetes ServiceAccount or RBAC permissions; user ID tokens pass through the tunnel to the API server.'
            )}
          </div>
        )}

        <div className="space-y-2">
          <Label htmlFor="prometheus-url">
            {t('clusterManagement.dialog.prometheusUrl', 'Prometheus URL')}
          </Label>
          <Input
            id="prometheus-url"
            type="url"
            value={formData.prometheusURL}
            onChange={(event) => change('prometheusURL', event.target.value)}
            placeholder={t('clusterManagement.dialog.prometheusUrlPlaceholder')}
          />
          <p className="text-xs text-muted-foreground">
            {t('clusterManagement.dialog.prometheusUrlDescription')}
          </p>
        </div>

        <div className="space-y-4 border-t pt-4">
          <div className="flex items-center justify-between">
            <Label htmlFor="cluster-enabled">
              {t('clusterManagement.dialog.enabled', 'Enable Cluster')}
            </Label>
            <Switch
              id="cluster-enabled"
              checked={formData.enabled}
              onCheckedChange={(value) => change('enabled', value)}
            />
          </div>
          <div className="flex items-center justify-between">
            <Label htmlFor="cluster-default">
              {t('clusterManagement.dialog.isDefault', 'Set as Default')}
            </Label>
            <Switch
              id="cluster-default"
              checked={formData.isDefault}
              onCheckedChange={(value) => change('isDefault', value)}
            />
          </div>
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t('common.actions.cancel', 'Cancel')}
          </Button>
          <Button type="submit" disabled={!canSubmit || isSubmitting}>
            {isSubmitting
              ? t('common.messages.saving', 'Saving...')
              : isEditMode
                ? t('common.actions.saveChanges', 'Save Changes')
                : t('clusterManagement.actions.add', 'Add Cluster')}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
